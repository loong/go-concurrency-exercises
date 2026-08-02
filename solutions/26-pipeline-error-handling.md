# Pipeline Error Handling: Result Values Instead of Aborting — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `26-pipeline-error-handling/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a two-stage pipeline, `parse -> reciprocal`, wired together by `ProcessReadings`:

```go
type Result struct {
	Value int
	Err   error
}

func parse(done <-chan struct{}, readings []string) <-chan Result
func reciprocal(done <-chan struct{}, in <-chan Result) <-chan Result
func ProcessReadings(done <-chan struct{}, readings []string) []Result
```

`parse` is supposed to convert every reading to an int and emit one `Result` per reading, success or failure, never stopping early. `reciprocal` is supposed to pass through any `Result` that already carries an error unchanged, and otherwise validate `Value != 0`, setting `Err` to the exported sentinel `ErrDivideByZero` when it is. Either way, both stages must keep processing every remaining reading no matter what they just saw, and `ProcessReadings` must always return exactly one `Result` per input reading, in order.

## Why the naive version is wrong

Both stages currently `return` from their goroutine — closing their output channel — the instant they hit a single bad reading, instead of reporting the problem on that one `Result` and moving on to the rest of the batch:

```go
func parse(done <-chan struct{}, readings []string) <-chan Result {
	out := make(chan Result)
	go func() {
		defer close(out)
		for _, reading := range readings {
			n, err := strconv.Atoi(reading)
			if err != nil {
				return // BUG: abort the whole batch
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
```

`reciprocal` has the same shape, `return`-ing the moment it sees a `Value` of `0`. The `Err` field on `Result` is declared but never actually set by either stage — a bad reading doesn't get reported, it just makes everything after it vanish.

Verified: running `go test -v ./...` (with `GOROOT` pointed at the go1.25.6 toolchain the repo's `go.mod` requires) against this naive `main.go` fails the four tests that exercise per-item error handling, and the failure messages are diagnostic about exactly what's wrong. The suite also has two tests dedicated to `done`-cancellation (`TestPipelineStopsPromptlyOnDone` and `TestPipelineNoGoroutineLeakRace`); those pass even against the naive version below, because its bug is confined to aborting on a *bad item* - the `select { case <-done: ...; case out <- res: }` send guard on every good item is already correct and unaffected by the bug this exercise is about:

```
=== RUN   TestAllReadingsProduceResults
    check_test.go:60: ProcessReadings(8 readings) returned 1 results, want 8 - looks like the pipeline aborted early instead of reporting each bad reading and continuing
--- FAIL: TestAllReadingsProduceResults (0.00s)
=== RUN   TestGoodReadingsSucceed
    check_test.go:77: no result for reading 2 ("5"): pipeline aborted early instead of reporting the error and continuing
--- FAIL: TestGoodReadingsSucceed (0.00s)
=== RUN   TestBadReadingsCarryErrors
    check_test.go:101: no result for reading 1 ("abc"): pipeline aborted early instead of reporting the error and continuing
--- FAIL: TestBadReadingsCarryErrors (0.00s)
=== RUN   TestProcessingContinuesPastBadReadings
    check_test.go:145: no result for reading 2 ("5"), which comes after a bad reading: the pipeline stopped processing instead of continuing past the error
--- FAIL: TestProcessingContinuesPastBadReadings (0.00s)
=== RUN   TestPipelineStopsPromptlyOnDone
--- PASS: TestPipelineStopsPromptlyOnDone (0.02s)
=== RUN   TestPipelineNoGoroutineLeakRace
--- PASS: TestPipelineNoGoroutineLeakRace (0.21s)
FAIL
```

(`TestBadReadingsCarryErrors` iterates over the `parseErrIndex`/`divZeroIndex` maps, so which specific bad index it names first can vary run to run since Go map iteration order is randomized - the diagnostic message and outcome are identical regardless of which index happens to be reported.)

With `readings := []string{"10", "abc", "5", "0", "-3", "xyz", "0", "7"}`, `parse` converts `"10"` fine, then hits `"abc"` at index 1 and gives up — `reciprocal` never even sees indices 2 through 7. `go run .` against the naive version confirms it: `got 1 results for 8 readings`.

## Approach: emit a `Result` for every item, keep the loop running

The fix removes the early `return` from each stage's failure branch and replaces it with "build the right `Result`, then keep looping":

```go
package main

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrDivideByZero is the sentinel error reciprocal sets on a Result
// whose Value is 0.
var ErrDivideByZero = errors.New("cannot compute reciprocal of zero")

// Result carries either a successfully processed Value (with Err ==
// nil) or a failure (with Err != nil, in which case Value should be
// ignored).
type Result struct {
	Value int
	Err   error
}

// parse converts every reading to an int, emitting one Result per
// reading - success or failure - without ever stopping early.
func parse(done <-chan struct{}, readings []string) <-chan Result {
	out := make(chan Result)
	go func() {
		defer close(out)
		for _, reading := range readings {
			n, err := strconv.Atoi(reading)

			var res Result
			if err != nil {
				res = Result{Value: 0, Err: fmt.Errorf("invalid reading %q: %w", reading, err)}
			} else {
				res = Result{Value: n}
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

// reciprocal passes through any Result that already carries an error
// unchanged, and otherwise validates Value != 0, setting Err to
// ErrDivideByZero when it is. It keeps processing every remaining
// Result regardless of what it just saw.
func reciprocal(done <-chan struct{}, in <-chan Result) <-chan Result {
	out := make(chan Result)
	go func() {
		defer close(out)
		for res := range in {
			if res.Err == nil && res.Value == 0 {
				res.Err = ErrDivideByZero
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
```

Design notes:

- **Why `Result{Value, Err}` on a single channel, not a second `<-chan error` or a batch-level `error`.** A parallel error channel would split what is really one fact ("reading #3 is bad") into two independently-arriving values on two independently-closing channels, and the consumer would have to `select` over both to reassemble which error belongs to which value - exactly the kind of accidental complexity `select` in a hot loop invites bugs into. Collapsing failures into a single `error` returned by `ProcessReadings` is worse for this exercise's purpose: it can only report "the batch had at least one problem," which is the *aborting* failure mode in a different costume - the caller still can't tell "reading #2 was garbage" from "reading #500 was garbage" or how many succeeded. Keeping `Value` and `Err` paired in one struct riding one channel makes "one `Result` per input, in order" a property of the type itself, not something the consumer has to reconstruct.
- **The `for _, reading := range readings` and `for res := range in` loops never `return` on a bad item anymore — only on `<-done`.** That single change is the entire fix: once neither stage treats "this item is bad" as a reason to stop, `len(results) == len(readings)` falls out automatically, because every iteration of every loop either sends exactly one `Result` downstream or observes `done` and stops *both* stages together (a closed `done` still short-circuits the whole pipeline on purpose — it's the caller's own signal, not an error condition).
- **`reciprocal`'s guard is `res.Err == nil && res.Value == 0`, not just `res.Value == 0`.** A `Result` that already failed to parse always has `Value == 0` (the zero value), so checking `Value` alone without also checking that `Err` is still `nil` would silently overwrite a parse error with `ErrDivideByZero` — exactly the "never overwrite an existing error" rule the exercise calls out. `res` is a local copy pulled off the channel; mutating that copy in place and resending it keeps this a single send path instead of two near-duplicate branches (one for "pass through untouched", one for "pass through with `Err` set").
- **The naive bug is really two walls, and a partial fix only clears the first one.** Against the shipped `readings`, `parse` aborts at index 1 (`"abc"`) before `reciprocal` ever gets a chance to see a `Value` of `0` and abort on its own - so a student who fixes only `parse` and leaves `reciprocal`'s early `return` in place still only gets 1 result back, now for a different underlying reason. Both stages need the same treatment.
- **Errors are attached with `%w`, not just formatted into a string**, so a caller of `ProcessReadings` could still `errors.Is`/`errors.As` down to the original `*strconv.NumError` if they cared to, even though the exercise itself only asserts `Err != nil` for parse failures.
- **The pipeline shape itself (one goroutine per stage, connected by an unbuffered channel, each respecting `done` on both the receive and the send side) is unchanged from the naive version** — this exercise isn't about the concurrency plumbing, which was already correct; it's specifically about not treating a bad *item* the same way as a cancellation signal.

**Verified**: dropped this `main.go` into a throwaway scratch module (stdlib-only, no dependency on this repo's `go.mod`) alongside the real `check_test.go`, and ran `go build ./...`, `go vet ./...`, then `go test -race -count=5 ./...` — clean, no flakes, across all six tests, including the crux `TestProcessingContinuesPastBadReadings` and the `done`-cancellation pair (`TestPipelineStopsPromptlyOnDone`, `TestPipelineNoGoroutineLeakRace`), which stall both stages mid-send with nobody draining the output before closing `done`, so a `select` that dropped the `case <-done:` arm on the send (or dropped `done`-handling entirely) would hang or run the batch to completion instead of stopping promptly - either way the test fails fast rather than hanging, since every blocking read on the test side is itself guarded by a timeout. Confirmed separately that the *un-fixed* naive `main.go` in `26-pipeline-error-handling/` fails the four error-handling tests with the messages quoted above (the two `done`-cancellation tests pass against it, as expected), and that `go build .`, `go vet .`, and `gofmt -l .` are clean against the naive version too (the exercise must compile and run before a student touches it — it just has to be wrong).

## Key takeaways

- **A pipeline stage that can fail per-item should never let a bad item cancel the batch.** The book's `Result{Value, Err}` pattern turns "this item failed" into a value flowing down the same channel as everything else, instead of a control-flow event (`return`, `panic`, `log.Fatal`) that the stage's own goroutine reacts to. Only the code that started the pipeline — the one place that actually knows what "acceptable failure rate" or "what to do with a bad reading" means for its use case — gets to decide what happens next.
- **`done` and "this item is bad" are two completely different signals and must stay that way.** `done` means "the consumer stopped caring, stop everything now." A parse failure means "this one item is bad, tell me about it and keep going." Naive code that funnels both into the same `return` conflates them; the fix is exactly to stop doing that for the second case while keeping it for the first.
- **When a later stage's "is this valid" check could be confused by an earlier stage's failure sentinel values (like a parse failure's `Value` defaulting to `0`, the same value that's *also* invalid for a real reason), check the upstream `Err` field first.** Downstream stages must treat an already-failed `Result` as opaque and pass it through, not re-validate fields that were never meaningful in the first place.
- **A single `Result` per input item, always, is what makes a batch pipeline usable at scale.** As soon as the count of outputs can silently drop below the count of inputs, callers lose the ability to tell "item N failed" apart from "item N was never looked at" — which is precisely what the naive version's missing tail of results does.
