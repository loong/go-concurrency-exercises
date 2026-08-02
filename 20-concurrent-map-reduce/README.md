# Concurrent Map-Reduce: Parallel Word Count

Given is a function `WordCount` that is supposed to count word
occurrences across many text chunks, using the map-reduce pattern: a
"map" phase that processes each chunk independently and concurrently
into its own partial count, and a "reduce" phase that merges all the
partials into one final result - all without multiple goroutines ever
writing to the same shared map at the same time (which would race).
The current implementation instead processes chunks sequentially, one
at a time, merging each chunk's words directly into a single shared
result map as it goes - which is correct, but leaves all the
chunk-level parallelism on the table, since chunk 2 can't even start
being tokenized until chunk 1's tokenizing (simulated by
`time.Sleep(ProcessDelay)` per chunk) has fully finished.

Your task is to reimplement `WordCount` as true map-reduce: spawn one
goroutine per chunk (the "map" phase) that tokenizes its own chunk
into its own local `map[string]int` (after `time.Sleep(ProcessDelay)`,
simulating the per-chunk cost), with no shared mutable state touched
during this phase. Then, once all map-phase goroutines have finished
(a `sync.WaitGroup`, or fanning the partials in over a channel of
maps, both work), a single "reduce" step merges every partial map into
one final result sequentially, on one goroutine, so no concurrent
writes to the final map are ever needed. The function signature must
stay the same:

```go
func WordCount(chunks []string) map[string]int
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
