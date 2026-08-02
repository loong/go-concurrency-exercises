//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"testing"
	"testing/synctest"
	"time"
)

// TestLivelockBothWorkersMakeProgress checks that neither worker is
// livelocked: after a moderately large number of attempts, BOTH
// aProgress and bProgress must have cleared a reasonable floor. Run
// inside synctest so the fake clock resolves every backoff instantly
// instead of costing any real wall-clock time. This also runs both
// workers' goroutines concurrently against the same shared counters,
// so `go test -race` exercises the concurrency-safety of whatever
// synchronization RunLivelockDemo ends up using.
//
// Against the naive, fixed-backoff implementation, both workers
// collide in lockstep on every single round and neither ever clears
// the floor - this test must fail against it.
//
// It also checks that RunLivelockDemo actually consumed a non-trivial
// amount of fake time getting there. The required announce/settle
// protocol (see main.go and README.md) costs at least two probeWindow
// sleeps per attempt even when a round is uncontested, so any
// implementation that keeps that protocol - jittered or not - burns
// through a predictable minimum of fake time for `attempts` rounds.
// An implementation that quietly drops the required protocol (e.g. a
// bare, un-synchronized TryLock loop, or a plain blocking Mutex with no
// backoff-and-retry at all) has no reason to consume anything close to
// that minimum: real contention on an all-but-instant critical section
// is rare enough that it can clear the progress floor while barely
// sleeping at all, or not sleeping at all. This is what actually
// exercises the "must back off and retry using the required protocol"
// requirement, rather than the progress floor alone, which a lucky
// implementation without any real contention handling can clear too.
func TestLivelockBothWorkersMakeProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const attempts = 50
		const floor = attempts / 4 // generous: neither worker should be anywhere near stuck at zero

		// Conservative lower bound on the per-attempt round overhead a
		// genuine implementation of the required announce/settle
		// protocol must incur, even with zero collisions - comfortably
		// under half of the documented 1ms probeWindow, so this never
		// false-fails a compliant implementation (see check below),
		// but well above what a handful of incidental real collisions
		// in a protocol-less TryLock/Mutex loop could rack up.
		const minRoundOverhead = 500 * time.Microsecond
		minElapsed := time.Duration(attempts) * minRoundOverhead

		start := time.Now()
		aProgress, bProgress := RunLivelockDemo(attempts)
		elapsed := time.Since(start)

		if aProgress < floor {
			t.Errorf("worker A only made %d/%d attempts of progress (floor %d) - looks livelocked", aProgress, attempts, floor)
		}
		if bProgress < floor {
			t.Errorf("worker B only made %d/%d attempts of progress (floor %d) - looks livelocked", bProgress, attempts, floor)
		}
		if elapsed < minElapsed {
			t.Errorf("RunLivelockDemo returned after only %v of (fake) time for %d attempts (want >= %v) - "+
				"looks like it isn't actually running the required announce/settle/backoff protocol",
				elapsed, attempts, minElapsed)
		}
	})
}

// TestDispatcherServicesQueuedJobsWhenAvailable is a basic sanity
// check, independent of the starvation bug: with no high-priority
// backlog at all, low-priority jobs still get serviced normally.
func TestDispatcherServicesQueuedJobsWhenAvailable(t *testing.T) {
	d := NewDispatcher()
	d.SubmitLowPriority(1)
	d.SubmitLowPriority(2)

	highCompleted, lowCompleted := d.RunDispatchCycles(2)

	if highCompleted != 0 || lowCompleted != 2 {
		t.Fatalf("expected 0 high-priority and 2 low-priority completions, got high=%d low=%d", highCompleted, lowCompleted)
	}
}

// TestDispatcherLowPriorityNotStarved keeps a deep, never-emptying
// high-priority backlog queued for the entire run and submits exactly
// one low-priority job alongside it. A fair policy must service that
// low-priority job within a bounded number of cycles no matter how
// deep the high-priority backlog is; this test runs enough cycles that
// any reasonable fairness policy (e.g. aging every handful of cycles)
// would clearly have serviced it by now.
//
// Against the naive strict-priority implementation, the high-priority
// backlog here never empties across any of the cycles run, so the
// low-priority job is never even looked at - this test must fail
// against it.
func TestDispatcherLowPriorityNotStarved(t *testing.T) {
	d := NewDispatcher()

	const highBacklog = 500
	const cycles = 100

	for i := 0; i < highBacklog; i++ {
		d.SubmitHighPriority(i)
	}
	d.SubmitLowPriority(1)

	highCompleted, lowCompleted := d.RunDispatchCycles(cycles)

	// Every cycle must have completed some job: the backlog (500) is
	// far bigger than the number of cycles run (100), so at no point
	// during this run can both queues have been empty.
	if highCompleted+lowCompleted != cycles {
		t.Fatalf("expected every one of %d cycles to complete some job, got high=%d low=%d (sum=%d)",
			cycles, highCompleted, lowCompleted, highCompleted+lowCompleted)
	}

	if lowCompleted < 1 {
		t.Fatalf("low-priority job was starved: 0 completions after %d cycles with a %d-job high-priority backlog still queued", cycles, highBacklog)
	}
}

// TestDispatcherHighPriorityKeepsMajorityShare keeps BOTH queues
// non-empty for the entire run (a deep high-priority backlog plus a
// deep-enough low-priority backlog that it never drains either) and
// checks the README's fairness requirement from the other direction:
// a fair policy must still hand high-priority work the large majority
// of cycles - at least two thirds - while it's guaranteeing low-priority
// jobs a bounded wait. This is what distinguishes a genuinely fair,
// aging-based policy from one that has simply swapped which queue gets
// starved (e.g. draining low-priority first whenever it's non-empty):
// an inverted policy still clears TestDispatcherLowPriorityNotStarved
// (it services low-priority jobs immediately) but fails the majority
// share checked here.
func TestDispatcherHighPriorityKeepsMajorityShare(t *testing.T) {
	d := NewDispatcher()

	const highBacklog = 500
	const lowBacklog = 50
	const cycles = 100

	for i := 0; i < highBacklog; i++ {
		d.SubmitHighPriority(i)
	}
	for i := 0; i < lowBacklog; i++ {
		d.SubmitLowPriority(i)
	}

	highCompleted, lowCompleted := d.RunDispatchCycles(cycles)

	if highCompleted+lowCompleted != cycles {
		t.Fatalf("expected every one of %d cycles to complete some job, got high=%d low=%d (sum=%d)",
			cycles, highCompleted, lowCompleted, highCompleted+lowCompleted)
	}

	if lowCompleted < 1 {
		t.Fatalf("low-priority job was starved: 0 completions after %d cycles with both queues kept busy", cycles)
	}

	if highCompleted < 2*lowCompleted {
		t.Fatalf("high-priority work didn't keep the majority share: high=%d low=%d over %d cycles "+
			"(want high >= 2x low, i.e. at least two thirds of cycles going to high-priority while both queues stay busy)",
			highCompleted, lowCompleted, cycles)
	}
}
