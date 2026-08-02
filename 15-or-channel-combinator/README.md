# Or-Channel Combinator: Combining Independent Shutdown Triggers

A service often needs to shut down as soon as ANY of several
independent triggers fires - a failed health check, an
admin-requested shutdown, a deadline expiring, ... . Each of these is
naturally represented as its own `<-chan struct{}` that gets closed
the moment that particular condition occurs. `or` is supposed to
combine an arbitrary, variadic number of such signal channels into a
single channel that callers can select on: it must close as soon as
ANY ONE of the input channels closes, no matter which one.

The current implementation ignores every channel except the first one
it was given - it only ever waits on `channels[0]`, so closing any
channel OTHER than `channels[0]` has no effect on the returned channel
at all.

Your task is to fix `or` so the returned channel closes as soon as ANY
of the input channels closes:

- With zero input channels there is nothing to wait on, so the
  simplest correct behavior is to return a channel that is never
  closed.
- With exactly one input channel, the fix is a trivial
  passthrough-via-relay.
- With more than one input channel, use the classic recursive
  divide-and-conquer idiom: watch `channels[0]` and `channels[1]`
  directly in a `select`, and recurse on `or(channels[2:]...)` as a
  third branch of that same `select`.

The function signature must stay the same:

```go
func or(channels ...<-chan struct{}) <-chan struct{}
```

so that it remains a drop-in replacement for the naive version below.

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
