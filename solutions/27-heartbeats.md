# Heartbeats: Detecting a Stalled Worker Before It's Too Late - Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `27-heartbeats/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is `DoWork`, a simulated long-running worker, and `WorkWithTimeout`, which is supposed to use `DoWork`'s heartbeat to detect a stalled worker quickly instead of only ever being able to wait for its eventual result:

```go
func DoWork(done <-chan struct{}, pulseInterval time.Duration, workUnits int) (heartbeat <-chan struct{}, results <-chan int)
func WorkWithTimeout(workUnits int, stallAfter int, perPulseTimeout time.Duration) ([]int, error)
```

`DoWork` is supposed to process `workUnits` units of work one at a time (see `mockworker.go`'s `SimulateUnit`), sending one result per completed unit on `results` and closing `results` when the job finishes. While actively working on a unit, it must also pulse on `heartbeat` roughly every `pulseInterval`, for as long as that unit takes — that pulse is the only way a caller can tell "still working, just slow" apart from "wedged forever," since a slow-but-healthy unit and a stalled one otherwise look identical to anyone who can only wait for a result.

`WorkWithTimeout` is supposed to run `DoWork`, reset a timer to `perPulseTimeout` on every heartbeat or result received, and return a non-nil error the instant `perPulseTimeout` elapses with neither — detecting a stall within roughly one `perPulseTimeout` window of it starting, not after however long the stalled unit would otherwise have taken.

`stallAfter` (a parameter of `WorkWithTimeout`, not of `DoWork`) selects which work-unit index should simulate a stall via `mockworker.go`'s package-level `SetStallUnit`; a negative or out-of-range value means "run normally" — that's the normal-path test case.

## Why the naive version is wrong

```go
func DoWork(done <-chan struct{}, pulseInterval time.Duration, workUnits int) (heartbeat <-chan struct{}, results <-chan int) {
	hb := make(chan struct{}, 1) // buffered so the lone startup pulse below can complete even though nobody ever reads it
	res := make(chan int)

	go func() {
		defer close(res)

		select {
		case hb <- struct{}{}: // one pulse, ever, right at startup
		case <-done:
			return
		}

		checkIn := func() bool { return true } // BUG: never actually sends a heartbeat

		for i := 0; i < workUnits; i++ {
			if !SimulateUnit(done, i, pulseInterval, checkIn) {
				return
			}
			select {
			case res <- i:
			case <-done:
				return
			}
		}
	}()

	return hb, res
}
```

`DoWork` sends exactly one heartbeat, at startup, then never again — the `checkIn` callback it hands to `SimulateUnit` (exactly where the exercise wants a real, per-slice pulse) is a no-op that just returns `true`. `WorkWithTimeout` compounds the problem: it never even reads from `heartbeat`, and instead of resetting a timer on every heartbeat/result, it sets a single flat `perPulseTimeout * workUnits` deadline once, up front, and never touches it again:

```go
timedOut := time.After(perPulseTimeout * time.Duration(workUnits))
for {
	select {
	case r, ok := <-results:
		...
	case <-timedOut:
		return nil, fmt.Errorf("work timed out after %s", ...)
	}
}
```

That single flat deadline can't win either test in this suite, and the arithmetic shows why:

- `TestNoStallCompletesSuccessfully` needs the deadline to be **longer** than a full healthy job: `UnitDuration(400ms) * workUnits(5) = 2000ms`.
- `TestStallDetectedPromptly` needs the deadline to fire **no later than** `stallStartsAt(800ms) + perPulseTimeout(300ms) + slack ≈ 1300ms` once a stall begins.

The naive deadline is `perPulseTimeout(300ms) * workUnits(5) = 1500ms` — simultaneously too short to satisfy the first requirement and too long to satisfy the second. No choice of a single flat, whole-job timeout can satisfy both at once; only a timer that resets on every sign of life can.

Verified: running `go test -race -timeout 30s -v ./...` (`GOROOT` pointed at the go1.25.6 toolchain `go.mod` requires) against this naive `main.go`:

```
=== RUN   TestNoStallCompletesSuccessfully
    check_test.go:33: expected nil error for a normal run, got work timed out after 1.5s
--- FAIL: TestNoStallCompletesSuccessfully (0.00s)
=== RUN   TestStallDetectedPromptly
    check_test.go:89: did not detect the stall promptly: took 1.5s, want at most 1.3s (stall starts at 800ms + one perPulseTimeout of 300ms)
--- FAIL: TestStallDetectedPromptly (0.00s)
=== RUN   TestConcurrentSafety
    check_test.go:129: caller 0: work timed out after 900ms
    check_test.go:129: caller 1: work timed out after 900ms
    check_test.go:129: caller 2: work timed out after 900ms
    ... (all 20 callers fail identically)
    check_test.go:129: caller 19: work timed out after 900ms
--- FAIL: TestConcurrentSafety (0.90s)
FAIL
FAIL	github.com/loong/go-concurrency-exercises/27-heartbeats	1.035s
```

`TestConcurrentSafety` uses `workUnits = 3`, so its flat deadline is `300ms * 3 = 900ms` — again shorter than the real `3 * 400ms = 1200ms` job, so even the plain concurrent-safety smoke test (no stall configured at all) fails on pure spurious-timeout grounds. All three failures point at the same root cause, with clean diagnostic messages — no hangs, no `synctest` deadlock panics, no `-race` reports.

## The fix

```go
package main

import (
	"fmt"
	"time"
)

// workerPulseInterval is the fixed interval WorkWithTimeout asks
// DoWork to pulse at. It must stay shorter than any perPulseTimeout a
// caller passes in, or every call would time out before a single
// heartbeat could possibly arrive.
const workerPulseInterval = 100 * time.Millisecond

// DoWork simulates doing workUnits units of work, one at a time,
// sending one result int per completed unit on results and closing
// results when the whole job is done (or done fires early). While it
// is actively working on a unit, it also pulses on heartbeat roughly
// every pulseInterval, for as long as that unit takes.
func DoWork(done <-chan struct{}, pulseInterval time.Duration, workUnits int) (heartbeat <-chan struct{}, results <-chan int) {
	hb := make(chan struct{})
	res := make(chan int)

	go func() {
		defer close(res)

		// checkIn is SimulateUnit's "still working, got a moment to
		// report in?" callback. Every time a healthy unit calls it
		// (between pulseInterval-sized slices of its work), we try to
		// send one pulse on hb - that's what makes the heartbeat track
		// genuine progress instead of firing on a schedule of its own.
		// A stalled unit never calls checkIn at all, so hb simply goes
		// quiet for as long as the stall lasts.
		//
		// The send itself also has to respect done: if nobody is
		// listening (the caller gave up) we must not block forever on
		// a send nobody will ever receive.
		checkIn := func() bool {
			select {
			case hb <- struct{}{}:
				return true
			case <-done:
				return false
			}
		}

		for i := 0; i < workUnits; i++ {
			if !SimulateUnit(done, i, pulseInterval, checkIn) {
				return
			}

			select {
			case res <- i:
			case <-done:
				return
			}
		}
	}()

	return hb, res
}

// WorkWithTimeout runs DoWork to completion and fails fast - within
// roughly one perPulseTimeout window - the moment the worker stops
// pulsing AND stops producing results, instead of waiting for however
// long the stalled unit would otherwise have taken.
func WorkWithTimeout(workUnits int, stallAfter int, perPulseTimeout time.Duration) ([]int, error) {
	SetStallUnit(stallAfter)

	done := make(chan struct{})
	defer close(done)

	heartbeat, results := DoWork(done, workerPulseInterval, workUnits)

	collected := make([]int, 0, workUnits)

	// The stall timer is reset to perPulseTimeout every time a
	// heartbeat OR a result arrives - it only ever gets to fire if
	// BOTH channels have gone silent for a full perPulseTimeout, which
	// is exactly the definition of "stalled" we care about.
	//
	// go.mod pins this module to Go 1.25.6. As of Go 1.23, a chan-based
	// time.Timer's Reset can be called directly on a running timer: any
	// receive from t.C after Reset returns is guaranteed not to observe
	// a stale value from the previous duration, so there is no need for
	// the older Stop-then-drain-then-Reset dance that earlier Go
	// versions required.
	timer := time.NewTimer(perPulseTimeout)
	defer timer.Stop()

	for {
		select {
		case <-heartbeat:
			timer.Reset(perPulseTimeout)

		case r, ok := <-results:
			if !ok {
				return collected, nil
			}
			collected = append(collected, r)
			timer.Reset(perPulseTimeout)

		case <-timer.C:
			return nil, fmt.Errorf("worker stalled: no heartbeat or result within %s", perPulseTimeout)
		}
	}
}
```

Design notes:

- **The pulse lives where the work happens, not on an independent ticker.** `checkIn` is only ever invoked by `SimulateUnit` *between* the small time-slices a healthy unit is broken into. Crucially, `mockworker.go`'s stalled path never calls `checkIn` at all — it blocks in one single, un-checkpointed wait. That's what makes a stall actually stop the heartbeats: if pulsing were driven by a `DoWork`-level `time.Ticker` racing in the same `select` as "is the unit done yet", it would keep firing on schedule no matter what the unit was actually doing, and no implementation — correct or naive — could ever detect a stall from the caller side. Tying the pulse to genuine, cooperative progress is the whole point of the heartbeat pattern.
- **`hb` goes back to unbuffered here, on purpose.** The naive `DoWork` needed `hb` buffered (capacity 1) purely so its one startup pulse could complete against a `WorkWithTimeout` that never reads `heartbeat` at all - without that buffer, the send would block forever and the naive worker would never even reach its first unit of work. Once `WorkWithTimeout` actually selects on `heartbeat`, that crutch is unnecessary: an unbuffered channel is what makes a pulse mean "someone was listening and received this the instant it was sent," a real synchronous liveness signal, rather than a fire-and-forget write into a buffer nobody may ever drain.
- **`checkIn` respects `done` with a `select`, never a blocking bare send.** `WorkWithTimeout` always `close(done)`s via `defer` before it returns, including on the stall-detected path, while the worker goroutine may still be mid-unit. Without the `<-done` case in `checkIn` (and in the two `res <- i` sends), a heartbeat or result send with nobody left receiving would leak the worker goroutine forever, parked on a channel operation that can never complete.
- **`heartbeat` is read as a value-only receive (`case <-heartbeat:`), not `v, ok := <-heartbeat`.** `DoWork` never closes `heartbeat` — only `results` gets a `defer close`. Handling a hypothetical `!ok` branch that the implementation can never actually produce would be dead code pretending to be a real case; the plain receive says exactly what's true: "a pulse arrived," full stop.
- **A single shared `timer.Reset` replaces the naive flat deadline**, and it's why both failing tests above pass here: the timer restarts on *either* channel activity, so it never measures "how long has the whole job been running" — only "how long has it been since we last heard *anything*." A healthy job that runs long keeps resetting the timer every `workerPulseInterval` (100ms) or so; a stalled unit stops resetting it the moment the stall begins, so the timer fires almost exactly `perPulseTimeout` after the last sign of life, regardless of how long the job as a whole would otherwise take.
- **Reusing one `time.Timer` via `Reset` instead of allocating a fresh one every loop iteration is safe here specifically because of the Go version this module is pinned to.** Before Go 1.23, `Timer.Reset` on a timer that might already have fired required first calling `Stop` and draining `timer.C` if `Stop` returned `false`, or a stale tick could sit in the channel and make the next `Reset` appear to fire instantly. `go.mod` pins this module to `go 1.25.6`, well past that change, so `timer.Reset(perPulseTimeout)` alone — with no `Stop`/drain — is documented as safe: any receive from `timer.C` after `Reset` returns is guaranteed not to observe a value from the previous duration.
- **`DoWork`'s work loop and `WorkWithTimeout`'s watch loop are two separate goroutines communicating only over channels.** Neither blocks the other from making unrelated progress, and `WorkWithTimeout` never needs to know *why* the pulses stopped — only that they did. That decoupling is also what makes the whole pattern composable: any caller that only cares about liveness, not the reason for its absence, can reuse the same watch-loop shape against a different heartbeat-emitting worker.

**Verified**: confirmed the naive `main.go` fails all three tests exactly as captured above (`go test -race -timeout 30s -v ./...`, `GOROOT` set to the go1.25.6 toolchain `go.mod` requires), then copied the exercise into a throwaway scratch directory (with its own standalone `go.mod` so it builds outside the monorepo module) and dropped in the fix shown above in place of the shipped naive `main.go`. In that scratch copy: `go build ./...`, `go vet ./...`, and `gofmt -l .` are all clean, and `go test -race -count=5 -v ./...` passed all three tests five times in a row (15 test executions total) with zero flakes:

```
=== RUN   TestNoStallCompletesSuccessfully
--- PASS: TestNoStallCompletesSuccessfully (0.00s)
=== RUN   TestStallDetectedPromptly
--- PASS: TestStallDetectedPromptly (0.00s)
=== RUN   TestConcurrentSafety
--- PASS: TestConcurrentSafety (1.21s)
... (repeated identically 4 more times)
PASS
ok  	scratch27heartbeats	7.749s
```

`TestNoStallCompletesSuccessfully` and `TestStallDetectedPromptly` run inside `synctest.Test` with a fake clock, so their `(0.00s)` timings are real — the ~2s and ~1.1s of *simulated* time they exercise cost no actual wall-clock time. `TestConcurrentSafety` runs 20 concurrent callers with real (short) durations under `-race` and took ~1.2s of genuine wall time, matching `workUnits=3 * UnitDuration=400ms` run largely in parallel.

The concrete timing math behind `TestStallDetectedPromptly`'s bound, for reference: with `UnitDuration=400ms`, `stallAfter=2`, and `testPerPulseTimeout=300ms`, the stalled unit begins at `2*400ms = 800ms` into the job; the last reset event before that is the result for unit 1, delivered at exactly `800ms`; the correct implementation therefore times out at `800ms + 300ms = 1.1s` — comfortably inside the test's `stallStartsAt + perPulseTimeout + 2*workerPulseInterval = 1.3s` bound, and nowhere near the `2s` a normal job takes or the `24h` `StallDuration` the stalled unit is configured for. The naive version's flat `perPulseTimeout*workUnits = 1.5s` deadline sits just outside that 1.3s bound, which is exactly why `TestStallDetectedPromptly` catches it — and, as shown above, it also sits *inside* the ~2s a healthy job legitimately needs, which is why `TestNoStallCompletesSuccessfully` catches it too.

The repo's real `27-heartbeats/main.go` was left untouched (still the naive version above) throughout this verification; only the throwaway scratch copy ever received the fix.

## Key takeaways

- A heartbeat is only meaningful if it's causally tied to genuine progress on the work being monitored. A pulse driven by an independent ticker that fires on a fixed schedule regardless of what the worker is actually doing can't distinguish "still going" from "wedged" — it will happily keep ticking even while the thing it's supposed to be reporting on is completely stuck. The pulse has to originate from the same execution path that would also, eventually, produce the result.
- The caller-side pattern is symmetric and simple once the heartbeat is trustworthy: reset a single timer on *either* a heartbeat *or* a result, and treat that timer firing as the one and only stall signal. No polling, no separate "is it dead yet" check — just whichever of the three channels (`heartbeat`, `results`, `timer.C`) is ready first.
- A single flat deadline sized off the *whole* job is a fundamentally different (and weaker) tool than a deadline that resets on every sign of life: it conflates "the job is naturally long" with "the job is stuck," and — as the arithmetic above shows concretely — it often can't be sized correctly for both at once, forcing a choice between killing healthy long jobs and being too slow to catch real stalls.
- Always give `done`/cancellation a real exit path out of every blocking send or receive in a heartbeat-driven worker (`select` against `done`, never a bare channel op) — a heartbeat pattern that can't be told to stop leaks exactly the kind of goroutine it was built to help you detect.
- `time.Timer.Reset`'s safety rules changed in Go 1.23 — know which Go version a module targets before deciding whether you need the older Stop-then-drain-then-Reset dance or can just call `Reset` directly on a live timer.
