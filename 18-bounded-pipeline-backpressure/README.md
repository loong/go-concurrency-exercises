# Bounded Pipeline with Backpressure

Given is a fast producer feeding a slow consumer through `SlowConsume`
(see `mockslowconsumer.go`), which simulates a slow downstream sink
(e.g. writing to a remote store) by sleeping for 50ms per item.
`RunPipeline` is supposed to stream items from the producer to the
consumer through a *bounded* channel, so that if the consumer falls
behind, the producer is forced to slow down too - backpressure -
instead of piling up unboundedly-buffered, unconsumed work in memory.
Right now it does the opposite: it produces every item into a giant
buffered channel sized to hold the entire run, before the consumer
even gets a chance to start, so the "fast producer, slow consumer"
mismatch never actually pushes back on the producer - it just silently
buffers everything.

Your task is to fix `RunPipeline` so the channel between producer and
consumer has a small, fixed buffer (e.g. size 2) instead of one sized
to the full item count, so that once the buffer (plus the one item the
consumer may be actively processing) is full, the producer's next send
blocks until the consumer drains an item - i.e. real backpressure. The
function signature must stay the same:

```go
func RunPipeline(itemCount int, produced func(i int), consumed func(i int))
```

`produced` and `consumed` must each be called exactly once per item
(`produced(i)` right when item `i` is generated, `consumed(i)` right
when item `i` finishes being passed to `SlowConsume`), and may be
called from different goroutines - the callbacks the tests pass in do
their own synchronization, so `RunPipeline` doesn't need to worry
about that itself.

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
