# Your Own errgroup: Concurrent Tasks with First-Error Capture — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `16-errgroup-failfast/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a `Group` that mimics the shape of `golang.org/x/sync/errgroup`'s `Group` (no importing the real package — everything from scratch, stdlib only):

```go
type Group struct {
	firstErr error
}

func (g *Group) Go(f func() error) {
	if err := f(); err != nil && g.firstErr == nil {
		g.firstErr = err
	}
}

func (g *Group) Wait() error {
	return g.firstErr
}
```

`Group` must:

- Let `Go(f func() error)` launch `f` in its **own goroutine**, immediately, without blocking the caller.
- Let `Wait() error` block until **every** goroutine launched via `Go` has finished, then return the **first** error encountered — captured safely even though several tasks might fail around the same moment.
- Track the in-flight goroutines via an internal `sync.WaitGroup` (per the exercise's own guidance in `main.go`'s header comment).
- Need no `context`/cancellation support — this is `errgroup`'s base `Group`, not the `WithContext` variant.
- Keep the signatures unchanged: `func (g *Group) Go(f func() error)` and `func (g *Group) Wait() error`.

## Why the naive version is wrong

`Go` calls `f()` **synchronously**, right on the calling goroutine, before returning. That means:

- Tasks registered via `Go` run one after another, not concurrently — five 100ms tasks take 500ms+ total instead of ~100ms.
- `Wait` "works" only by accident: since `Go` already blocked until `f` finished, there's nothing left to wait for by the time `Wait` is called.
- `firstErr` is a plain field read and written with **no synchronization**. It happens to be race-free today only because `Go` never actually runs concurrently with anything. The moment `Go` is fixed to launch a real goroutine, concurrent writes to `firstErr` from multiple goroutines (and a concurrent read from `Wait`) become a live data race.

Verified: running the current `check_test.go` against this naive `main.go` in a throwaway scratch copy fails `TestGroupRunsConcurrently` (`Wait took 1s (sequential would take 1s); want well under 300ms`). The other two tests pass by coincidence, since the naive version is sequential by construction and therefore never races.

## Approach 1: `sync.WaitGroup` + `sync.Once`

```go
package main

import (
	"sync"
)

// Group runs a collection of tasks, started via Go, and collects the
// first error (if any) returned by them once every task has finished.
type Group struct {
	wg       sync.WaitGroup
	once     sync.Once
	firstErr error
}

// Go launches f in its own goroutine and returns immediately.
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

// Wait blocks until every task started via Go has finished, and
// returns the first error encountered, if any.
func (g *Group) Wait() error {
	g.wg.Wait()
	return g.firstErr
}
```

Design notes:

- **`wg.Add(1)` happens on the caller's goroutine, before `go func() {...}()` starts** — this is the standard, safe ordering for `sync.WaitGroup`; adding *inside* the new goroutine would race against `Wait()` possibly observing a zero counter before the `Add` runs.
- **`sync.Once` is what makes "first error wins" race-free.** Every failing task calls `g.once.Do(...)`, but only the very first caller to reach it actually executes the closure that writes `firstErr`; every later caller's `Do` is a no-op. `sync.Once` guarantees the happens-before edge itself, so no separate mutex is needed around the write.
- **`Wait` needs no lock to read `firstErr`.** `wg.Wait()` only returns once every tracked goroutine has called `Done()` — and every goroutine that might write `firstErr` does so *before* its `defer g.wg.Done()` fires (because the `once.Do` call is inside the goroutine body, ahead of the deferred `Done`). So by the time `Wait()`'s `wg.Wait()` unblocks, every write to `firstErr` has already happened-before it, transitively through the `WaitGroup`'s internal synchronization.
- Since `Group` is used as a zero value (`var g Group`) throughout the tests, both `sync.WaitGroup` and `sync.Once` being usable directly in their zero form (no constructor needed) is essential — this is exactly why they're a good fit here over, say, a channel that would need explicit initialization.

**Verified**: copied this exercise into a throwaway scratch directory, confirmed the naive `main.go` fails `TestGroupRunsConcurrently`, then dropped in this solution. `go vet ./...` is clean, and `go test -race -count=5 ./...` passes repeatably — including `TestGroupFirstErrorRaceSafe`, which stress-tests 50 tasks all failing at once with no sleep, specifically to catch a racy "first error" capture.

## Approach 2: `sync.WaitGroup` + buffered `chan error` (non-blocking send)

A genuinely different synchronization primitive for "capture only the first error": instead of `sync.Once` guarding a plain field, use a `chan error` with capacity 1 and a non-blocking `select`/`default` send — only the first send into the channel's single buffer slot succeeds; every subsequent send hits `default` and is silently dropped.

```go
package main

import (
	"sync"
)

// Group runs a collection of tasks, started via Go, and collects the
// first error (if any) returned by them once every task has finished.
type Group struct {
	wg       sync.WaitGroup
	errCh    chan error
	initOnce sync.Once
}

// init lazily creates errCh. It's called from both Go and Wait - not
// just Go - because Group has no constructor (callers use the zero
// value, var g Group), so every method that touches errCh must go
// through the same sync.Once to avoid an unsynchronized read/write
// race on the field itself the very first time either method runs.
func (g *Group) init() {
	g.initOnce.Do(func() {
		g.errCh = make(chan error, 1)
	})
}

// Go launches f in its own goroutine and returns immediately.
func (g *Group) Go(f func() error) {
	g.init()
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := f(); err != nil {
			select {
			case g.errCh <- err: // first failing task claims the one buffer slot
			default: // slot already taken - drop this error, keep the first
			}
		}
	}()
}

// Wait blocks until every task started via Go has finished, and
// returns the first error encountered, if any.
func (g *Group) Wait() error {
	g.init()
	g.wg.Wait()
	select {
	case err := <-g.errCh:
		return err
	default:
		return nil
	}
}
```

Design notes and tradeoffs versus Approach 1:

- **`errCh` needs lazy initialization, and `init()` must be called from *every* method that touches it, including `Wait`** — not just `Go`. `Group` has no constructor (tests use `var g Group`), so the channel doesn't exist until something creates it. It's tempting to only call `init()` from `Go`, since that's the only place that sends — but `Wait()` also reads the `errCh` *field itself*, and if `Wait()` is ever called concurrently with (or before) the very first `Go`, an unguarded field read races against `Go`'s unguarded field write. Routing both methods through the same `sync.Once` closes that gap.
- **The non-blocking `select`/`default` send is the channel-based analogue of `sync.Once.Do`**: the buffered channel's single slot can only be "claimed" once, so exactly one failing task's error gets through and every later one is silently dropped at the `default` case — same first-wins semantics, different primitive.
- **Important asymmetry: `Wait()` is single-use here, unlike Approach 1.** Because `Wait()` *drains* the channel (`<-g.errCh`), a second call to `Wait()` on the same `Group` returns `nil` even if the first call returned a real error — the buffered value was already consumed. Approach 1's `Wait()` is idempotent (`firstErr` is just read, never consumed), so calling it twice returns the same error both times. Confirmed empirically: added a second `g.Wait()` call after the first in both scratch versions — Approach 1 printed the same error both times; Approach 2 printed the error once, then `<nil>`. The given tests only call `Wait()` once, so neither behavior is caught by them, but it's a real difference to be aware of before choosing between the two in production code.
- Functionally equivalent for everything the tests actually assert; pick `sync.Once` (Approach 1) if `Wait()` might reasonably be called more than once, and the channel approach only if single-use `Wait()` is acceptable or you specifically want to model "one slot, first writer wins" with a channel instead of a guarded field.

**Verified**: same scratch-directory protocol. An earlier draft only called `init()` from `Go`, which left `Wait()`'s field read unguarded — this is precisely the kind of race the exercise's own tests can miss (all `Go` calls happen sequentially before `Wait` in every test, so the race is real but untriggered by this test suite). Fixed by calling `init()` at the top of `Wait()` too; after the fix, `go vet ./...` is clean and `go test -race -count=5 ./...` passes repeatably, including `TestGroupFirstErrorRaceSafe`.

## Key takeaways

- `sync.WaitGroup.Add(1)` must happen on the *calling* goroutine before `go func(){...}()`, never inside the new goroutine — otherwise `Wait()` can race past a counter that hasn't been incremented yet.
- `sync.Once` is a clean, lock-free way to express "only the first success/failure among many racing goroutines counts" — it needs no separate mutex because it provides its own happens-before guarantee for the guarded closure.
- A buffered `chan error` of capacity 1 with a non-blocking `select`/`default` send is a real alternative to `sync.Once` for "capture only the first" — but it comes with a hidden cost: reading via `<-ch` *consumes* the value, so the read side (`Wait`) becomes single-use unless you add extra bookkeeping to make it idempotent.
- When a type is used via its zero value with no constructor (`var g Group`, no `NewGroup()`), any lazily-initialized internal field (like a channel) must be guarded by the *same* `sync.Once` in **every** method that touches it — not just the method that happens to write it first in the common test flow. Missing this is a real, evidence-based data race that a happy-path test suite (calling `Go` before `Wait`, never concurrently) can fail to catch even under `-race`.
- Don't let a synchronous naive implementation's "accidental" race-freedom fool you: sequential code by construction never triggers the race detector, so a naive `Go`-blocks-immediately stub can look deceptively safe on the concurrency-safety tests while completely failing the concurrency-throughput test.
