# Pipeline Error Handling: Result Values Instead of Aborting

Given is a small two-stage pipeline that takes a batch of raw
sensor-reading strings, parses each one to an int (`parse`), then
checks each parsed value for a divide-by-zero condition
(`reciprocal`). Two things can go wrong per reading: it might not
parse as an integer at all, or it might be exactly `0`, which the
downstream `reciprocal` step can't handle.

Right now, the moment either stage hits **one** bad reading, it just
gives up on the entire rest of the batch: the stage's goroutine
returns early (closing its output channel) instead of reporting the
problem and moving on. A single bad reading anywhere in a batch of,
say, 500 currently means every reading after it silently vanishes -
the caller has no way to tell "reading #2 was garbage" apart from
"the pipeline never even looked at readings #3 through #500".

Your task is to make `parse` and `reciprocal` let errors flow
downstream *alongside* values, as a `Result{Value, Err}` pair, instead
of aborting the whole batch or panicking on the first bad item - so
only the caller of `ProcessReadings` decides what to do about
failures.

```go
type Result struct {
	Value int
	Err   error
}
```

- **`parse(done <-chan struct{}, readings []string) <-chan Result`**
  must emit exactly one `Result` per input reading, in the same order
  as `readings`. On success that's `Result{Value: n, Err: nil}`; on a
  parse failure that's `Result{Value: 0, Err: <non-nil error>}`.
  `parse` must keep going and process every remaining reading
  regardless of what it just saw.

- **`reciprocal(done <-chan struct{}, in <-chan Result) <-chan Result`**
  receives `parse`'s output. If the incoming `Result` already carries
  an error, `reciprocal` must pass it through completely unchanged -
  never overwrite an existing error, never even look at a `Value`
  that was never valid to begin with. Otherwise, since a true
  reciprocal isn't representable as an `int`, `reciprocal`'s job here
  is simply to validate `Value != 0` and pass `Value` through
  unchanged, setting `Err` to the exported sentinel `ErrDivideByZero`
  when `Value == 0`. `reciprocal` must keep going and process every
  remaining `Result` regardless of what it just saw. (The exact
  numeric transform isn't the point of this exercise - per-item error
  propagation without aborting the batch is.)

- **`ProcessReadings(done <-chan struct{}, readings []string) []Result`**
  wires `parse -> reciprocal` and drains the final channel into a
  `[]Result` to return: one `Result` per input reading, same order as
  `readings`.

All three must keep respecting `done` throughout, the same as every
other pipeline stage you've built in this repo.

Keep the `Result` type and all three signatures identical, and keep
the exported sentinel:

```go
var ErrDivideByZero = errors.New(...)
```

for callers (and the tests) to check against with `errors.Is`.

## Test your solution

To complete this exercise, you must pass the tests:
```
go test
go test --race
```
