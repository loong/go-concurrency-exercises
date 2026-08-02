# Or-Done Channel: Stopping a Long-Lived Monitoring Feed Cleanly — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `07-or-done-channel/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`StartMetricStream` (in `mockmonitor.go`) simulates a long-lived feed:
it spawns a goroutine that sleeps `MetricInterval` (20ms), sends the
next counter value, and repeats — forever. It never closes its output
channel, because a real feed like this (a metrics websocket, a tailed
log) has no way to know when the consumer has walked away.

`orDone` is supposed to sit between that feed and a consumer, letting
the consumer opt out early via a `done` channel:

```go
func orDone(done <-chan struct{}, c <-chan int) <-chan int
```

The given implementation is a no-op passthrough:

```go
func orDone(done <-chan struct{}, c <-chan int) <-chan int {
	return c
}
```

so closing `done` has zero effect — the returned channel *is* the
input channel, unconditionally.

## Why the naive version is wrong

Because `orDone` just hands back `c` directly, `done` is never
consulted anywhere in the pipeline. There's no forwarding goroutine to
select on it, and no way for the consumer to signal "stop" that
`orDone` itself would notice.

The tests make this concrete. `orDoneStopsPromptly` wires `orDone` to
a channel `c` that is *never written to* — so any unconditional read
on the wrapped channel blocks forever unless `orDone` is itself
watching `done`. Against the passthrough, `out` literally is `c`:
closing `done` does nothing to it, and the read hangs until the test's
100ms safety-net timeout fires:

```
--- FAIL: TestOrDoneStopsPromptlyOnDone (0.10s)
    check_test.go:85: orDone did not stop promptly after done was closed -
    the forwarding goroutine appears to be leaked, blocked forever on a
    receive/send that will never happen
```

That's the leak the exercise is about: without `orDone` actively
forwarding through a `select`, there's no goroutine anywhere in the
pipeline that can notice `done` closing and react to it.

## Approach 1: Forwarding goroutine with a double `select`

The fix spawns exactly one goroutine that ranges over `c`, forwarding
each value onto a fresh `out` channel, and uses `select` at *both*
points where it could otherwise block forever:

```go
func orDone(done <-chan struct{}, c <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for {
			select {
			case <-done:
				return
			case v, ok := <-c:
				if !ok {
					return
				}
				select {
				case out <- v:
				case <-done:
					return
				}
			}
		}
	}()

	return out
}
```

Why two selects and not one:

- **Receiving from `c`** can block forever if the upstream producer
  has stalled or if nobody will ever send again. The outer `select`
  races that receive against `<-done`, so a closed `done` unblocks the
  goroutine even while it's waiting for the *next* input value.
- **Sending on `out`** can equally block forever if the consumer has
  stopped reading (which is exactly the scenario the consumer just
  triggered by closing `done`). The inner `select` races that send
  against `<-done` too, so a value already in hand doesn't wedge the
  goroutine if nobody's left to receive it.
- `defer close(out)` runs on every exit path — whether `c` closed
  naturally or `done` fired — so a `range` over `out` at the call site
  always terminates instead of hanging.

A tempting-looking but broken alternative is a single select inside a
`for range c` loop:

```go
// BROKEN: only the send is done-aware
for v := range c {
	select {
	case out <- v:
	case <-done:
		return
	}
}
```

This looks like it handles shutdown, and it even happens to forward
values correctly while `c` is producing. But `range c` itself is an
unconditional receive with no `done` awareness — if `c` has nothing to
give (exactly the `orDoneStopsPromptly` test's setup: a channel nobody
ever writes to), the loop blocks on that receive forever, `done`
closing or not. That's the same failure the naive passthrough has,
just moved one layer in. The receive needs to be done-aware, not just
the send — which is why both operations get their own `select` in the
correct version above.

Tracing through the tests against this implementation:

- `TestOrDoneForwardsValues` — while `done` stays open, the goroutine
  just receives from `c` and re-sends on `out`, so values pass through
  unchanged and in order.
- `TestOrDoneStopsPromptlyOnDone` / `TestOrDoneNoGoroutineLeakRace` —
  closing `done` unblocks whichever select the goroutine is currently
  parked in (whether it was waiting to receive from `c` or to send on
  `out`), it returns, and `close(out)` fires — so the reader on the
  other end sees `ok == false` promptly instead of hanging.

This exercise really only has one idiomatic shape (spawn a forwarder,
select on both blocking points, close `out` on the way out), so there
isn't a second, meaningfully different approach worth presenting here
— unlike exercise 6, forcing an "Approach 2" would just be a cosmetic
rewrite of the same idea.

## Key takeaways

- A channel can block on *both* ends — receiving from an empty/stalled
  channel and sending to one nobody's draining — and `done` needs to
  be raced against each one separately with its own `select`, not just
  the operation that happens to be top-of-mind (usually the send).
- `close(out)` via `defer` inside the forwarding goroutine is what
  makes the wrapped channel usable with a plain `for range` at the
  call site: every exit path (upstream closed, or `done` fired) leaves
  `out` in a well-defined, closed state instead of leaking the
  goroutine or leaving `out` open forever.
- This "orDone" pattern generalizes: any time you wrap a channel you
  don't control (can't close, can't stop the producer) with logic that
  should respect an external cancellation signal, the wrapping
  goroutine needs a `done`-aware select on every blocking channel
  operation it performs, not just the obvious one.
