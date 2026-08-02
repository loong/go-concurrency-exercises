# Graceful Multi-Stage Shutdown: Knowing When a Worker Pool Has REALLY Finished — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `25-graceful-multistage-shutdown/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is `Start`, which launches a small, fixed-size pool of worker goroutines:

```go
const numWorkers = 4

func Start(jobs <-chan int, process func(item int)) <-chan struct{} {
	for i := 0; i < numWorkers; i++ {
		go func() {
			for item := range jobs {
				process(item)
			}
		}()
	}

	done := make(chan struct{})
	close(done)
	return done
}
```

`Start` must:

- Return a `done` channel that closes **only** once `jobs` has been closed **and** every worker goroutine has fully returned from its `range jobs` loop — i.e. every worker has finished calling `process` on its last-received item.
- Keep the signature unchanged: `func Start(jobs <-chan int, process func(item int)) <-chan struct{}`.
- Use a `sync.WaitGroup`, incremented once per worker, marked `Done` when each worker returns from ranging over `jobs`, plus a small goroutine that calls `wg.Wait()` and then closes `done` — the natural tool per the exercise's own guidance.

## Why the naive version is wrong

`done` is created and immediately closed, with no relationship at all to the worker goroutines started just above it. A caller that waits on `done` gets no guarantee whatsoever that any job — let alone every job — has finished being processed; it only knows that `Start` has returned. If the caller then tears down something `process` writes into (a file, a DB connection, a socket), it can do so while workers are still silently mid-flight on jobs they already pulled off the channel, corrupting or losing that in-flight work.

Verified: running the current `check_test.go` against the naive `main.go` in a throwaway scratch copy fails `TestStartProcessesEveryJob`:

```
--- FAIL: TestStartProcessesEveryJob (0.00s)
    check_test.go:53: done closed with 0/20 jobs actually processed; done must only close once every submitted job has been FULLY processed, not merely received off the jobs channel
FAIL
```

`done` closes with **zero** of the 20 jobs actually processed — confirming it fires essentially instantly, well before any worker has done real work. `TestStartDoesNotDoubleProcessOrDropJobs` passes against the naive version too (it deliberately doesn't trust `done` for synchronization, polling the collected results instead), since the naive workers do eventually process every job correctly in the background — it's purely the *signal* that's wrong, not the underlying processing.

## Approach 1: `sync.WaitGroup` + a small closer goroutine

```go
package main

import (
	"sync"
)

const numWorkers = 4

// Start launches numWorkers worker goroutines that read from jobs and
// call process on each item until jobs is closed. The returned done
// channel closes only once jobs has been closed AND every worker has
// fully finished calling process on its last-received item, so a
// caller can safely wait on done and then treat the whole pipeline as
// gracefully, completely finished.
func Start(jobs <-chan int, process func(item int)) <-chan struct{} {
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				process(item)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	return done
}
```

Design notes:

- **`wg.Add(1)` happens on the calling goroutine, once per worker, before `go func() {...}()` starts** — the standard safe ordering for `sync.WaitGroup`. Incrementing from inside the new goroutine instead would race against the closer goroutine's `wg.Wait()` possibly observing a zero (or partially-incremented) counter before every `Add` has actually run.
- **`defer wg.Done()` sits at the top of each worker, so it fires exactly when the worker returns from `range jobs`** — which only happens once `jobs` is closed and drained. That's precisely "every worker has finished calling `process` on its last-received item": the `range` loop's last iteration completes its `process` call before the loop exits and the deferred `Done()` runs.
- **A separate goroutine does `wg.Wait()` then `close(done)`**, rather than the caller of `Start` doing it inline — `Start` must return immediately without blocking (it's meant to hand back a channel the caller waits on whenever it likes), so the wait has to happen concurrently with `Start`'s return.
- **`done` is closed, not sent on** — closing lets any number of goroutines observe the event via `<-done` without racing over who "gets" the single value a channel send would produce, and receiving from an already-closed channel never blocks, so late-arriving readers still see it fire correctly.

**Verified**: copied this exercise into a throwaway scratch directory, confirmed the naive `main.go` fails `TestStartProcessesEveryJob` (see above), then dropped in this solution. `gofmt -l` is clean, `go vet ./...` is clean, and `go test -race -count=5 ./...` passes repeatably — including `TestStartDoesNotDoubleProcessOrDropJobs`, which stress-tests 50 jobs for drops/duplicates independently of the `done`-timing fix.

## Approach 2: the exercise-16 `Group` pattern instead of a bare `WaitGroup`

A genuinely different composition, worth knowing if you've just come from [exercise 16](../16-errgroup-failfast) (`16-errgroup-failfast/`): that exercise builds a tiny hand-rolled `errgroup.Group` — `Go(f func() error)` launches a task in its own goroutine without blocking, and `Wait() error` blocks until every launched task returns, then returns the first error encountered (via `sync.WaitGroup` + `sync.Once`). That `Group` is a natural fit here too: instead of managing a bare `sync.WaitGroup` by hand in `Start`, launch each worker via `g.Go(...)` and let the coordinator goroutine call `g.Wait()` before closing `done`.

```go
package main

import (
	"sync"
)

const numWorkers = 4

// Group is the exact same hand-rolled errgroup.Group built in exercise
// 16 (16-errgroup-failfast/main.go, Approach 1 there): Go launches a
// task in its own goroutine without blocking, tracked by an internal
// WaitGroup, and Wait blocks until every task launched via Go has
// returned, then returns the first error encountered (via sync.Once).
// This exercise's workers never fail, so every worker closure just
// returns nil - but the type is unmodified from exercise 16.
type Group struct {
	wg       sync.WaitGroup
	once     sync.Once
	firstErr error
}

func (g *Group) Go(f func() error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := f(); err != nil {
			g.once.Do(func() {
				g.firstErr = err
			})
		}
	}()
}

func (g *Group) Wait() error {
	g.wg.Wait()
	return g.firstErr
}

// Start launches numWorkers worker goroutines (via Group.Go) that read
// from jobs and call process on each item until jobs is closed. The
// returned done channel closes only once jobs has been closed AND
// every worker has fully finished calling process on its
// last-received item.
func Start(jobs <-chan int, process func(item int)) <-chan struct{} {
	var g Group

	for i := 0; i < numWorkers; i++ {
		g.Go(func() error {
			for item := range jobs {
				process(item)
			}
			return nil
		})
	}

	done := make(chan struct{})
	go func() {
		_ = g.Wait() // no worker here ever returns an error
		close(done)
	}()

	return done
}
```

Design notes and tradeoffs versus Approach 1:

- **Structurally identical under the hood** — `Group.Go` is still a `wg.Add(1)` on the caller's goroutine followed by a tracked goroutine that calls `wg.Done()` via `defer`, and `Group.Wait` is still `wg.Wait()`. The difference is purely one of composition: Approach 1 manages the `WaitGroup` directly inside `Start`, while Approach 2 delegates that bookkeeping to a reusable `Group` type that also happens to carry error-collection semantics this exercise doesn't need.
- **The worker closures have to return `error` to fit `Group.Go`'s signature**, even though nothing here can fail — hence `return nil` at the end of each worker's loop, and `_ = g.Wait()` discarding the always-`nil` result. That's the real cost of reusing a general-purpose `Group`: it's built for tasks that can fail, and adapting a can't-fail worker to it means threading a trivial `nil` through.
- **Worth it mainly as a cross-reference, not as an improvement**: if you're already building (or have already built) an `errgroup`-style `Group` elsewhere in a codebase, reusing it here avoids a second, slightly different hand-rolled `WaitGroup` pattern living side by side with it. If this were the only place needing "wait for N goroutines to finish," Approach 1's bare `sync.WaitGroup` is simpler and doesn't require inventing a `nil` return value.

**Verified**: same scratch-directory protocol, in a separate throwaway copy from Approach 1. `gofmt -l` is clean, `go vet ./...` is clean, and `go test -race -count=5 ./...` passes repeatably, including `TestStartProcessesEveryJob` and `TestStartDoesNotDoubleProcessOrDropJobs`.

## Key takeaways

- "The channel returned by a function closes eventually" and "the channel closes only after specific work has genuinely finished" are very different guarantees — a `done` channel that's just created-and-closed on the spot satisfies the first trivially while providing none of the second. Always ask what event the close is actually synchronized with.
- `sync.WaitGroup.Add(1)` must happen on the *calling* goroutine before `go func(){...}()`, never inside the new goroutine — otherwise `Wait()` can race past a counter that hasn't been (fully) incremented yet.
- `defer wg.Done()` placed at the very top of a goroutine, right after entry, is what ties "the WaitGroup counter reaches zero" to "every one of those goroutines has fully returned" — including from its last unit of real work, not just from having been scheduled.
- A dedicated goroutine doing `wg.Wait(); close(done)` is the standard way to convert a blocking wait into a channel-based signal that the function itself can return immediately, letting the caller decide when (or whether) to block on it.
- Reusing a more general pattern (like exercise 16's `errgroup`-style `Group`) in a spot that doesn't need its extra capability (error collection, here) is a legitimate but not free choice — it costs a bit of ceremony (threading a `nil` error through) in exchange for one less bespoke synchronization pattern in the codebase.
