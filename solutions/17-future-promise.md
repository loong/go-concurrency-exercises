# Future/Promise Pattern: Async, Memoized Computation — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `17-future-promise/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a `Future` that's supposed to represent an asynchronous, memoized result:

```go
type Future struct {
	result int
}

func NewFuture(key string) *Future {
	return &Future{result: ComputeExpensive(key)}
}

func (f *Future) Get() int {
	return f.result
}
```

`Future` must:

- Have `NewFuture(key string) *Future` **kick off `ComputeExpensive(key)` in its own goroutine** (see `mockcompute.go` — it sleeps for `ComputeLatency` = 150ms before returning a deterministic, key-derived result) and **return near-instantly**, without waiting for the computation.
- Have `Get() int` **block until the result is ready**, be **safe to call concurrently from many goroutines**, and be safe to call **multiple times** — always returning the same cached result.
- Trigger **exactly one** underlying call to `ComputeExpensive`, no matter how many goroutines call `Get()` or how many times.
- Keep the signatures unchanged: `func NewFuture(key string) *Future` and `func (f *Future) Get() int`.

## Why the naive version is wrong

`NewFuture` calls `ComputeExpensive(key)` **synchronously**, right on the calling goroutine, before the `Future` is even constructed. That means:

- Every call to `NewFuture` blocks the caller for the full 150ms of `ComputeExpensive` up front — defeating the entire point of a future, which is to let the caller go do other work while the result is computed in the background.
- `Get()` just returns the already-computed `result` field, so it happens to look "correct" and "memoized" — but only because all the real work already happened, synchronously, inside the constructor.

Verified: running the current `check_test.go` against this naive `main.go` in a throwaway scratch copy fails `TestNewFutureReturnsImmediately` (`NewFuture took 150ms (ComputeExpensive takes 150ms); want near-instant construction`). `TestFutureGetReturnsCorrectResult` and `TestFutureGetMemoizesAcrossManyCallers` both pass against the naive version — since the computation already happened before `Get` is ever called, there's nothing left to race on, and returning the cached field is trivially "memoized." The only test the naive version actually fails is the one asserting eagerness/asynchrony of construction itself.

## Approach 1: `done chan struct{}`, closed once the result is stored

```go
package main

import (
	"fmt"
	"time"
)

// Future represents the result of an asynchronous computation that
// may not have finished yet.
type Future struct {
	result int
	done   chan struct{}
}

// NewFuture starts computing the result for key and returns a Future
// representing it.
func NewFuture(key string) *Future {
	f := &Future{done: make(chan struct{})}

	go func() {
		f.result = ComputeExpensive(key)
		close(f.done)
	}()

	return f
}

// Get returns the result, blocking until it is ready.
func (f *Future) Get() int {
	<-f.done
	return f.result
}

func main() {
	start := time.Now()
	f := NewFuture("report-42")
	constructTime := time.Since(start)

	fmt.Printf("NewFuture returned after %s\n", constructTime)

	result := f.Get()
	fmt.Printf("Result: %d (total elapsed %s)\n", result, time.Since(start))
}
```

Design notes:

- **`done` is created before the goroutine launches**, so there's no window where a caller could call `Get()` against a `nil` channel.
- **The background goroutine writes `f.result` *before* closing `f.done`.** Closing a channel is a broadcast: every current and future receive on a closed channel unblocks immediately, and — critically — `close()` establishes a happens-before edge with every receive that observes it. So any goroutine unblocked by `<-f.done` is guaranteed to see the `f.result` write that happened before the `close`, with no separate lock needed around `result` itself.
- **This satisfies "exactly once" for free**, without any explicit call counter or `sync.Once` guard: there's only ever one goroutine launched per `Future` (inside `NewFuture`, called once per instance), and every `Get()` call — from one goroutine or a thousand racing ones — just blocks on the same channel and reads the same already-written field. No caller can trigger a second computation because nothing in `Get()` ever calls `ComputeExpensive` at all.
- `Get()` called multiple times, even after the result is ready, is safe and cheap: receiving from an already-closed channel never blocks, so repeated calls just return immediately with the cached `result`.

**Verified**: copied this exercise into a throwaway scratch directory, confirmed the naive `main.go` fails `TestNewFutureReturnsImmediately`, then dropped in this solution. `go vet ./...` is clean, and `go test -race -count=5 ./...` passes repeatably — including `TestFutureGetMemoizesAcrossManyCallers`, which launches 20 concurrent `Get()` callers and asserts `ComputeExpensive` ran exactly once.

## A tempting variant that doesn't actually qualify (not shipped as "Approach 2")

A natural-looking alternative: make `NewFuture` cheap by doing *nothing* but store the key, and defer the actual computation to a `sync.Once` triggered lazily on the *first* `Get()` call:

```go
type Future struct {
	key    string
	once   sync.Once
	result int
}

func NewFuture(key string) *Future {
	return &Future{key: key}
}

func (f *Future) Get() int {
	f.once.Do(func() {
		f.result = ComputeExpensive(f.key)
	})
	return f.result
}
```

This was checked empirically against the real `check_test.go`, and — worth calling out explicitly — **it passes all three tests**, `TestNewFutureReturnsImmediately` included. That's not a bug in the test, it's a gap in what that particular test can distinguish: `TestNewFutureReturnsImmediately` only measures how long the `NewFuture(...)` call itself takes to *return*; it never checks whether `ComputeExpensive` has actually started running in the background by the time control comes back to the caller. Since this variant does no work at all inside `NewFuture`, it "returns near-instantly" trivially — for the wrong reason.

But the exercise's own contract — stated in both `README.md` and the header comment in `main.go` — is explicit: *"`NewFuture(key string) *Future` kicks off `ComputeExpensive(key)` **in its own goroutine** and returns near-instantly."* That means the computation must start immediately, at construction time, running concurrently while the caller goes on to do other things — that's the entire premise of a "future" as a concurrency primitive (see the exercise's own framing: *"defeating the entire point of a future — you can't do other work while it's computing"*). The lazy-on-first-`Get` variant does the opposite: nothing happens until *someone* calls `Get()`, and if that call doesn't happen for a while (or ever), `ComputeExpensive` never runs at all during that window. A caller that does `f := NewFuture(key)` and then goes off to do 500ms of unrelated work, expecting the result to be ready or nearly ready by the time it calls `f.Get()`, gets no benefit whatsoever from this variant — it pays the full 150ms starting only at `Get()`, exactly like the original naive, fully-synchronous version, just moved to a different call site.

So this is a real, distinct synchronization idea (`sync.Once`-guarded lazy computation) — but it solves a different problem ("memoize an expensive on-demand computation, computed at most once, whenever first requested") rather than *this* exercise's problem (a true future: eager, backgrounded computation started at construction). It's included here as a documented near-miss rather than as "Approach 2," precisely so it isn't mistaken for a valid alternative solution.

## Key takeaways

- A closed channel is a broadcast with a happens-before guarantee: writing a result and then `close()`-ing a signal channel lets any number of goroutines block on `<-ch` and all safely observe the write the moment they unblock — no separate mutex needed around the result field.
- "Exactly once, no matter how many concurrent callers" falls out for free from the future pattern's shape: launch the background computation exactly once, in the constructor, and have every `Get()` — however many, from however many goroutines — merely *wait on* that single result rather than potentially triggering new work.
- Before trusting a test to fully pin down a spec, ask what it actually measures. `TestNewFutureReturnsImmediately` measures construction *latency*, not construction *eagerness* — a variant that does zero work in the constructor passes it just as well as one that correctly launches a background goroutine, even though only one of them is an actual future. When a given test doesn't distinguish two designs, fall back to the exercise's stated contract (README + doc comments) to judge correctness, not just "does `go test` pass."
- A `sync.Once`-guarded lazy computation and an eagerly-started background goroutine are both legitimate concurrency patterns, but they answer different questions — "compute once, on demand, whenever first asked" versus "start now, let the caller collect the result later" — and swapping one for the other silently changes when the expensive work actually runs, which matters a great deal to real callers even when a test suite can't tell the difference.
