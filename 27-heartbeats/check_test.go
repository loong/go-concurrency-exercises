//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// perPulseTimeout used across the synctest-driven tests below. It
// must be longer than workerPulseInterval (see main.go) or no
// implementation, correct or not, could ever see a heartbeat before
// timing out.
const testPerPulseTimeout = 300 * time.Millisecond

// TestNoStallCompletesSuccessfully runs a job with no stall
// (stallAfter out of range) and checks that it completes normally,
// returning every result with a nil error - a slow-but-healthy job
// must never be mistaken for a stalled one.
func TestNoStallCompletesSuccessfully(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workUnits = 5

		res, err := WorkWithTimeout(workUnits, -1, testPerPulseTimeout)
		if err != nil {
			t.Fatalf("expected nil error for a normal run, got %v", err)
		}

		if len(res) != workUnits {
			t.Fatalf("expected %d results, got %d: %v", workUnits, len(res), res)
		}

		for i, v := range res {
			if v != i {
				t.Errorf("res[%d] = %d, want %d", i, v, i)
			}
		}
	})
}

// TestStallDetectedPromptly runs a job whose 3rd unit (index 2) is
// configured to stall - see mockworker.go's SimulateUnit - and checks
// that WorkWithTimeout returns a non-nil error within roughly one
// testPerPulseTimeout window of the stall starting (unit 2 begins at
// 2*UnitDuration = 800ms of simulated time into the job), rather than
// only after however long the stalled unit would otherwise have taken
// (StallDuration is 24h - effectively forever). This fails against
// the naive main.go, whose single flat whole-job timeout only fires
// at testPerPulseTimeout*workUnits = 1.5s, well after this test's
// deadline.
func TestStallDetectedPromptly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			workUnits  = 5
			stallAfter = 2
		)

		start := time.Now()
		res, err := WorkWithTimeout(workUnits, stallAfter, testPerPulseTimeout)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("expected a non-nil error once the worker stalled, got nil (results: %v)", res)
		}

		// Sanity floor: the stall doesn't even start until unit 2
		// begins, at 2*UnitDuration into the job. Firing before that
		// would mean something else entirely is wrong.
		stallStartsAt := 2 * UnitDuration
		if elapsed < stallStartsAt {
			t.Fatalf("detected a stall before unit %d even started: elapsed %s, unit %d starts at %s",
				stallAfter, elapsed, stallAfter, stallStartsAt)
		}

		// Promptness bound: detection must land close to
		// stallStartsAt + testPerPulseTimeout (~1.1s here), not
		// anywhere near the ~2s the full job would otherwise take, and
		// nowhere near the 24h StallDuration the stalled unit is
		// configured for.
		deadline := stallStartsAt + testPerPulseTimeout + 2*workerPulseInterval
		if elapsed > deadline {
			t.Fatalf("did not detect the stall promptly: took %s, want at most %s "+
				"(stall starts at %s + one perPulseTimeout of %s)",
				elapsed, deadline, stallStartsAt, testPerPulseTimeout)
		}
	})
}

// TestConcurrentSafety hammers WorkWithTimeout from many goroutines at
// once with real (short) durations to catch data races on DoWork's
// shared heartbeat/results plumbing and mockworker.go's package-level
// stall configuration. Every caller passes a negative stallAfter so
// they're all exercising the normal path and don't fight each other
// over which unit should stall. Run with `go test -race`.
func TestConcurrentSafety(t *testing.T) {
	const (
		callers   = 20
		workUnits = 3
	)

	var wg sync.WaitGroup
	wg.Add(callers)

	errs := make([]error, callers)

	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()

			res, err := WorkWithTimeout(workUnits, -1, testPerPulseTimeout)
			if err == nil && len(res) != workUnits {
				err = errors.New("unexpected result count from a non-stalling run")
			}
			errs[i] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
}
