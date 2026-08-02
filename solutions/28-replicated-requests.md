# Replicated Requests: Racing Redundant Calls for Lower Tail Latency - Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `28-replicated-requests/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a `FetchFastest` that fans out the same request to several redundant `Replica` handlers concurrently and is supposed to return whichever one answers first, so that no single replica's unpredictable tail latency slows the caller down:

```go
type Replica func(done <-chan struct{}) (string, error)

func FetchFastest(done <-chan struct{}, replicas ...Replica) (string, error) {
	if len(replicas) == 0 {
		return "", errors.New("fetchfastest: no replicas provided")
	}

	results := make(chan replicaResult, len(replicas))

	for _, replica := range replicas {
		go func(r Replica) {
			// BUG: every replica is handed a nil done, so none of
			// them can ever learn that FetchFastest already picked a
			// winner (or that the caller cancelled). A losing replica
			// just keeps running for its full artificial latency
			// instead of stopping as soon as it lost the race.
			value, err := r(nil)
			results <- replicaResult{value: value, err: err}
		}(replica)
	}

	select {
	case res := <-results:
		return res.value, res.err
	case <-done:
		return "", errors.New("fetchfastest: cancelled before any replica responded")
	}
}
```

It already gets the headline behavior right: it starts one goroutine per replica and returns the first value/error that arrives on `results`, so a test that only checks "does it return the right value" passes against it as-is.

The `Replica` type and the `FetchFastest(done <-chan struct{}, replicas ...Replica) (string, error)` signature must stay identical.

## Why the naive version is wrong

Look at the goroutine each replica runs in: `r(nil)`. Every replica is handed a `nil` done channel — not the caller's `done`, not any channel `FetchFastest` controls. A `nil` channel blocks forever on either send or receive, so from inside a `Replica`, `case <-done` (where `done` is `nil`) never fires. Once `FetchFastest` has its winner and returns, every losing replica has absolutely no way to find out. It just keeps sleeping out its own artificial latency (or doing whatever work it was doing) to the very end, wastefully, with nobody left listening for the result — a goroutine leak per call that self-heals only because the mock's timer eventually fires on its own.

Verified: running the current `check_test.go` against this naive `main.go` fails two tests, and for exactly this reason - every in-flight replica keeps running to completion because it was handed a `nil` done, whether a replica already won or the caller's own `done` fired first:

```
=== RUN   TestFetchFastestReturnsFastestValue
    check_test.go:58: replica "slow1" never observed its done channel being closed - it is going to keep running all the way to its full artificial latency instead of stopping as soon as it lost the race
    check_test.go:58: replica "slow2" never observed its done channel being closed - it is going to keep running all the way to its full artificial latency instead of stopping as soon as it lost the race
    check_test.go:58: replica "slow3" never observed its done channel being closed - it is going to keep running all the way to its full artificial latency instead of stopping as soon as it lost the race
--- FAIL: TestFetchFastestReturnsFastestValue (0.00s)
=== RUN   TestFetchFastestReturnsWinnerError
--- PASS: TestFetchFastestReturnsWinnerError (0.00s)
=== RUN   TestFetchFastestClosedDoneCancelsEarly
    check_test.go:142: replica "a" never observed its done channel being closed after the caller's done fired with no winner - it is going to keep running all the way to its full artificial latency instead of stopping as soon as the caller cancelled
    check_test.go:142: replica "b" never observed its done channel being closed after the caller's done fired with no winner - it is going to keep running all the way to its full artificial latency instead of stopping as soon as the caller cancelled
--- FAIL: TestFetchFastestClosedDoneCancelsEarly (0.00s)
=== RUN   TestFetchFastestNoReplicas
--- PASS: TestFetchFastestNoReplicas (0.00s)
=== RUN   TestFetchFastestConcurrentSafety
--- PASS: TestFetchFastestConcurrentSafety (0.00s)
FAIL
```

`TestFetchFastestReturnsWinnerError`, `TestFetchFastestNoReplicas`, and `TestFetchFastestConcurrentSafety` all pass against the naive version — none of them inspect what happens to the *losing* replicas, only that `FetchFastest` itself answers correctly (value, winner's error, or the no-replicas error) and doesn't race. `TestFetchFastestReturnsFastestValue` and `TestFetchFastestClosedDoneCancelsEarly` are the ones that specifically check the losers' fate (`ObservedCancellation()` / `RanToCompletion()`) - one for the "a replica won" cancellation trigger, the other for the "caller's own `done` fired with no winner" trigger - and both catch the naive version. That's the point of the exercise: the naive version's answer is right, its cleanup is not.

## Approach 1: a single shared `stop` channel, closed via `defer`

```go
package main

import (
	"errors"
	"fmt"
	"time"
)

// Replica represents one redundant handler that can serve a request.
// A well-behaved Replica must return promptly once done is closed,
// instead of continuing to run to completion regardless.
type Replica func(done <-chan struct{}) (string, error)

// replicaResult carries one replica's outcome back to FetchFastest.
type replicaResult struct {
	value string
	err   error
}

// FetchFastest calls every replica concurrently (one goroutine each)
// and returns the value and error from whichever one sends on its own
// result channel FIRST - later stragglers are ignored. If done is
// closed before any replica has responded, FetchFastest returns early
// with an error and no winner.
func FetchFastest(done <-chan struct{}, replicas ...Replica) (string, error) {
	if len(replicas) == 0 {
		return "", errors.New("fetchfastest: no replicas provided")
	}

	// stop is the done channel every replica goroutine actually
	// receives. Closing it is how FetchFastest tells every
	// still-running replica "stop, you already lost (or the caller
	// gave up)". It is closed exactly once, on the way out of this
	// function, by the deferred close below - covering both the
	// "we have a winner" and "the caller's done fired" return paths.
	stop := make(chan struct{})
	defer close(stop)

	results := make(chan replicaResult, len(replicas))

	for _, replica := range replicas {
		go func(r Replica) {
			value, err := r(stop)
			results <- replicaResult{value: value, err: err}
		}(replica)
	}

	select {
	case res := <-results:
		return res.value, res.err
	case <-done:
		return "", errors.New("fetchfastest: cancelled before any replica responded")
	}
}

func main() {
	replicas := []Replica{
		NewMockReplica("replica-A", 150*time.Millisecond).Replica,
		NewMockReplica("replica-B", 10*time.Millisecond).Replica,
		NewMockReplica("replica-C", 300*time.Millisecond).Replica,
	}

	done := make(chan struct{})
	start := time.Now()

	value, err := FetchFastest(done, replicas...)

	fmt.Printf("winner after %s: value=%q err=%v\n", time.Since(start), value, err)
	fmt.Println("...and replica-A and replica-C were told to stop as soon as replica-B won,")
	fmt.Println("instead of running to completion in the background for nothing.")

	// Give the (now-cancelled) losing replicas a moment to actually
	// return before main exits, just so this demo doesn't race main's
	// own exit against their early-return bookkeeping.
	time.Sleep(20 * time.Millisecond)
}
```

Design notes:

- **One channel, closed once, does double duty for both cancellation sources.** `stop` is what every replica goroutine actually receives as its own `done`. `defer close(stop)` fires on *every* return path out of `FetchFastest` — whether the `select` took the "a replica won" branch or the "caller's `done` fired" branch — so there is exactly one place that decides "everyone stop now," and it's impossible to add a future return path that forgets to close it.
- **The `results` channel's buffer size (`len(replicas)`) is not incidental — it's what makes the fix actually leak-free.** After `stop` closes, every losing replica wakes up from its `case <-done` branch and still runs `results <- replicaResult{...}`. If `results` were unbuffered, every one of those sends would block forever with nobody left to receive them — you'd have swapped a *timer* leak for a *channel-send* leak, and the goroutines would still never exit even though `ObservedCancellation()` now (correctly) reports `true`. Buffering to `len(replicas)` guarantees every send — the eventual winner's and every loser's — always has room, so no goroutine ever blocks on that send. (The equally correct alternative, if you didn't want to size the buffer to match the fan-out, is to make each goroutine's send itself cancellable: `select { case results <- r: case <-stop: }`.) Under `testing/synctest`, getting this wrong doesn't show up as a failing assertion — it surfaces as a hard panic. Confirmed by actually making `results` unbuffered (`make(chan replicaResult)`) in a throwaway scratch build against this fix and running `TestFetchFastestReturnsFastestValue`: it panics with `deadlock: main bubble goroutine has exited but blocked goroutines remain`, with the reported blocked goroutines sitting at exactly the `results <- replicaResult{...}` send, tagged `chan send (durable)` — a goroutine blocked forever on an unbuffered send is exactly the kind of durably-blocked goroutine synctest refuses to let the bubble close around.
- **A replica only ever needs to know "stop," never "who won" or "why."** `stop` carries no payload and is never inspected for a reason — it's a pure "close = stop" signal, so every `Replica` implementation only needs one `select` arm (`case <-done:`) to be well-behaved, regardless of whether it lost to another replica or the caller gave up on all of them.

**Verified**: ran the current `check_test.go` against this naive `main.go` in a throwaway scratch copy and confirmed the failure above, then dropped in this fix (in the same scratch copy — the real repo's `28-replicated-requests/main.go` was never modified). `go build ./...` and `go vet ./...` are clean, and `go test -race -count=5 ./...` and a further `go test -race -count=15 ./...` both passed with zero flakes across all five tests — including `TestFetchFastestReturnsFastestValue` and `TestFetchFastestClosedDoneCancelsEarly`, which both run inside `synctest.Test` with fake time and specifically check `ObservedCancellation()`/`RanToCompletion()` on every loser (the former for the "a replica won" trigger, the latter for the "caller's own `done` fired with no winner" trigger), `TestFetchFastestReturnsWinnerError`, which checks that the winner's own error is propagated rather than discarded, and `TestFetchFastestConcurrentSafety`, which hammers `FetchFastest` from 20 concurrent goroutines under `-race`.

## Approach 2: `context.WithCancel` with a bridge goroutine (alternative)

A genuinely different cancellation primitive: instead of a raw `chan struct{}` you control directly, use a `context.Context` and hand each replica `ctx.Done()`.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Replica func(done <-chan struct{}) (string, error)

type replicaResult struct {
	value string
	err   error
}

// FetchFastest calls every replica concurrently (one goroutine each)
// and returns the value and error from whichever one sends on its own
// result channel FIRST - later stragglers are ignored. If done is
// closed before any replica has responded, FetchFastest returns early
// with an error and no winner.
//
// This version cancels the losers via a context.Context instead of a
// plain channel. Since Replica takes a <-chan struct{} rather than a
// context.Context, ctx.Done() is handed to each replica directly (it
// already has exactly that type), and a small bridge goroutine
// forwards the caller's done into ctx's cancellation.
func FetchFastest(done <-chan struct{}, replicas ...Replica) (string, error) {
	if len(replicas) == 0 {
		return "", errors.New("fetchfastest: no replicas provided")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // covers both return paths below, same as Approach 1's defer close(stop)

	// Bridge: turn the caller-supplied done into ctx cancellation.
	// This goroutine always exits on its own once FetchFastest
	// returns - either done fired (the case it's waiting on) or
	// cancel() fired first (ctx.Done() case), so it never leaks.
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()

	results := make(chan replicaResult, len(replicas))

	for _, replica := range replicas {
		go func(r Replica) {
			value, err := r(ctx.Done())
			results <- replicaResult{value: value, err: err}
		}(replica)
	}

	select {
	case res := <-results:
		return res.value, res.err
	case <-done:
		return "", errors.New("fetchfastest: cancelled before any replica responded")
	}
}

func main() {
	replicas := []Replica{
		NewMockReplica("replica-A", 150*time.Millisecond).Replica,
		NewMockReplica("replica-B", 10*time.Millisecond).Replica,
		NewMockReplica("replica-C", 300*time.Millisecond).Replica,
	}

	done := make(chan struct{})
	start := time.Now()

	value, err := FetchFastest(done, replicas...)

	fmt.Printf("winner after %s: value=%q err=%v\n", time.Since(start), value, err)
	fmt.Println("...and replica-A and replica-C were told to stop as soon as replica-B won,")
	fmt.Println("instead of running to completion in the background for nothing.")

	time.Sleep(20 * time.Millisecond)
}
```

Design notes and honest tradeoffs versus Approach 1:

- **`ctx.Done()` already has type `<-chan struct{}`**, which is exactly the type `Replica` expects — so replicas plug in with zero adaptation. What context buys you elsewhere (deadlines, `context.WithTimeout`, propagating request-scoped values, wiring straight into an HTTP client via `req.WithContext`) is real, but nothing here actually uses any of it: this version calls `context.WithCancel`, never `WithTimeout` or `WithValue`.
- **The cost of using context here is the bridge goroutine.** `FetchFastest`'s own cancellation source is the caller's `done chan struct{}` — a plain channel, not a context — because that's what the exercise's signature requires. Since `context.Context` has no "cancel me when this arbitrary channel closes" constructor, the only way to make the caller's `done` also cancel `ctx` is a dedicated goroutine whose entire job is `select { case <-done: cancel() case <-ctx.Done(): }`. That's an extra goroutine, doing nothing but translating one cancellation shape into another, that Approach 1 simply doesn't need — there, the caller's `done` and the replicas' `stop` both feed the same `select` in `FetchFastest` directly, no bridge required.
- **Verifying the bridge goroutine doesn't leak is its own small proof obligation.** It has to exit on both of `FetchFastest`'s return paths: if a replica wins, `defer cancel()` fires, satisfying the goroutine's `case <-ctx.Done()`; if the caller's `done` fires first, the goroutine's own `case <-done` fires and calls `cancel()` (redundantly making the deferred `cancel()` a no-op, which is fine — `cancel` is idempotent). Either way it exits before `FetchFastest` returns to its caller. This is exactly the kind of extra invariant that context buys you nothing for here — Approach 1's `stop` needs no analogous proof because there's no bridge to have one.
- **When it's worth it:** if `Replica` took a `context.Context` instead of a raw channel — e.g. because replicas were real network calls behind an HTTP client or gRPC stub that already accept a `Context` for deadline propagation — `context.WithCancel` (or `WithTimeout`) would be the natural, idiomatic choice with no bridge needed, since the caller's context would just *be* the cancellation source, no translation goroutine required. Given the signature as specified (`done <-chan struct{}`), Approach 1's shared channel is the simpler, more direct fit; reach for context only if the surrounding code already speaks in contexts.

One naive variant worth naming even though it isn't a full second approach: giving each replica its **own** `done` channel (a `[]chan struct{}` sized to `len(replicas)`, closed one by one in a loop once there's a winner) behaves identically to a single shared `stop` but needs more state and more code to get there — a shared channel already means "everyone stop," so a per-replica channel buys nothing except a slice to manage and a loop to get right.

**Verified**: same scratch-directory protocol as Approach 1 — dropped this version into the same throwaway copy in place of the `defer close(stop)` version, confirmed `go build ./...` and `go vet ./...` are clean, and ran `go test -race -count=10 ./...`, which passed with zero flakes across all five tests, including the two `synctest`-driven cancellation tests, the winner-error test, and the 20-goroutine concurrent-safety test.

## Key takeaways

- A `Replica`'s own `done` and the caller's `done` are two different cancellation *sources* that both need to reach the same place: every in-flight replica goroutine. The naive bug isn't really "forgot to cancel" — it's "handed replicas a channel (`nil`) that can never signal anything at all," which is a strict subset of a working implementation, not an alternative one.
- `defer close(stop)` (or `defer cancel()` for the context version) is the cheapest correct way to make "however we leave this function, tell everyone still running to stop" true on every return path, including ones added later — there's no `if`/`else` to keep in sync.
- Buffering the results-collection channel to `len(replicas)` (or making each send itself `select`-cancellable) is not a minor tuning detail — it's the difference between "the losers stop *and* actually exit" and "the losers stop looping but now block forever on a send nobody will ever receive," which is the same leak with a different cause. `-race` won't catch this; only checking that goroutines actually terminate (or, as here, that `testing/synctest` doesn't deadlock/panic on a still-blocked goroutine) will.
- Prefer the primitive that matches what the signature already speaks in: `Replica` takes a raw `<-chan struct{}`, so a raw `chan struct{}` you control is the direct fit; reach for `context.Context` only when you actually want its extra features (deadlines, values, idiomatic propagation into a call that already accepts a `Context`) or the surrounding code already hands you one — otherwise it's an extra goroutine spent converting one cancellation shape into another for no functional gain.
