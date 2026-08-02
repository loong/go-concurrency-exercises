//////////////////////////////////////////////////////////////////////
//
// DoWork is supposed to simulate a long-running worker that processes
// workUnits units of work one at a time (see mockworker.go for the
// per-unit timing helper), sending one result per completed unit on
// results and closing results once the whole job is done. While it is
// actively working on a unit, it is also supposed to pulse on
// heartbeat roughly every pulseInterval, for as long as that unit
// takes - that pulse is the only way a caller monitoring the worker
// can tell "still working, just slow" apart from "wedged forever",
// since a unit that takes a long time and a unit that has stalled
// completely look identical if all you can do is wait for its result.
//
// Right now DoWork's heartbeat is basically decorative: it fires a
// single pulse the instant the goroutine starts, then never again -
// no matter how long an individual unit takes, or whether one has
// stalled outright. The declared heartbeat return value exists, but
// nothing after startup ever sends on it again.
//
// WorkWithTimeout is supposed to run DoWork and use the heartbeat to
// detect a stalled worker quickly: reset a timer every time a
// heartbeat OR a result arrives, and fail the moment perPulseTimeout
// elapses with neither. Right now it does nothing like that - it just
// starts a single flat timer sized off the whole job at the very
// beginning and never resets it, and it never even looks at
// heartbeat. That means it can't tell "worker pulsing normally on a
// slow-but-fine job" apart from "worker wedged": a healthy job that
// happens to run a bit long gets killed just as dead as a genuinely
// stalled one, and a real stall isn't caught within one pulse
// interval of it starting - only whenever that same flat deadline
// happens to expire, however long that turns out to be.
//
// Your task is to fix both:
//
//   - DoWork must pulse on heartbeat throughout each unit's work, not
//     just once at startup, while still respecting done (stop
//     promptly instead of blocking forever on a send nobody is
//     receiving).
//   - WorkWithTimeout must select on heartbeat, results, AND a timer
//     that gets reset to perPulseTimeout on every heartbeat or result
//     received - returning a non-nil error the moment perPulseTimeout
//     elapses with neither, instead of waiting for however long the
//     stalled unit would otherwise have taken.
//
// The signatures must stay the same:
//
//	func DoWork(done <-chan struct{}, pulseInterval time.Duration, workUnits int) (heartbeat <-chan struct{}, results <-chan int)
//	func WorkWithTimeout(workUnits int, stallAfter int, perPulseTimeout time.Duration) ([]int, error)
//
// stallAfter < 0 or stallAfter >= workUnits means "no stall, run
// normally" - see mockworker.go's SetStallUnit for how the simulated
// stall is configured.
//

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
// results when the whole job is done (or done fires early). It is
// meant to also pulse on heartbeat roughly every pulseInterval for as
// long as it is actively working on a unit - see the package comment
// above for why the implementation below does not actually do that
// yet.
func DoWork(done <-chan struct{}, pulseInterval time.Duration, workUnits int) (heartbeat <-chan struct{}, results <-chan int) {
	// hb is deliberately buffered (capacity 1) so the single startup
	// pulse below can actually complete even though WorkWithTimeout
	// (see below) discards the heartbeat return value and never
	// receives from it - an unbuffered channel here would make that
	// first send block forever, deadlocking the whole worker before
	// it ever reached its first unit of work.
	hb := make(chan struct{}, 1)
	res := make(chan int)

	go func() {
		defer close(res)

		// A single "I'm alive" pulse, sent once, right at startup.
		// This is NOT what the exercise asks for: it says nothing
		// about whether the worker is still making progress a second,
		// or a minute, into some later unit of work.
		select {
		case hb <- struct{}{}:
		case <-done:
			return
		}

		// checkIn is passed to SimulateUnit as its "still working,
		// got a moment to report in?" callback - but we never wire it
		// up to actually send a heartbeat, so no matter how long a
		// unit takes (normal or stalled), nothing more is ever sent
		// on hb after the single pulse above.
		checkIn := func() bool { return true }

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

// WorkWithTimeout runs DoWork to completion and is meant to fail fast
// - within roughly one perPulseTimeout window - the moment the worker
// stops pulsing AND stops producing results, instead of waiting for
// however long the stalled unit would otherwise have taken. See the
// package comment above for why the implementation below does not
// actually do that yet.
func WorkWithTimeout(workUnits int, stallAfter int, perPulseTimeout time.Duration) ([]int, error) {
	SetStallUnit(stallAfter)

	done := make(chan struct{})
	defer close(done)

	_, results := DoWork(done, workerPulseInterval, workUnits)

	// A single flat deadline for the ENTIRE job, set once, up front,
	// and never reset. It has no way to distinguish "still going,
	// just slow" from "wedged five seconds ago": both look exactly
	// the same to a timer that only ever fires once, however long
	// after the job started that turns out to be.
	timedOut := time.After(perPulseTimeout * time.Duration(workUnits))

	collected := make([]int, 0, workUnits)

	for {
		select {
		case r, ok := <-results:
			if !ok {
				return collected, nil
			}
			collected = append(collected, r)

		case <-timedOut:
			return nil, fmt.Errorf("work timed out after %s", perPulseTimeout*time.Duration(workUnits))
		}
	}
}

func main() {
	start := time.Now()
	res, err := WorkWithTimeout(5, -1, 300*time.Millisecond)
	fmt.Printf("no-stall run: results=%v err=%v (took %s)\n", res, err, time.Since(start))

	start = time.Now()
	res, err = WorkWithTimeout(5, 2, 300*time.Millisecond)
	fmt.Printf("stalled run:  results=%v err=%v (took %s)\n", res, err, time.Since(start))
}
