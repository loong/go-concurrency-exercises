//////////////////////////////////////////////////////////////////////
//
// This file has two independent, deliberately-broken pieces, each
// demonstrating a failure mode that is NOT deadlock (see
// 21-dining-philosophers for that one).
//
// Part 1 - RunLivelockDemo (LIVELOCK):
//
// Two workers, A and B, each need brief exclusive access to one shared
// resource, up to `attempts` times each. On contention, a worker that
// fails to acquire the resource is supposed to back off briefly and
// retry rather than blocking indefinitely. The naive version below
// backs off for the exact same fixed duration every time, with no
// randomness at all, and both workers start their attempt loop at the
// same instant. The result: whenever they collide, they both wait out
// the identical delay and retry at the exact same instant again - a
// perfect, self-sustaining lockstep. They collide, back off, collide,
// back off, forever. Both workers are constantly busy (no goroutine is
// ever stuck waiting on anything indefinitely - that's deadlock, a
// different failure mode), yet aProgress and bProgress both stay at (or
// near) zero even after attempts is fairly large, because neither of
// them ever manages to be the sole contender in a round.
//
// Each attempt round follows a fixed, four-step protocol - this
// protocol IS part of what you must keep; it's what makes contention
// deterministic and reproducible under testing/synctest instead of a
// matter of scheduler luck. It has nothing to do with the bug and
// nothing to do with the fix:
//
//  1. Announce: register as a contender for this round.
//  2. Settle: sleep a short, fixed probeWindow so every other
//     contender has time to announce too before anyone reads.
//  3. Read: snapshot how many contenders announced this round.
//  4. Settle again: sleep probeWindow a second time so nobody
//     withdraws (see step 5) before every contender has taken its
//     snapshot in step 3.
//  5. Withdraw, then act on the step-3 snapshot: exactly one
//     contender this round means an uncontested win; more than one
//     means a collision, and every colliding worker must back off
//     before retrying.
//
// The ONE thing that must change to fix the livelock is step 5's
// backoff duration: give each worker its own randomized jitter
// (seeded deterministically per worker so tests stay reproducible)
// instead of the identical fixed duration below, so the lockstep
// breaks and both workers accumulate steady real progress.
//
// Part 2 - Dispatcher (STARVATION):
//
// Dispatcher runs queued jobs in dispatch cycles, picking exactly one
// currently-queued job per cycle. The naive policy below is strict,
// never-aging priority: a cycle drains a high-priority job if one is
// queued, full stop, and only ever looks at the low-priority queue when
// the high-priority queue is completely empty. As long as the
// high-priority queue stays non-empty (a deep-enough backlog, or a
// steady trickle of new high-priority submissions), a low-priority job
// sitting right behind it can wait forever - even though the dispatcher
// is always doing useful work. That's starvation: no deadlock, no
// livelock, just an unfair policy that lets one goroutine's work be
// perpetually deprioritized.
//
// Your task: make RunDispatchCycles fair - e.g. guarantee at least 1
// out of every K cycles goes to a low-priority job if one is waiting,
// regardless of how deep the high-priority backlog is - so a
// low-priority job is guaranteed to complete within a bounded number of
// cycles no matter what. High-priority jobs must still get the large
// majority of cycles (at least two thirds) whenever both queues are
// busy - only the starvation has to go away.
//

package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// probeWindow is how long a worker waits, twice per round, for every
// other contender to announce and be read before anyone withdraws. It
// is part of the required contention protocol described above, not
// part of the bug and not part of the fix - leave both its shape (two
// sleeps per round) AND this exact value unchanged; check_test.go's
// minimum-elapsed-time check is calibrated against it.
const probeWindow = 1 * time.Millisecond

// fixedBackoff is the delay a worker sleeps out after losing contention
// for the shared resource. It is IDENTICAL for both workers and has no
// randomness whatsoever - that fixed, shared duration is exactly what
// makes the livelock below reproduce every single run instead of being
// a matter of scheduler luck.
const fixedBackoff = 10 * time.Millisecond

// retryBudgetMultiplier bounds the worst case: even in this naive,
// perpetually-colliding version, RunLivelockDemo must still return
// eventually instead of spinning forever. This is scaffolding, not the
// fix - a correct, jittered implementation should finish all attempts
// for both workers long before this budget is ever threatened.
const retryBudgetMultiplier = 200

// RunLivelockDemo runs two workers, A and B, each attempting to acquire
// and immediately release one shared resource up to attempts times.
// It returns how many times each worker actually succeeded before both
// finished (or the overall retry budget ran out).
func RunLivelockDemo(attempts int) (aProgress, bProgress int) {
	var contenders int32 // how many workers want the resource this round

	totalRetryBudget := int64(attempts) * retryBudgetMultiplier
	var totalRetries int64

	run := func(succeeded *int) {
		for *succeeded < attempts {
			if atomic.AddInt64(&totalRetries, 1) > totalRetryBudget {
				return // escape hatch: the demo itself must not hang
			}

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

			// Collision: someone else wanted the resource this exact
			// round too, so BOTH back off - and because the backoff is
			// the exact same fixed duration for both of them, they
			// wake at the exact same instant and collide again next
			// round. This is the bug: no jitter, so the lockstep never
			// breaks on its own.
			time.Sleep(fixedBackoff)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); run(&aProgress) }()
	go func() { defer wg.Done(); run(&bProgress) }()
	wg.Wait()

	return
}

// Dispatcher runs queued jobs in dispatch cycles. In its current, naive
// form it is a strict, never-aging priority queue: a cycle always
// drains a high-priority job if one is waiting, and only ever looks at
// low-priority jobs when the high-priority queue is completely empty.
type Dispatcher struct {
	mu   sync.Mutex
	high []int
	low  []int
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
func (d *Dispatcher) RunDispatchCycles(n int) (highCompleted, lowCompleted int) {
	for i := 0; i < n; i++ {
		d.mu.Lock()

		// BUG: high-priority jobs are always drained first, with no
		// regard for how long a low-priority job has been waiting. As
		// long as the high-priority queue keeps getting refilled (or
		// just starts deep enough), the branch below never runs.
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
	// NOTE: because of the missing-jitter bug described above, this
	// call will, in practice, churn for several real seconds -
	// exhausting its entire retry budget colliding in lockstep -
	// while printing zero progress for both workers. That's the bug
	// this half of the exercise is about. The graded artifact is the
	// test suite (check_test.go), which drives this through
	// testing/synctest's fake clock so it doesn't cost any real
	// wall-clock time.
	aProgress, bProgress := RunLivelockDemo(5)
	fmt.Printf("livelock demo: a=%d b=%d (both stuck near zero is the bug)\n", aProgress, bProgress)

	d := NewDispatcher()
	for i := 0; i < 20; i++ {
		d.SubmitHighPriority(i)
	}
	d.SubmitLowPriority(1)

	highCompleted, lowCompleted := d.RunDispatchCycles(10)
	fmt.Printf("dispatcher demo: high=%d low=%d (low stuck at zero is the bug)\n", highCompleted, lowCompleted)
}
