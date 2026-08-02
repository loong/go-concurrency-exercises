# Healing Unhealthy Goroutines: A Steward That Restarts a Wedged Ward - Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `29-healing-goroutines/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a `NewSteward` that is supposed to watch a "ward" goroutine's heartbeat and transparently restart it if it ever wedges — hangs or deadlocks, stops pulsing forever, and stops honoring its `done` channel too (see `mockward.go`'s `MockWard`, which simulates exactly that). Instead, it does nothing of the sort:

```go
func NewSteward(timeout time.Duration, ward StartGoroutineFn) StartGoroutineFn {
	return func(done <-chan struct{}, pulseInterval time.Duration) <-chan struct{} {
		return ward(done, pulseInterval)
	}
}
```

`NewSteward` must keep this exact signature — it returns a `StartGoroutineFn`, the same shape as the thing it watches, so stewards compose:

```go
type StartGoroutineFn func(done <-chan struct{}, pulseInterval time.Duration) (heartbeat <-chan struct{})
func NewSteward(timeout time.Duration, ward StartGoroutineFn) StartGoroutineFn
```

When the returned function is called with `(done, pulseInterval)`, it must:

- Start a ward generation via `ward(wardDone, pulseInterval)` with a fresh, **steward-owned** `wardDone` — never the steward's own incoming `done` passed straight through.
- Forward every pulse the current generation's heartbeat sends onto the steward's own returned heartbeat.
- Track time elapsed since the last forwarded pulse. If more than `timeout` elapses with no pulse, close the current generation's `wardDone` (best effort — a truly wedged ward may never honor it, and the steward must not block waiting for it to) and immediately start a brand-new generation with a brand-new `wardDone`, continuing to forward its pulses.
- If the steward's own incoming `done` is closed, close whatever the current generation's `wardDone` is and stop everything, including no longer sending on its own heartbeat.

## Why the naive version is wrong

`NewSteward`'s returned function is a pure pass-through: it hands the ward its *own* incoming `done` instead of a steward-owned `wardDone`, and returns the ward's heartbeat channel directly as its own. There is no timeout tracking, no "time since last pulse" bookkeeping, and no restart logic anywhere. The moment the one ward generation it started wedges, the heartbeat channel it handed back goes silent right along with it, forever — exactly the single point of failure a steward exists to eliminate.

Verified: running the current `check_test.go` against this naive `main.go` (both in the repo copy itself, and again after copying into a throwaway scratch directory) fails two tests, for the right reasons:

```
=== RUN   TestStewardRestartsWedgedWard
    check_test.go:47: timed out after only 5 pulse(s); an unsupervised ward would already have gone silent for good after 5 pulses (~500ms) - the steward does not appear to be restarting it
--- FAIL: TestStewardRestartsWedgedWard (0.00s)
=== RUN   TestStewardUsesFreshPerGenerationDone
    check_test.go:114: timed out waiting for 3 ward generations to start; only 1 started
--- FAIL: TestStewardUsesFreshPerGenerationDone (0.00s)
=== RUN   TestStewardStopsOnDone
--- PASS: TestStewardStopsOnDone (0.00s)
FAIL
FAIL	github.com/loong/go-concurrency-exercises/29-healing-goroutines	0.426s
FAIL
```

`TestStewardRestartsWedgedWard` and `TestStewardUsesFreshPerGenerationDone` both fail because the naive version never starts a second generation at all — it has no restart logic, so there's nothing for either test to observe past the first (pass-through) generation. `TestStewardStopsOnDone` passes even against the naive version — it only exercises a *healthy* ward (`pulsesBeforeWedge = 1000`, effectively never wedging) that also never restarts, and a pure pass-through steward correctly stops forwarding once its own `done` is closed, because closing `done` propagates straight through to the one ward generation it started. That test genuinely needs the restart machinery to fail; it doesn't need a restart, so it passes regardless. `-race` on the naive version shows the same two clean failures, with no hang and no panic — it fails cleanly on the assertions, not on a deadlock.

## Approach: one monitoring goroutine, per-generation steward-owned `wardDone`

```go
package main

import (
	"fmt"
	"time"
)

// StartGoroutineFn is the shape shared by every goroutine that can be
// launched, monitored and restarted this way: given a done channel to
// signal it should stop, and how often it should pulse, it starts and
// returns a heartbeat channel that it sends on roughly every
// pulseInterval for as long as it's alive.
type StartGoroutineFn func(done <-chan struct{}, pulseInterval time.Duration) (heartbeat <-chan struct{})

// NewSteward wraps ward with monitoring: it returns a StartGoroutineFn
// with the exact same shape as ward itself, so stewards compose. The
// returned function restarts ward with a fresh generation whenever
// more than timeout elapses without a pulse from the current
// generation.
func NewSteward(timeout time.Duration, ward StartGoroutineFn) StartGoroutineFn {
	return func(done <-chan struct{}, pulseInterval time.Duration) <-chan struct{} {
		heartbeat := make(chan struct{})

		go func() {
			defer close(heartbeat)

			// startGeneration starts a brand-new, steward-owned
			// generation of the ward and returns its wardDone (so it
			// can later be closed to ask this generation to stop) and
			// its heartbeat (so pulses can be forwarded).
			startGeneration := func() (wardDone chan struct{}, wardHeartbeat <-chan struct{}) {
				wardDone = make(chan struct{})
				wardHeartbeat = ward(wardDone, pulseInterval)
				return wardDone, wardHeartbeat
			}

			// newTimeout returns a fresh timer channel armed for
			// `timeout` from now. Creating a new one on every reset
			// (rather than reusing a single time.Timer via Reset)
			// sidesteps the classic Timer.Reset race, where a timer
			// that has already fired but whose value hasn't been
			// drained yet would fire again immediately after being
			// "reset".
			newTimeout := func() <-chan time.Time {
				return time.After(timeout)
			}

			wardDone, wardHeartbeat := startGeneration()
			timeoutC := newTimeout()

			for {
				select {
				case <-done:
					// The steward itself is being asked to stop. Best
					// effort: tell the current generation to stop too.
					// A truly wedged ward may never honor this, but
					// closing a channel never blocks, so we don't wait
					// around to find out - we just stop forwarding and
					// return, which (via the defer above) also closes
					// our own heartbeat.
					close(wardDone)
					return

				case _, ok := <-wardHeartbeat:
					if !ok {
						// The current generation's heartbeat channel
						// was closed rather than just going silent.
						// A closed channel is permanently ready to
						// receive, so if we just `continue`d here
						// this case would fire on every single loop
						// iteration - a hot spin that (under
						// synctest) also starves the timeout case of
						// ever winning a select, since a runnable
						// goroutine holds the fake clock back from
						// advancing. Disable the case instead by
						// nilling the channel (receiving from a nil
						// channel blocks forever) and let the timeout
						// path restart the generation.
						wardHeartbeat = nil
						continue
					}

					select {
					case heartbeat <- struct{}{}:
					case <-done:
						close(wardDone)
						return
					}
					timeoutC = newTimeout()

				case <-timeoutC:
					// No pulse from the current generation within
					// timeout: it's presumed wedged. Ask it to stop
					// (best effort, non-blocking - close never
					// blocks) and immediately start a fresh
					// generation, without waiting to see whether the
					// old one ever actually exits.
					close(wardDone)
					wardDone, wardHeartbeat = startGeneration()
					timeoutC = newTimeout()
				}
			}
		}()

		return heartbeat
	}
}

func main() {
	const (
		pulseInterval     = 50 * time.Millisecond
		timeout           = 200 * time.Millisecond
		pulsesBeforeWedge = 4
	)

	ward := NewMockWard(pulsesBeforeWedge)
	steward := NewSteward(timeout, ward.Start)

	done := make(chan struct{})
	heartbeat := steward(done, pulseInterval)

	for i := 1; i <= 10; i++ {
		select {
		case <-heartbeat:
			fmt.Println("pulse", i)
		case <-time.After(pulseInterval * 4):
			fmt.Println("no pulse this round")
		}
	}

	close(done)
	fmt.Println("ward generations started:", ward.Generations())
}
```

`main` here is byte-identical to the scaffold's `main` — the entire fix lives in `NewSteward`. Its "no pulse this round" print is deliberately neutral wording that stays true whether or not `NewSteward` has been fixed: against the naive version it prints on every iteration after the first wedge, forever; against the fixed `NewSteward` it marks only the brief gap while a stalled generation is being replaced (the loop still receives 10 pulses total across two-plus generations, instead of going silent for good after the first 4) — the final `ward generations started:` line is what actually distinguishes the two.

### Design notes

- **One monitoring goroutine owns all the mutable state.** `wardDone`, `wardHeartbeat`, and `timeoutC` are all plain local variables read and written only inside that single goroutine's `for`/`select` loop — there is no shared state between it and the caller, so no mutex is needed anywhere in `NewSteward`. This is the same reasoning that makes the classic "or-done channel" and pipeline-stage patterns single-goroutine-owned: a select loop is naturally a serial state machine, and serial code doesn't need locks.
- **The invariant that makes this correct: at every point in the loop, `wardDone`/`wardHeartbeat` name exactly one live generation, and closing the old `wardDone` always happens *before* a new generation is started.** That ordering means there is never a moment where two generations are both being awaited on `wardHeartbeat` — the variable is simply reassigned, dropping the reference to the old heartbeat. If the old generation truly is wedged, its goroutine just keeps running abandoned in the background with nobody listening — which is exactly the "don't block waiting for it to exit" requirement made concrete: the steward abandons it, it doesn't wait on it.
- **`close(wardDone)` is always non-blocking**, which is precisely why it's the right primitive for "ask a possibly-wedged goroutine to stop without waiting to find out if it listened." Closing a channel is guaranteed to return immediately regardless of what's on the other end, unlike sending a value, which could block forever against a wedged reader.
- **Why `time.After` and not `time.Timer.Reset`.** A single `time.Timer`, `Reset` after every pulse, is the more "obvious" way to implement a per-generation deadline, and it's what a naive port of this pattern often reaches for first. It's subtly wrong here: `Reset` on a timer that has *already fired* but whose value hasn't been drained from its channel yet does not clear that pending value — the next `select` would immediately re-fire on the stale old timeout, even though the timer was just "reset" to a fresh interval. Since this loop doesn't drain `timeoutC` on every iteration (only the `<-timeoutC` case does), there's no way to guarantee `timeoutC` is always drained right before a `Reset`. Allocating a fresh `time.After(timeout)` on every pulse and on every restart sidesteps the issue entirely, at the cost of one extra timer allocation per pulse — for a heartbeat-frequency operation (not a hot loop), that's the right trade.
- **The nil-channel disable for a closed `wardHeartbeat`.** `mockward.go`'s `MockWard` never closes its heartbeat channel (a wedged generation just goes silent, as the comment there explains: closing it would look like an infinite stream of pulses to a reader, the opposite of a deadlock), so this branch is never exercised by the test suite as written. But `StartGoroutineFn` as a general contract doesn't forbid a well-behaved ward from closing its heartbeat on a graceful exit, and a receive on an already-closed channel succeeds *every single time* without blocking. An earlier draft of this solution `continue`d straight through on `ok == false` — that turns the case into a hot spin, selected on every loop iteration. Under `synctest` that's worse than wasted CPU: a goroutine that's always immediately runnable holds the fake clock back from ever advancing, so `timeoutC` would never get a chance to fire and the test would hang inside the `synctest.Test` bubble instead of failing fast. Setting `wardHeartbeat = nil` disables that case (a receive on a nil channel blocks forever), leaving the `timeoutC` case to do the actual restart once the deadline elapses — the standard idiom for turning off one arm of a `select`.
- **Blocking forward, not a non-blocking `default:` drop.** The inner `select { case heartbeat <- struct{}{}: case <-done: ...}` blocks until either the steward's own reader consumes the pulse, or the steward is told to stop — it never drops a pulse silently. This is a deliberate choice, not the only reasonable one: the book's original presentation of this pattern (Katherine Cox-Buday, *Concurrency in Go*, "Healing Unhealthy Goroutines") forwards with a non-blocking send behind a `default:` case instead, trading "never drop a pulse" for "the monitor loop is never stalled waiting on a slow consumer, so stall detection stays live even if nobody's reading right now." The spec here says to forward *every* pulse the current generation's heartbeat sends, which reads most naturally as the blocking version, and the tests read continuously in a loop so the trade-off never bites in practice — but it is a real trade-off: against a genuinely slow or absent reader, this implementation's timeout clock effectively pauses (neither `wardHeartbeat` nor `timeoutC` is being watched while blocked on the send), so a wedged ward wouldn't get detected any faster than the consumer catches up. If a consumer that might legitimately fall behind were a real requirement, the buffered non-blocking variant would be the better fit.
- **Why the test suite can catch "restarts correctly, but reuses `done` as every generation's `wardDone`" as a distinct bug.** `MockWard` exits its generation's goroutine autonomously the moment it wedges (it just `return`s; it never waits to observe `wardDone` being closed), so a subtly-wrong steward that restarts right on schedule but reuses the caller's own `done` for every generation's `wardDone` would still make `TestStewardRestartsWedgedWard` pass — pulses keep flowing and `Generations()` still climbs past 1, regardless of what `wardDone` value each generation actually got. `mockward.go` closes that gap with `Dones() []<-chan struct{}`, which records the exact `done` channel each generation was started with, in order. `TestStewardUsesFreshPerGenerationDone` uses it to assert directly that no generation's recorded `done` equals the steward's own incoming `done`, and that no two generations share the same `done` value — catching the reused-`done` bug even though `MockWard` itself never reacts differently based on what `wardDone` it was given.
- **Why `TestStewardStopsOnDone` also asserts `Generations() == 1` on a healthy ward.** A steward that restarts on a fixed cadence (e.g. every `timeout`, via a `time.Ticker` that's never reset on a pulse) instead of genuinely tracking time-since-last-pulse would still pass `TestStewardRestartsWedgedWard` — a wedged ward gets restarted on schedule either way — and used to pass `TestStewardStopsOnDone` too, since that test only read a handful of pulses before closing `done` and never checked how many generations had run. `TestStewardStopsOnDone` now reads 40 pulses (four full `timeout` periods) from a ward that never stalls and asserts `Generations() == 1` throughout: a correctly-implemented steward resets its "time since last pulse" clock on every forwarded pulse and so never restarts a healthy ward, while a fixed-cadence steward would already show more than one generation well before the 40th pulse.

### Alternative worth naming: composing `wardDone` with `done` via `or()`

The book's own implementation of this pattern doesn't close `wardDone` explicitly on the `done`-closed path; instead it starts every generation with `or(wardDone, done)` — an "or-done channel" that closes the moment *either* input channel closes — so a single incoming `done` closing automatically tears down whatever generation is currently running, with no separate `close(wardDone)` call needed on that path.

This exercise's spec is explicit that the steward itself must close the current generation's `wardDone` when its own `done` is closed, rather than relying on composition to do it implicitly — which is what this solution does instead. Both approaches produce the same externally observable behavior for a direct steward; the `or()` version additionally makes the composed-steward case ("a steward watching a steward") self-cleaning without an extra explicit close in the outer steward's own `done` case, at the cost of needing readers to understand the `or()` idiom and pulling in one more helper. This solution took the simpler, spec-literal one: an explicit `close(wardDone)` sitting right there in the `done` case, no extra composition helper required.

## Verified

Confirmed the naive `main.go` fails first — both in the repo's own `29-healing-goroutines/` and in a scratch copy of it — with `go test -v ./...` producing the two failures shown above (`TestStewardRestartsWedgedWard` and `TestStewardUsesFreshPerGenerationDone`; `TestStewardStopsOnDone` passing, as expected, since it never exercises a wedge or a restart), and `go test -race -v ./...` showing the identical two clean failures with no hang, no panic, and no race.

Copied the exercise into a throwaway scratch directory (with its own standalone `go.mod`, `go 1.25.6`) and dropped this `NewSteward` in. `go build ./...`, `go vet ./...`, and `gofmt -l .` are all clean. `go test -race -count=5 ./...` passed with zero flakes across all three tests (`TestStewardRestartsWedgedWard`, `TestStewardUsesFreshPerGenerationDone`, `TestStewardStopsOnDone`); while writing up the design notes, the nil-channel-disable bug described above was found and fixed (an initial `continue`-only version on a closed `wardHeartbeat` would hot-spin and hang under `synctest` — not exercised by `MockWard`, so `go test -race -count=5` alone had not caught it). After the fix, re-ran `go test -race -count=20 ./...` — 20 repetitions, zero flakes.

Also verified the two specific gaps these tests exist to close, using two deliberately-wrong hand-written variants (not committed anywhere): a steward with fully correct restart-on-stall logic that reuses the caller's own `done` as every generation's `wardDone` instead of minting a fresh one fails only `TestStewardUsesFreshPerGenerationDone`, cleanly, on the "started with the steward's own incoming done" message; and a steward that restarts on a fixed cadence (a `time.Ticker` on `timeout`, never reset on a pulse) instead of tracking time-since-last-pulse fails only `TestStewardStopsOnDone`'s `Generations() == 1` assertion. Both confirm the new tests catch exactly the bug they're meant to.

`go run .` was also run manually against the fixed `NewSteward` and printed pulses continuing past the point the first generation wedges, ending with a `ward generations started:` count of more than one (the exact count is timing-dependent since `main` runs on the real clock, not fake time — typically 2-3 for the constants in the scaffold) — confirming an actual restart rather than a fluke of timing.

The real repo's `29-healing-goroutines/main.go` was left untouched as the original naive version throughout — every fix iteration happened only in the scratch copy.
