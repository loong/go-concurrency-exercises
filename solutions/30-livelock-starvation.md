# Livelock & Starvation: Two Failure Modes Beyond Deadlock - Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `30-livelock-starvation/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

This exercise has two independent, deliberately-broken pieces in one package, contrasting two failure modes that are *not* deadlock (that one's covered in [21-dining-philosophers](../21-dining-philosophers)):

**Part 1 — livelock.** `RunLivelockDemo(attempts int) (aProgress, bProgress int)` runs two workers, A and B, each trying to acquire-and-release one shared resource up to `attempts` times. Every attempt round runs through a fixed, five-step protocol that is required scaffolding (not the bug, not the fix — see `main.go` and `README.md`): announce as a contender, sleep `probeWindow`, snapshot the contender count, sleep `probeWindow` again, then withdraw and act on the snapshot (sole contender = win, otherwise collision = back off and retry). The naive version's bug is entirely in step 5's backoff: both workers back off for the exact same fixed `10ms` duration, with no randomness. Because both workers start their attempt loop at the same instant and the protocol keeps their rounds aligned, the result is a perfect, self-sustaining lockstep: both workers collide, both back off identically, both retry at the identical instant, collide again — forever (bounded only by an escape-hatch retry budget so the demo itself terminates).

**Part 2 — starvation.** `Dispatcher` runs jobs in dispatch cycles via `RunDispatchCycles(n int) (highCompleted, lowCompleted int)`, picking exactly one queued job per cycle from either its high- or low-priority queue (`SubmitHighPriority`/`SubmitLowPriority`). The naive policy is strict, never-aging priority: drain a high-priority job if one is queued, full stop, and only ever look at low-priority when high is *completely* empty. As long as the high-priority queue stays non-empty — a deep enough backlog, or a steady trickle of new submissions — a low-priority job sitting right behind it waits forever, even though the dispatcher is always doing useful work (no deadlock, no livelock — just an unfair policy).

Required signatures (must stay identical):

```go
func RunLivelockDemo(attempts int) (aProgress, bProgress int)

type Dispatcher struct{ /* unexported fields, your choice */ }
func NewDispatcher() *Dispatcher
func (d *Dispatcher) SubmitHighPriority(job int)
func (d *Dispatcher) SubmitLowPriority(job int)
func (d *Dispatcher) RunDispatchCycles(n int) (highCompleted, lowCompleted int)
```

## Why the naive version is wrong

- **Livelock:** `fixedBackoff = 10 * time.Millisecond` is shared by both workers, with no jitter, and it's used as step 5 of the required announce/settle/read/settle/decide protocol. Both workers register as contenders and check for a collision on the exact same round (their `Sleep(probeWindow)` calls resolve at the same fake-clock instant, deterministically, under `testing/synctest`), then wait out the identical fixed delay before the next round — so whichever worker "loses" a round always loses in perfect sync with the other, and the pair never spontaneously desynchronizes. Both `aProgress` and `bProgress` stay stuck at (or near) zero however large `attempts` gets — busy the whole time, making zero real progress.
- **Starvation:** `RunDispatchCycles`'s naive body is `if len(d.high) > 0 { drain high; continue }` first, `low` only ever considered in the branch where `high` is empty at that exact instant. With a deep high-priority backlog kept non-empty across the whole run, the `low` branch is provably never reached.

Verified: running the current `check_test.go` against the naive `main.go` in the repo directory itself (before any fix was applied) fails all three bug-targeting tests, for exactly the described reasons:

```
=== RUN   TestLivelockBothWorkersMakeProgress
    check_test.go:63: worker A only made 0/50 attempts of progress (floor 12) - looks livelocked
    check_test.go:66: worker B only made 0/50 attempts of progress (floor 12) - looks livelocked
--- FAIL: TestLivelockBothWorkersMakeProgress (0.02s)
=== RUN   TestDispatcherServicesQueuedJobsWhenAvailable
--- PASS: TestDispatcherServicesQueuedJobsWhenAvailable (0.00s)
=== RUN   TestDispatcherLowPriorityNotStarved
    check_test.go:125: low-priority job was starved: 0 completions after 100 cycles with a 500-job high-priority backlog still queued
--- FAIL: TestDispatcherLowPriorityNotStarved (0.00s)
=== RUN   TestDispatcherHighPriorityKeepsMajorityShare
    check_test.go:163: low-priority job was starved: 0 completions after 100 cycles with both queues kept busy
--- FAIL: TestDispatcherHighPriorityKeepsMajorityShare (0.00s)
FAIL
FAIL	github.com/loong/go-concurrency-exercises/30-livelock-starvation	0.184s
```

Both livelock/starvation failures are the exact ones the exercise is about — worker A and B stuck at *exactly* 0/50 (not "a bit low", genuinely zero: the lockstep collision is deterministic every round), and the low-priority job stuck at *exactly* 0/100 despite the dispatcher completing all 100 cycles (`highCompleted + lowCompleted == 100` still holds — the dispatcher is always making progress, just never on the low-priority job). `TestDispatcherServicesQueuedJobsWhenAvailable` passes against the naive version too, as expected — it's a sanity check with no high-priority backlog at all, so the bug never triggers. (`TestDispatcherHighPriorityKeepsMajorityShare` also fails against the naive version, but only incidentally, via the same zero-`lowCompleted` starvation bug — it isn't the test that's specifically designed to catch the naive strict-priority bug; see "The fix" below for what it's actually for.)

## The fix

```go
package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// probeWindow is how long a worker waits, twice per round, for every
// other contender to announce and be read before anyone withdraws.
// This is required scaffolding (see README.md) - unchanged from the
// naive version.
const probeWindow = 1 * time.Millisecond

// baseBackoff is the minimum delay a worker sleeps out after losing
// contention for the shared resource. Each worker adds its own
// independently-seeded random jitter on top of this so the two workers'
// retry rhythms are never in lockstep.
const baseBackoff = 10 * time.Millisecond

// jitterSpan is the width of the random jitter window added on top of
// baseBackoff.
const jitterSpan = 10 * time.Millisecond

// RunLivelockDemo runs two workers, A and B, each attempting to acquire
// and immediately release one shared resource up to attempts times. It
// returns how many times each worker actually succeeded. The
// announce/settle/read/settle/decide protocol is unchanged from the
// naive version - the only change is that step 5's backoff is now
// jittered independently per worker instead of an identical fixed
// duration, so the lockstep the naive version suffers from can't
// reproduce indefinitely.
func RunLivelockDemo(attempts int) (aProgress, bProgress int) {
	var contenders int32 // how many workers want the resource this round

	// Each worker gets its own independently-seeded RNG. Fixed seeds
	// keep the demo reproducible across runs/tests, while still giving
	// the two workers uncorrelated backoff durations - that's the whole
	// fix: once their retry rhythms are no longer identical, a
	// collision can't perfectly repeat forever.
	randA := rand.New(rand.NewSource(1))
	randB := rand.New(rand.NewSource(2))

	run := func(succeeded *int, rng *rand.Rand) {
		for *succeeded < attempts {
			// Step 1: announce as a contender for this round.
			atomic.AddInt32(&contenders, 1)

			// Step 2: settle so every contender gets a chance to
			// announce before anyone reads.
			time.Sleep(probeWindow)

			// Step 3: snapshot how many contenders announced.
			n := atomic.LoadInt32(&contenders)

			// Step 4: settle again so nobody withdraws before every
			// contender has taken its own snapshot.
			time.Sleep(probeWindow)

			// Step 5: withdraw, then act on the step-3 snapshot.
			atomic.AddInt32(&contenders, -1)

			if n == 1 {
				// Sole contender this round: acquire-and-release the
				// resource succeeds.
				*succeeded++
				continue
			}

			// Collision: back off for a jittered duration unique to
			// this worker, then retry. Independent jitter is what
			// breaks the lockstep - the two workers no longer wake at
			// the same instant, so they stop re-colliding round after
			// round.
			jitter := time.Duration(rng.Int63n(int64(jitterSpan)))
			time.Sleep(baseBackoff + jitter)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); run(&aProgress, randA) }()
	go func() { defer wg.Done(); run(&bProgress, randB) }()
	wg.Wait()

	return
}

// agingPeriod controls fairness: at least 1 out of every agingPeriod
// dispatch cycles is reserved for a waiting low-priority job (if any),
// no matter how deep the high-priority backlog is. High-priority jobs
// still win the large majority of cycles (agingPeriod-1 out of every
// agingPeriod) when both queues are busy.
const agingPeriod = 5

// Dispatcher runs queued jobs in dispatch cycles using a fair,
// weighted-round-robin policy: high-priority jobs get priority, but a
// waiting low-priority job is guaranteed a cycle at least once every
// agingPeriod cycles, so it can never be starved indefinitely.
type Dispatcher struct {
	mu    sync.Mutex
	high  []int
	low   []int
	cycle int // total cycles run so far, used to schedule the aging slot
}

// NewDispatcher creates an empty Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// SubmitHighPriority enqueues job onto the high-priority queue.
func (d *Dispatcher) SubmitHighPriority(job int) {
	d.mu.Lock()
	d.high = append(d.high, job)
	d.mu.Unlock()
}

// SubmitLowPriority enqueues job onto the low-priority queue.
func (d *Dispatcher) SubmitLowPriority(job int) {
	d.mu.Lock()
	d.low = append(d.low, job)
	d.mu.Unlock()
}

// RunDispatchCycles runs n dispatch cycles. Each cycle picks exactly
// one currently-queued job to run to completion, recording it in
// highCompleted or lowCompleted depending on which queue it came from.
//
// Fairness policy: every agingPeriod-th cycle is reserved for the
// low-priority queue if it has anything waiting (aging). All other
// cycles prefer high-priority. Either way, if the preferred queue for
// this cycle is empty, the other queue is tried instead, so a cycle
// only does nothing when both queues are empty.
func (d *Dispatcher) RunDispatchCycles(n int) (highCompleted, lowCompleted int) {
	for i := 0; i < n; i++ {
		d.mu.Lock()
		d.cycle++

		lowIsAgingTurn := d.cycle%agingPeriod == 0

		if lowIsAgingTurn && len(d.low) > 0 {
			d.low = d.low[1:]
			d.mu.Unlock()
			lowCompleted++
			continue
		}

		if len(d.high) > 0 {
			d.high = d.high[1:]
			d.mu.Unlock()
			highCompleted++
			continue
		}

		if len(d.low) > 0 {
			d.low = d.low[1:]
			d.mu.Unlock()
			lowCompleted++
			continue
		}

		d.mu.Unlock() // nothing queued this cycle
	}

	return
}

func main() {
	aProgress, bProgress := RunLivelockDemo(50)
	fmt.Printf("livelock demo: a=%d b=%d (both should be close to 50)\n", aProgress, bProgress)

	d := NewDispatcher()
	for i := 0; i < 500; i++ {
		d.SubmitHighPriority(i)
	}
	d.SubmitLowPriority(1)

	highCompleted, lowCompleted := d.RunDispatchCycles(100)
	fmt.Printf("dispatcher demo: high=%d low=%d (low should be >= 1)\n", highCompleted, lowCompleted)
}
```

## Design notes

**Livelock — why the announce/settle protocol has to stay, and what a naive port would get wrong:**

- The naive version's whole failure mode comes from *correlated* retry timing, not from the choice of primitive. The bug is that both workers' backoff durations are drawn from the same deterministic, fixed source, so once they collide once, they collide on every subsequent round too. The fix changes exactly one thing: step 5's backoff duration goes from a shared `fixedBackoff` to an independently-jittered one per worker. Everything else - the announce/settle/read/settle/decide shape of each round - is unchanged.
- That protocol is required, not decorative, and this is worth being explicit about because an earlier draft of this exercise got it backwards: it built the naive version's forced-collision behavior on top of a separate, clearly-optional-looking `sync.Cond`-driven "cadence" goroutine, and told students they didn't need to preserve it. In practice that invited two ways to "solve" the exercise without actually fixing anything: (1) delete the cadence and swap in a bare `mu.TryLock()` loop while *keeping* the identical fixed backoff - contention on an all-but-instant critical section becomes so rare without a forcing mechanism that the naive bug essentially never reproduces, so the floor check passes anyway; (2) delete the retry/backoff pattern entirely and use a plain blocking `mu.Lock()` - trivially non-livelocked (nothing is ever busy-retrying) and trivially passes the floor check too, without implementing anything the exercise is about. Both were confirmed to pass the then-current test 10-30/30 times. The fix here is to make the announce/settle protocol part of the *baseline implementation itself* (not a peripheral, removable mechanism), so that reproducing the naive bug or its fix means changing the one line the exercise is actually about, not deleting the whole mechanism that makes contention observable in the first place.
- Each worker owns its own `*rand.Rand` with its own fixed seed (`1` and `2`). Two things ride on this: (1) independent generators mean that if the two workers *do* land on the same round at the same instant, the jittered delays they each draw afterward have no reason to re-align, so any collision that occurs is a one-off, not the start of a new repeating cycle; (2) fixed (not time-based) seeds keep `RunLivelockDemo` fully deterministic and reproducible run-to-run and under `testing/synctest`'s fake clock, without needing real entropy.
- `check_test.go` verifies more than just the progress floor: it also asserts that `RunLivelockDemo` consumes at least `attempts * 500µs` of fake time. Every round of the required protocol costs at least two `probeWindow` (1ms) sleeps *even when uncontested*, so any implementation that actually keeps the protocol burns through a predictable minimum of fake time proportional to `attempts`, win or lose. An implementation that dropped the protocol (either adversarial variant above) has no reason to consume anywhere near that minimum, since real contention on an empty critical section is rare enough that it can clear the progress floor while sleeping barely at all. Measured directly: the naive fixed-backoff-without-protocol variant (adversarial variant 1 above) and the plain-blocking-mutex variant (adversarial variant 2) both now fail this check reliably - variant 1 failed 125/125 times across 25 separate `go test -race -count=5` invocations, variant 2 failed every run (elapsed stays at exactly `0s`, since it never calls `time.Sleep` at all) - while the actual fix above passes every time (elapsed is consistently ~900ms for `attempts=50`, comfortably above the 25ms floor and nowhere near the retry-budget timeout the naive version needs).
- Under `testing/synctest`, `time.Sleep` resolves against the fake clock instantly once every other goroutine is durably blocked, so this whole demo costs no wall-clock time in the test even though it now deterministically executes real `time.Sleep` calls every round.

**Starvation — why weighted round-robin/aging, and what a naive port would get wrong:**

- The naive policy's bug isn't "wrong primitive" - a single `sync.Mutex` guarding two slices is completely fine for this. The bug is purely in the *scheduling decision*: "high always wins if non-empty" has no path by which accumulated waiting time for a low-priority job ever changes the outcome. Any fix has to introduce some notion of *aging* - the longer something waits, the more claim it accrues on being serviced - even in the simplest possible form.
- The fix here uses the simplest aging rule that satisfies the spec: a monotonically increasing `d.cycle` counter, and "if `cycle % agingPeriod == 0` and low has something waiting, take low first." This guarantees a low-priority job is picked within at most `agingPeriod` cycles of becoming the head of the low queue, *regardless* of how many high-priority jobs are queued behind it or how many keep arriving - the aging check runs unconditionally, before the high-priority check, so no high-priority backlog size can ever suppress it.
- Order of the three checks inside the critical section matters and is deliberate: aging-slot-for-low first, then high, then low-as-fallback. If the aging check were placed *after* the high-priority check ("drain high if non-empty, otherwise take the aging turn"), a permanently non-empty high queue would make the aging branch just as unreachable as the original bug.
- `check_test.go` checks the starvation fix from *both* directions, which matters because they're independently gameable: `TestDispatcherLowPriorityNotStarved` (deep high backlog, exactly one low job) catches a naive strict-high-priority policy, but it does *not* catch a policy that has simply swapped which queue is favored - a Dispatcher that always drains low first, and only touches high when low is empty, services that single low job on cycle 1 and clears `lowCompleted >= 1` trivially, while starving high instead. `TestDispatcherHighPriorityKeepsMajorityShare` closes that gap: with both a deep high backlog (500) *and* a deep-enough low backlog (50) kept non-empty for the whole 100-cycle run, it asserts `highCompleted >= 2*lowCompleted` (i.e. at least two thirds of cycles go to high-priority), which the low-favored inversion fails outright (it produces a 50/50 split) while the `agingPeriod = 5` fix above passes with room to spare (it produces 80/20 - every 5th cycle is low's aging turn, 20 out of 100, and high takes the rest). Verified directly: an inverted-priority Dispatcher variant (low drained first, high only when low is empty) passes `TestDispatcherLowPriorityNotStarved` but fails `TestDispatcherHighPriorityKeepsMajorityShare` on every one of 5 runs with `high=50 low=50`.
- `agingPeriod = 5` is a policy knob, not a correctness requirement - any period from 2 up to about 7 or 8 keeps both the "low serviced within a bounded number of cycles" and "high keeps at least two thirds of cycles" requirements satisfied simultaneously; smaller periods favor fairness more, larger periods favor high-priority throughput more (but push closer to, and eventually past, the two-thirds floor the tests check).

## Key takeaways

- **Livelock and starvation are not the same bug, and neither is deadlock.** Deadlock: goroutines are stuck, blocked forever, doing nothing. Livelock: goroutines are constantly busy, actively executing, and *still* making zero net progress because their actions keep canceling each other out in a repeating pattern. Starvation: the system as a whole is making steady progress - deadlock and livelock are both absent - but one specific participant's requests are systematically never selected by the granting policy. All three look identical from a "is anything crashing?" monitoring dashboard; only progress-per-participant over time distinguishes them.
- **Backoff-and-retry is not automatically a livelock fix; it can just as easily reproduce the thing it's supposed to prevent** if all retriers use the same deterministic delay. The fix isn't "add backoff," it's "add backoff whose *timing is not shared* across contenders" - independent jitter, not merely non-zero jitter.
- **A black-box progress-floor test can be satisfied by implementations that don't fix anything, if the thing being fixed is a timing property and the test doesn't force the timing conditions that expose it.** A bare `TryLock` loop or a plain blocking mutex both trivially clear "did both workers make progress" without ever exercising contention-and-recovery at all. Pairing the progress check with a minimum-elapsed-fake-time check (grounded in the *mandatory* per-round protocol cost, not the bug or the fix) is what actually forces an implementation to go through real, repeated contention before it can pass.
- **A "fair enough" scheduling policy needs an aging mechanism with a provable worst-case bound**, not just "look at the other queue sometimes." A counter-based modular check (`cycle % agingPeriod == 0`) gives an exact, easy-to-reason-about bound (serviced within `agingPeriod` cycles of being at the head of its queue) with a single extra field and one extra `if`.
- **"The low-priority job eventually runs" and "high-priority still gets the majority of the work" are two separate properties that both need their own test.** A policy that just inverts which queue is favored passes the first trivially while failing the second; testing only the first can't tell a genuinely fair, aging-based policy apart from one that has simply moved the starvation to the other queue.

**Verified**: ran the existing `check_test.go` against the repo's naive `main.go` in place, confirming `TestLivelockBothWorkersMakeProgress`, `TestDispatcherLowPriorityNotStarved`, and `TestDispatcherHighPriorityKeepsMajorityShare` all fail with the exact output shown above (`TestDispatcherServicesQueuedJobsWhenAvailable` passes against the naive version too, as expected). `go build .`, `go vet .`, and `gofmt -l .` are all clean on the repo's naive `main.go`.

Then copied the exercise into a throwaway scratch directory (own `go.mod`, outside the repo) and dropped in the fix above in place of the naive `main.go`. `go build .` and `go vet .` are clean, `gofmt -l .` reports nothing. Ran `go test -race -count=5 -v ./...` (all 4 tests pass on every one of 5 iterations) and then `go test -race -count=30 ./...` (clean, no flakes) in the scratch copy.

Additionally built three adversarial variants to confirm the strengthened tests actually discriminate (not just the naive-vs-fix pair):
- **Variant 1** (protocol removed, bare `mu.TryLock()` loop, identical fixed 10ms backoff kept): failed `TestLivelockBothWorkersMakeProgress` on 125/125 sub-runs across 25 separate `go test -race -count=5` invocations, via the elapsed-fake-time check (`0s`/`10ms` observed, both well under the 25ms floor).
- **Variant 2** (plain blocking `mu.Lock()`, no backoff/retry at all): failed the same test on every run (elapsed stays exactly `0s`).
- **Variant 3** (Dispatcher priority fully inverted - low drained first, high only when low is empty): passed `TestDispatcherLowPriorityNotStarved` but failed `TestDispatcherHighPriorityKeepsMajorityShare` on every one of 5 runs (`high=50 low=50`, want `high >= 2*low`).

The repo's own copy of `30-livelock-starvation/main.go` was left untouched as the original naive version throughout.
