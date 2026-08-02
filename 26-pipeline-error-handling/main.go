//////////////////////////////////////////////////////////////////////
//
// Given is a small pipeline that takes a batch of raw sensor-reading
// strings, parses each one to an int (parse), then checks each parsed
// value for a divide-by-zero condition (reciprocal). Two things can go
// wrong per reading: it might not parse as an integer at all, or it
// might be exactly 0, which the downstream reciprocal step can't
// handle.
//
// Right now, the moment either stage hits ONE bad reading, it just
// stops - the goroutine returns early instead of reporting the
// problem and moving on to the rest of the batch. Concretely:
//
//   - parse returns from its goroutine (closing its output channel)
//     the instant strconv.Atoi fails on a single reading, without
//     ever looking at any reading that came after it.
//   - reciprocal does the same thing the instant it sees a Value of
//     0, abandoning every reading still waiting behind it.
//   - The Err field on Result is declared but never actually set by
//     either stage - a bad reading doesn't get reported, it just
//     makes everything after it vanish.
//
// This is a problem: ProcessReadings("10", "abc", "5", ...) should
// give the caller one Result per input reading, in order, so the
// caller can decide what to do about each failure - not silently
// truncate the whole batch because reading #2 out of #500 happened to
// be garbage.
//
// Your task is to make parse and reciprocal let errors flow
// downstream ALONGSIDE values instead of aborting on them, so only
// ProcessReadings's caller decides what to do about a bad reading:
//
//   - parse must emit exactly one Result per input reading, in the
//     same order as the input. On success that's
//     Result{Value: n, Err: nil}; on a parse failure that's
//     Result{Value: 0, Err: <non-nil error>} - and parse must keep
//     going and process every remaining reading regardless.
//
//   - reciprocal receives parse's output. If the incoming Result
//     already carries an error, reciprocal must pass it through
//     completely unchanged (never overwrite an existing error, never
//     even look at a Value that was never valid to begin with).
//     Otherwise, since a true reciprocal isn't representable as an
//     int, reciprocal's job is simply to validate Value != 0 and pass
//     Value through unchanged, setting Err to ErrDivideByZero when
//     Value == 0. Either way, reciprocal must keep going and process
//     every remaining Result regardless of what it just saw.
//
//   - ProcessReadings wires parse -> reciprocal and drains the final
//     channel into a []Result to return: one Result per input
//     reading, same order as input.
//
// All three must keep respecting the done channel throughout, the
// same as every other pipeline stage you've built in this repo.
//
// The signatures must stay the same:
//
//     func parse(done <-chan struct{}, readings []string) <-chan Result
//     func reciprocal(done <-chan struct{}, in <-chan Result) <-chan Result
//     func ProcessReadings(done <-chan struct{}, readings []string) []Result
//

package main

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrDivideByZero is the sentinel error reciprocal sets on a Result
// whose Value is 0. The naive implementation below never actually
// reaches the point of setting it, because it gives up on the whole
// batch before getting there.
var ErrDivideByZero = errors.New("cannot compute reciprocal of zero")

// Result carries either a successfully processed Value (with Err ==
// nil) or a failure (with Err != nil, in which case Value should be
// ignored).
type Result struct {
	Value int
	Err   error
}

// parse is supposed to convert every reading to an int, emitting one
// Result per reading - success or failure - without ever stopping
// early. Right now it gives up on the entire remaining batch the
// moment it hits the first reading that doesn't parse.
func parse(done <-chan struct{}, readings []string) <-chan Result {
	out := make(chan Result)
	go func() {
		defer close(out)
		for _, reading := range readings {
			n, err := strconv.Atoi(reading)
			if err != nil {
				// BUG: abort the whole batch on the first bad
				// reading instead of reporting it and continuing.
				return
			}

			select {
			case <-done:
				return
			case out <- Result{Value: n}:
			}
		}
	}()
	return out
}

// reciprocal is supposed to pass through any Result that already
// carries an error unchanged, and otherwise validate Value != 0,
// setting Err to ErrDivideByZero when it is. Right now it gives up on
// every remaining Result the moment it sees a single Value of 0.
func reciprocal(done <-chan struct{}, in <-chan Result) <-chan Result {
	out := make(chan Result)
	go func() {
		defer close(out)
		for res := range in {
			if res.Value == 0 {
				// BUG: abort the whole batch on the first
				// divide-by-zero instead of reporting it and
				// continuing.
				return
			}

			select {
			case <-done:
				return
			case out <- res:
			}
		}
	}()
	return out
}

// ProcessReadings runs readings through the parse -> reciprocal
// pipeline and drains the result into a slice, one Result per input
// reading, in the same order as readings.
func ProcessReadings(done <-chan struct{}, readings []string) []Result {
	out := reciprocal(done, parse(done, readings))

	results := make([]Result, 0, len(readings))
	for res := range out {
		results = append(results, res)
	}
	return results
}

func main() {
	done := make(chan struct{})
	defer close(done)

	readings := []string{"10", "abc", "5", "0", "-3", "xyz", "0", "7"}

	results := ProcessReadings(done, readings)

	fmt.Printf("got %d results for %d readings\n", len(results), len(readings))
	for i, res := range results {
		fmt.Printf("  [%d] %q -> %+v\n", i, readings[i], res)
	}
}
