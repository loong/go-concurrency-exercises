# Tee Channel: Duplicating a Sensor Stream to Two Consumers — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `14-tee-channel/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`StartSensor(count)` (see `mocksensor.go`) emits `count` incrementing integer readings, one every 5ms, then closes its channel. `Tee` is supposed to duplicate every value it reads from `in` onto two independent output channels, so two consumers — say a live display and a logger — can each observe the full, identical sequence in order, even if one of them reads much slower than the other. Closing `done` should make `Tee` abandon everything promptly and close both outputs.

The naive implementation is:

```go
func Tee(done <-chan struct{}, in <-chan int) (<-chan int, <-chan int) {
	return in, in
}
```

## Why the naive version is wrong

`out1` and `out2` here are literally the same channel as `in`. Every value sent on that channel goes to whichever of the two consumer goroutines happens to win the race to receive it — never both. `check_test.go`'s key test, `TestTeeDuplicatesToBothConsumers`, proves this directly: it runs a fast consumer (drains `out1` as fast as possible) against a slow one (`out2`, sleeping 2ms between reads) and checks both end up with the full `[0..9]` sequence.

Verified against the naive version in a scratch copy:

```
--- FAIL: TestTeeDuplicatesToBothConsumers (0.00s)
    check_test.go:77: fast consumer got [1 3 5 7 9], want [0 1 2 3 4 5 6 7 8 9] (every value must reach every consumer)
    check_test.go:80: slow consumer got [0 2 4 6 8], want [0 1 2 3 4 5 6 7 8 9] (every value must reach every consumer)
--- FAIL: TestTeeStopsOnDone (0.03s)
    check_test.go:138: expected out1 to be closed once done fires, got value 4 instead
--- FAIL: TestTeeStopsOnDoneRace (0.03s)
    check_test.go:147: expected out1 to be closed once done fires, got value 4 instead
```

The fast consumer steals the odd-indexed values, the slow one gets the even-indexed ones — each is missing exactly half the sequence, so `reflect.DeepEqual` against the expected `[0..9]` fails for both. `TestTeeStopsOnDone` fails for a related reason: since `out1`/`out2` are `in` itself, closing `done` has no effect on it at all — `in` keeps producing values from `StartSensor` regardless, so the very next read returns a real value instead of observing the channel closed.

## Approach 1: per-value goroutine, inner select with nil-out-on-send

There's really one idiomatic pattern for `Tee`, so this is the only approach shipped here — a second, genuinely different design isn't really available: you need to hold each value until *both* outputs have received it, and you need an inner loop so a slow reader on one output can't block delivery to the other (or ignore `done`).

For each value read from `in`, use two local variables that alias `out1`/`out2` and get **nilled out once that output has received the value** — sending on a nil channel blocks forever, so once an output's variable is nil it simply drops out of the `select` and can never be chosen again for this value. The inner loop keeps going until both variables are nil (both outputs have gotten the value) or `done` fires.

```go
package main

import (
	"fmt"
	"sync"
)

// Tee duplicates every value read from `in` onto two independent
// output channels, so that two consumers can each observe the full
// sequence of values `in` produces, in order, regardless of how fast
// or slow either one reads. It closes both outputs once `in` is
// exhausted or done fires.
func Tee(done <-chan struct{}, in <-chan int) (<-chan int, <-chan int) {
	out1 := make(chan int)
	out2 := make(chan int)

	go func() {
		defer close(out1)
		defer close(out2)

		for {
			var v int
			var ok bool

			select {
			case <-done:
				return
			case v, ok = <-in:
				if !ok {
					return
				}
			}

			// Send v to whichever of out1/out2 hasn't received it yet,
			// using a separate select per output so that a slow reader
			// on one output doesn't block delivery to the other, and
			// done still gets a chance to fire while we wait.
			out1Ch, out2Ch := out1, out2
			for out1Ch != nil || out2Ch != nil {
				select {
				case out1Ch <- v:
					out1Ch = nil
				case out2Ch <- v:
					out2Ch = nil
				case <-done:
					return
				}
			}
		}
	}()

	return out1, out2
}

func main() {
	done := make(chan struct{})
	defer close(done)

	in := StartSensor(10)
	out1, out2 := Tee(done, in)

	var wg sync.WaitGroup
	var fast, slow []int

	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range out1 {
			fast = append(fast, v)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range out2 {
			slow = append(slow, v)
		}
	}()

	wg.Wait()

	fmt.Println("consumer 1:", fast)
	fmt.Println("consumer 2:", slow)
}
```

Why this shape specifically:

- **One goroutine reading `in`, per-value fan-out inner loop.** Only one goroutine ever touches `in`, so there's no risk of two goroutines racing to read the same value out of `in` (which would just relocate the original bug one level down).
- **`out1Ch`/`out2Ch` locals, nilled on send, not the real `out1`/`out2`.** Nilling the *local copy* of the channel variable (not the channel itself) removes that branch from future iterations of the inner `select` without affecting the other consumer's view of the channel. A nil channel is never ready in a `select`, so once `out1Ch` is nil, the `select` only has `out2Ch <- v` and `<-done` left to consider.
- **`done` is checked in both the outer receive and the inner send loop.** Without the inner `case <-done`, a value that's already been delivered to the fast consumer but not yet to a stalled slow consumer would block the whole goroutine forever once `done` closes — exactly what `TestTeeStopsOnDone`/`TestTeeStopsOnDoneRace` are designed to catch.
- **Both `close(out1)`/`close(out2)` are deferred up front**, so every return path (input exhausted, or `done` fired mid-send) closes both outputs exactly once, letting `range` loops on either side terminate cleanly instead of leaking a goroutine or hanging a consumer.

**Verified** in a scratch copy of `14-tee-channel/` (never modifying the live repo directory): the naive `main.go` fails `TestTeeDuplicatesToBothConsumers`, `TestTeeStopsOnDone`, and `TestTeeStopsOnDoneRace` as shown above; swapping in the solution above, `go test -race -count=5 ./...` passes cleanly, 5/5 runs, no data races.

## Key takeaways

- `return in, in` isn't "duplication" — it's the same channel handed out twice, so consumers race for each value instead of each seeing every value.
- Tee needs to hold each value until *every* output has received it, with an inner `select` (not a fixed order of blocking sends) so a slow consumer on one output can't stall delivery to a faster one on the other.
- Nilling out a *local* channel variable per output, per value, is the standard trick for "send to whichever of these haven't received it yet" — a nil channel is never selectable, so it naturally drops out of the `select` once satisfied.
- `done` must be checked in the inner per-value send loop, not just the outer receive loop — otherwise a value stuck waiting on a stalled/slow consumer can hang the whole goroutine past shutdown.
