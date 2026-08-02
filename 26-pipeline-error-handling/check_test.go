//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"errors"
	"testing"
	"time"
)

// readings mixes several valid numeric strings with several invalid
// ones (non-numeric AND "0") scattered throughout, including good
// readings AFTER bad ones, so that an abort-on-first-error bug can't
// hide behind a batch that happens to end on a good note.
//
//	index:   0     1      2    3    4     5      6    7
//	value:  "10" "abc"   "5"  "0"  "-3"  "xyz"  "0"  "7"
//	kind:    ok   parse   ok  div   ok   parse   div   ok
//	              err          0                       0
var readings = []string{"10", "abc", "5", "0", "-3", "xyz", "0", "7"}

// goodIndex -> expected Value for readings that should succeed end to
// end (valid int, non-zero).
var goodIndex = map[int]int{
	0: 10,
	2: 5,
	4: -3,
	7: 7,
}

// parseErrIndex marks readings that fail to parse as an int at all.
var parseErrIndex = map[int]bool{
	1: true,
	5: true,
}

// divZeroIndex marks readings that parse fine but are exactly 0, so
// reciprocal should report ErrDivideByZero.
var divZeroIndex = map[int]bool{
	3: true,
	6: true,
}

// TestAllReadingsProduceResults checks that ProcessReadings always
// returns exactly one Result per input reading, no matter how many of
// them are bad. This is the length gate: if it fails, none of the
// per-index checks below can run meaningfully, which is itself a
// symptom of the abort-on-first-error bug.
func TestAllReadingsProduceResults(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	got := ProcessReadings(done, readings)

	if len(got) != len(readings) {
		t.Fatalf("ProcessReadings(%d readings) returned %d results, want %d - "+
			"looks like the pipeline aborted early instead of reporting "+
			"each bad reading and continuing", len(readings), len(got), len(readings))
	}
}

// TestGoodReadingsSucceed checks that every reading which should
// succeed end to end comes back with Err == nil and the expected
// Value, regardless of where it sits relative to a bad reading.
func TestGoodReadingsSucceed(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	got := ProcessReadings(done, readings)

	for idx, wantValue := range goodIndex {
		if len(got) <= idx {
			t.Fatalf("no result for reading %d (%q): pipeline aborted early "+
				"instead of reporting the error and continuing", idx, readings[idx])
		}
		res := got[idx]
		if res.Err != nil {
			t.Errorf("reading %d (%q): expected no error, got %v", idx, readings[idx], res.Err)
		}
		if res.Value != wantValue {
			t.Errorf("reading %d (%q): expected Value %d, got %d", idx, readings[idx], wantValue, res.Value)
		}
	}
}

// TestBadReadingsCarryErrors checks that every bad reading (whether a
// parse failure or a divide-by-zero) comes back with a non-nil Err
// instead of vanishing or crashing the batch.
func TestBadReadingsCarryErrors(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	got := ProcessReadings(done, readings)

	for idx := range parseErrIndex {
		if len(got) <= idx {
			t.Fatalf("no result for reading %d (%q): pipeline aborted early "+
				"instead of reporting the error and continuing", idx, readings[idx])
		}
		res := got[idx]
		if res.Err == nil {
			t.Errorf("reading %d (%q): expected a parse error, got Err == nil", idx, readings[idx])
		}
		if errors.Is(res.Err, ErrDivideByZero) {
			t.Errorf("reading %d (%q): got ErrDivideByZero, want a parse error - "+
				"reciprocal must never overwrite an existing (parse) error, and a "+
				"failed parse's Value defaults to 0 which must NOT be re-validated "+
				"as a divide-by-zero", idx, readings[idx])
		}
		if res.Value != 0 {
			t.Errorf("reading %d (%q): expected Value 0 on a parse failure, got %d", idx, readings[idx], res.Value)
		}
	}

	for idx := range divZeroIndex {
		if len(got) <= idx {
			t.Fatalf("no result for reading %d (%q): pipeline aborted early "+
				"instead of reporting the error and continuing", idx, readings[idx])
		}
		if !errors.Is(got[idx].Err, ErrDivideByZero) {
			t.Errorf("reading %d (%q): expected ErrDivideByZero, got %v", idx, readings[idx], got[idx].Err)
		}
	}
}

// TestProcessingContinuesPastBadReadings is the crux check: readings
// #2, #4, and #7 all come after at least one bad reading (a parse
// failure or a divide-by-zero), and #7 comes after two of each. If
// any stage aborts on the first bad item instead of reporting it and
// moving on, these later, otherwise-good readings are exactly the
// ones that go missing.
func TestProcessingContinuesPastBadReadings(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	got := ProcessReadings(done, readings)

	afterABadOne := []int{2, 4, 7}
	for _, idx := range afterABadOne {
		if len(got) <= idx {
			t.Fatalf("no result for reading %d (%q), which comes after a bad reading: "+
				"the pipeline stopped processing instead of continuing past the error", idx, readings[idx])
		}
		res := got[idx]
		if res.Err != nil {
			t.Errorf("reading %d (%q) after a bad reading: expected no error, got %v", idx, readings[idx], res.Err)
		}
		if res.Value != goodIndex[idx] {
			t.Errorf("reading %d (%q) after a bad reading: expected Value %d, got %d", idx, readings[idx], goodIndex[idx], res.Value)
		}
	}
}

// doneReadings is a batch of only valid, non-zero readings, long enough
// that neither stage can drain it instantly - it exists purely to give
// the done-cancellation tests below room to stall the pipeline mid-flight
// before closing done.
var doneReadings = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}

// pipelineStopsPromptlyOnDone is the shared body of the done-respecting
// check: it wires parse -> reciprocal directly (bypassing
// ProcessReadings so the test controls draining pace), reads exactly one
// Result to prove the pipeline is alive, then deliberately stops
// draining for a moment. With nobody reading, both parse's and
// reciprocal's blocking `out <- res` sends stall - exactly the state in
// which an implementation that dropped the `case <-done:` arm from its
// send select (or dropped done-handling entirely) would hang forever.
// It then closes done and asserts that the final channel closes
// promptly instead of the pipeline hanging, and that strictly fewer
// than len(doneReadings) results ever made it through - proving done
// actually cut the batch short rather than the pipeline having already
// finished on its own.
func pipelineStopsPromptlyOnDone(t *testing.T) {
	t.Helper()

	done := make(chan struct{})
	out := reciprocal(done, parse(done, doneReadings))

	select {
	case _, ok := <-out:
		if !ok {
			t.Fatal("pipeline closed its output before producing a single result")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("pipeline never produced a first result")
	}
	drained := 1

	// Let parse and reciprocal both settle into a blocked `out <- res`
	// send with nobody reading, so closing done below can only unblock
	// them via the `case <-done:` arm, never via a lucky concurrent
	// send/receive rendezvous.
	time.Sleep(20 * time.Millisecond)

	close(done)

	for {
		select {
		case _, ok := <-out:
			if !ok {
				if drained >= len(doneReadings) {
					t.Errorf("pipeline drained all %d readings after done was closed - "+
						"expected done to cut the batch short instead of letting it run to completion", drained)
				}
				return
			}
			drained++
			if drained >= len(doneReadings) {
				t.Fatal("pipeline kept producing results well past done being closed instead of stopping promptly")
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("pipeline did not stop promptly after done was closed - a stage appears to be leaked, " +
				"blocked forever on a send that ignores done")
		}
	}
}

// TestPipelineStopsPromptlyOnDone proves that closing done actually
// unblocks and shuts down both parse and reciprocal - including while
// each is blocked mid-send on a full pipeline - rather than letting
// either one run the batch to completion or hang forever.
func TestPipelineStopsPromptlyOnDone(t *testing.T) {
	pipelineStopsPromptlyOnDone(t)
}

// TestPipelineNoGoroutineLeakRace repeats the shutdown scenario a number
// of times so that, run with `go test -race`, it has a reasonable
// chance of catching a data race in a select-based shutdown path that
// only shows up under contention.
func TestPipelineNoGoroutineLeakRace(t *testing.T) {
	for i := 0; i < 10; i++ {
		pipelineStopsPromptlyOnDone(t)
	}
}
