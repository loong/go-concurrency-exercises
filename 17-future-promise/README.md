# Future/Promise Pattern: Async, Memoized Computation

Given is a `Future` type that is supposed to represent an
asynchronous, memoized result: creating one should kick off
`ComputeExpensive` (see `mockcompute.go`) in the background and
return immediately, and `Get()` should block until the result is
ready, be safely callable from multiple goroutines at once, and only
ever trigger ONE underlying call to `ComputeExpensive` no matter how
many times `Get()` is called or from how many goroutines.

Right now `NewFuture` is not async at all - it calls
`ComputeExpensive` synchronously, on the calling goroutine, before
returning - so creating a `Future` blocks the caller for the full
150ms up front, defeating the entire point of a future (you can't do
other work while it's computing).

Your task is to fix `Future` so that:

- `NewFuture(key string) *Future` kicks off `ComputeExpensive(key)`
  in its own goroutine and returns near-instantly.
- `Get() int` blocks until the result is ready (e.g. via a channel
  that's closed once the result is stored, or a
  `sync.WaitGroup`/`sync.Once`) and is safe to call concurrently from
  many goroutines, and multiple times, always returning the same
  cached result without triggering any additional calls to
  `ComputeExpensive`.

The signatures must stay the same:

```go
func NewFuture(key string) *Future
func (f *Future) Get() int
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
