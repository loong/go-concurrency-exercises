# Or-Channel Combinator: Combining Independent Shutdown Triggers — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `15-or-channel-combinator/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

A service often needs to shut down as soon as ANY of several independent triggers fires — a failed health check, an admin-requested shutdown, a deadline expiring. Each trigger is its own `<-chan struct{}` that gets closed the moment its condition occurs. `or` is supposed to combine an arbitrary, variadic number of such signal channels into a single channel that closes as soon as ANY ONE of the inputs closes — no matter which one, and no matter how many channels there are (including 0 or 1).

The naive implementation only ever watches `channels[0]`:

```go
func or(channels ...<-chan struct{}) <-chan struct{} {
	orDone := make(chan struct{})
	if len(channels) == 0 {
		return orDone
	}
	go func() {
		defer close(orDone)
		<-channels[0]
	}()
	return orDone
}
```

## Why the naive version is wrong

Closing any channel *other than* `channels[0]` has no effect at all — the combined channel just sits there, blocked forever, even though one of the shutdown triggers already fired. `check_test.go`'s key test, `TestOrFiresOnAnyChannel`, closes `chans[3]` out of 5 and expects `or()` to unblock promptly; `TestOrHandlesManyChannels` does the same with 20 channels, closing index 12.

Verified against the naive version in a scratch copy:

```
--- FAIL: TestOrFiresOnAnyChannel (0.30s)
    check_test.go:69: or() did not fire within 300ms after channels[3] closed - looks like or() isn't watching every channel it was given (only channels[0]?)
--- FAIL: TestOrHandlesManyChannels (0.30s)
    check_test.go:131: or() with 20 channels did not fire within 300ms after channels[12] closed
```

Both failures are clean timeouts (the tests guard their blocking receive with `select` + `time.After(300ms)`), not panics — which matters here specifically because these two tests deliberately run on the real clock instead of inside `synctest.Test`. Inside a `synctest` bubble, a goroutine that never unblocks and has no fake time left to advance makes every goroutine in the bubble "durably blocked," which `synctest` reports as a fatal deadlock panic that would crash the whole test binary rather than failing the test cleanly. `TestOrFiresOnFirstChannel` (closes `channels[0]`) and `TestOrHandlesEdgeCases` (0 and 1 channels) pass even against the naive code — they exercise exactly the one case the buggy implementation accidentally gets right, plus edges it doesn't touch at all.

## Approach 1: recursive divide-and-conquer

The classic idiom: watch `channels[0]` and `channels[1]` directly in a `select`, and fold everything else into a single recursive third branch.

```go
package main

import (
	"fmt"
	"time"
)

// or combines any number of done/signal channels into a single
// channel that closes as soon as ANY of the input channels closes.
func or(channels ...<-chan struct{}) <-chan struct{} {
	orDone := make(chan struct{})

	switch len(channels) {
	case 0:
		return orDone
	case 1:
		// Nothing to combine it with: hand the caller's own channel
		// back unchanged. No new goroutine needed - and spawning one
		// here that just relays a single, possibly-never-closed
		// channel would leak forever once the rest of the select
		// chain above it has already fired on some other branch.
		return channels[0]
	}

	go func() {
		defer close(orDone)

		// Thread this level's own orDone into the recursive tail
		// alongside the channels we haven't looked at yet. That way,
		// if THIS select fires because of channels[0] or channels[1],
		// closing orDone (via the defer above) also unblocks whatever
		// goroutine the recursive or() call below spawned - instead
		// of leaving it orphaned forever, still waiting on channels
		// further down the list that may never close.
		tail := make([]<-chan struct{}, 0, len(channels)-1)
		tail = append(tail, channels[2:]...)
		tail = append(tail, orDone)

		select {
		case <-channels[0]:
		case <-channels[1]:
		case <-or(tail...):
		}
	}()

	return orDone
}

func main() {
	healthCheckFailed := make(chan struct{})
	adminShutdown := make(chan struct{})
	deadlineExceeded := make(chan struct{})

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(healthCheckFailed)
	}()

	start := time.Now()
	combined := or(healthCheckFailed, adminShutdown, deadlineExceeded)
	<-combined
	elapsed := time.Since(start)

	fmt.Printf("shutdown signal received after %s (triggered by healthCheckFailed)\n", elapsed)
}
```

Why the two base cases matter, not just the recursive step:

- **Zero channels**: nothing to react to. Returning a channel that's never closed is the only sane behavior — there is no trigger to wait for, so the function must not fabricate one that fires on its own.
- **One channel**: return it directly, don't wrap it in a relay goroutine. A relay (`go func() { defer close(orDone); <-channels[0]; }()`) looks harmless, but it has no way to know when to give up — if that single channel is one of several independent triggers that *doesn't* end up firing (completely normal — only one trigger fires in practice), the relay goroutine blocks on it forever. `TestOrHandlesEdgeCases`'s "one channel" subtest exercises exactly this path.

The trap in the recursive case, spelled out: if you recurse on plain `or(channels[2:]...)`, and *this* level's `select` happens to fire because of `channels[0]` or `channels[1]` — not the recursive branch — the goroutine spawned by that recursive call is left behind. It's still blocked in its own `select`, waiting on channels further down the list that may never close, forever. That's a goroutine leak on *every single call* where the winning channel isn't in the deepest recursion level — which, for a signal fired somewhere in the middle of a long channel list, is the common case, not a corner case.

The fix is to fold this level's own `orDone` into what gets recursed on: `or(append(channels[2:], orDone)...)`. Since `orDone` is closed via `defer` no matter which branch of *this* `select` fires, closing it also unblocks the recursive call's `select` (via its own `<-or(tail...)` branch, which bottoms out on this `orDone`), so that goroutine gets to return and exit instead of being orphaned. The same reasoning applies transitively at every level of the recursion, all the way down — each level's shutdown collapses the ones below it.

**Verified** in a scratch copy of `15-or-channel-combinator/` (never modifying the live repo directory): the naive `main.go` fails `TestOrFiresOnAnyChannel` and `TestOrHandlesManyChannels` with clean timeouts (not panics) as shown above, while `TestOrFiresOnFirstChannel` and `TestOrHandlesEdgeCases` already pass. Swapping in the solution above, `go test -race -count=5 ./...` passes cleanly, 5/5 runs, no data races, covering 0, 1, and many (20) channels.

## Approach 2: `reflect.Select` over a single flat case list

A genuinely different design for the "more than one channel" case: instead of building a recursive tree of `select`s (and a matching tree of goroutines, one per recursion level), construct one `[]reflect.SelectCase` covering *all* N channels up front, and let a single goroutine block on all of them at once via `reflect.Select`.

```go
package main

import (
	"fmt"
	"reflect"
	"time"
)

// or combines any number of done/signal channels into a single
// channel that closes as soon as ANY of the input channels closes.
//
// This version builds one flat reflect.Select over all N channels at
// once instead of recursing: a single goroutine blocks on all of them
// simultaneously and returns as soon as any one becomes ready.
func or(channels ...<-chan struct{}) <-chan struct{} {
	orDone := make(chan struct{})

	switch len(channels) {
	case 0:
		return orDone
	case 1:
		return channels[0]
	}

	cases := make([]reflect.SelectCase, len(channels))
	for i, c := range channels {
		cases[i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(c),
		}
	}

	go func() {
		defer close(orDone)
		reflect.Select(cases)
	}()

	return orDone
}

func main() {
	healthCheckFailed := make(chan struct{})
	adminShutdown := make(chan struct{})
	deadlineExceeded := make(chan struct{})

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(healthCheckFailed)
	}()

	start := time.Now()
	combined := or(healthCheckFailed, adminShutdown, deadlineExceeded)
	<-combined
	elapsed := time.Since(start)

	fmt.Printf("shutdown signal received after %s (triggered by healthCheckFailed)\n", elapsed)
}
```

The two base cases (0 and 1 channels) are identical to Approach 1 for the same reasons given above — those aren't where the two approaches diverge.

Where they genuinely differ, for N > 1 channels:

- **Goroutine count.** Approach 1 spawns one goroutine per recursion level — roughly N-1 goroutines for N channels, forming a tree, each blocked in its own two-channel-plus-recursive-tail `select`. Approach 2 spawns exactly **one** goroutine total, regardless of N, blocked in a single `reflect.Select` over a flat, N-case list. There's no recursive tail to worry about leaking, because there's no recursion — the orphaned-goroutine trap Approach 1 has to explicitly work around doesn't exist here in the first place.
- **Shape of the wait.** Approach 1 is an O(log N)-ish (in practice closer to O(N), since it peels off 2 channels per level) *tree* of two-way selects; Approach 2 is a single O(N) *flat* fan-in built once via reflection.
- **Cost tradeoff.** `reflect.Select` has to build a `[]reflect.SelectCase` (one allocation-heavy value per channel) and pay reflection overhead on every call, which a hand-written `select` with a compile-time-fixed number of cases doesn't. In exchange, it avoids the recursive goroutine tree and the leak trap entirely, and it's arguably simpler to read for "watch all of these at once" since there's no recursion to reason about.

Both are correct; which one to reach for is a call between "idiomatic Go, no reflection, but must get the recursive-leak trick right" (Approach 1) and "no recursion or leak trap to think about, at the cost of reflection overhead and losing compile-time channel-type checking" (Approach 2).

**Verified** in the same scratch copy, swapping Approach 1's `main.go` for the `reflect.Select` version above: `go test -race -count=10 ./...` passes cleanly, 10/10 runs, no data races — including the 0-channel, 1-channel, and 20-channel (`TestOrHandlesManyChannels`) cases.

## Key takeaways

- Watching only `channels[0]` is a common naive shape for variadic "combine these signals" helpers — it happens to pass any test that only ever closes the first channel, which is why `TestOrFiresOnFirstChannel` alone wouldn't have caught this bug.
- The 0-channel and 1-channel base cases aren't just tidiness — the 1-channel case in particular has a real trap: wrapping a single passthrough channel in a relay goroutine leaks that goroutine forever whenever the channel it watches is never closed, which is the normal case for one of several independent triggers.
- The classic recursive `or` idiom has its own trap: recursing on `or(channels[2:]...)` unmodified orphans a goroutine every time the *current* level's select wins on `channels[0]`/`channels[1]` instead of the recursive branch. Folding this level's own `orDone` into the recursive tail (`or(append(channels[2:], orDone)...)`) closes that goroutine down as a side effect of this level shutting down, at every level of the recursion.
- `reflect.Select` offers a genuinely different structural tradeoff for the same problem: one goroutine and one flat N-way select instead of a goroutine-per-level recursive tree — no leak trap to get right, at the cost of reflection overhead and an allocation-heavy `[]reflect.SelectCase` built on every call.
- Tests that must observe a *hang* (not just a value) can't safely run inside `testing/synctest`'s fake-clock bubble: a goroutine that's supposed to never unblock makes the whole bubble "durably blocked," which `synctest` treats as a fatal deadlock and panics instead of letting the test fail normally. That's why `TestOrFiresOnAnyChannel` and `TestOrHandlesManyChannels` here use the real clock with `select` + `time.After` instead.
