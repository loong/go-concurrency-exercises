# Bounded Pipeline with Backpressure — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `18-bounded-pipeline-backpressure/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`RunPipeline(itemCount, produced, consumed)` is supposed to stream `itemCount` items from a fast producer to a slow consumer (`SlowConsume`, which sleeps 50ms per item — see `mockslowconsumer.go`) through a *bounded* channel, so that once the consumer falls behind, the producer is forced to slow down too instead of piling up unconsumed work in memory. `produced(i)` must fire the instant item `i` is generated and `consumed(i)` the instant item `i` finishes `SlowConsume`, each exactly once; the callbacks may run on different goroutines and do their own locking, so `RunPipeline` doesn't need to synchronize around them.

`check_test.go` enforces this with two tests:

- `TestRunPipelineProcessesAllItems` — every index in `[0, itemCount)` is produced and consumed exactly once. This one is timing-agnostic and passes against both the naive and the fixed version.
- `TestRunPipelineAppliesBackpressure` — runs under `synctest.Test` (a fake, deterministic clock that jumps forward once every goroutine in the bubble is durably blocked) and tracks, at the moment each `produced(i)` fires, how many items have already been fully consumed. The gap `i - consumedSoFar` measures how far ahead of the consumer the producer was allowed to race. The test requires `maxGap <= 5` for `itemCount = 20`.

## Why the naive version is wrong

```go
ch := make(chan int, itemCount) // unbounded: buffers the whole run
```

The channel's buffer is sized to `itemCount`, so every one of the 20 sends in the producer's loop completes immediately — the producer can finish emitting the entire run before the consumer, which pays a real 50ms per item, has even started on item 0. Verified directly against this test suite: the naive version produces a `maxGap` of 19 (item 19 is produced while 0 items have been consumed), which blows the `<= 5` assertion:

```
--- FAIL: TestRunPipelineAppliesBackpressure (0.00s)
    check_test.go:104: max gap between how far production got ahead of consumption = 19, want <= 5 ...
FAIL
```

This isn't just a "the producer is briefly ahead" nuance — it's a memory bug that scales with input. `make(chan int, itemCount)` means the buffer size (and therefore worst-case buffered-but-unconsumed work) grows linearly with however many items you throw at it, which defeats the entire point of streaming through a pipeline instead of materializing the whole run up front.

## Approach 1: small, fixed-size buffered channel

Replace `itemCount` with a small constant (2 in the reference solution). Once that many items (plus the one item the consumer may be actively processing) are in flight, the producer's next `ch <- i` blocks until the consumer drains one — that's the backpressure, and it holds regardless of `itemCount`.

```go
package main

import "fmt"

// boundedBufferSize is a small, fixed channel buffer between the
// producer and the consumer. Once this many items (plus the one item
// the consumer may be actively processing) are in flight, the
// producer's send blocks until the consumer drains one - that's the
// backpressure.
const boundedBufferSize = 2

// RunPipeline streams itemCount items (0..itemCount-1) from a fast
// producer to SlowConsume through a small, bounded channel. Once the
// buffer fills up, the producer's send blocks until the consumer
// drains an item, so a slow consumer naturally throttles a fast
// producer instead of letting it buffer the entire run in memory.
func RunPipeline(itemCount int, produced func(i int), consumed func(i int)) {
	ch := make(chan int, boundedBufferSize)

	go func() {
		defer close(ch)
		for i := 0; i < itemCount; i++ {
			produced(i)
			ch <- i // blocks once the buffer is full - backpressure
		}
	}()

	for i := range ch {
		SlowConsume(i)
		consumed(i)
	}
}

func main() {
	RunPipeline(20, func(i int) {
		fmt.Println("produced", i)
	}, func(i int) {
		fmt.Println("consumed", i)
	})
}
```

**Verified**: `go test -race ./...` passes both tests (repeated with `-count=3`); measured wall time ~4.9–5.0s for the full 20-item run, consistent with the consumer's fixed 50ms/item cost dominating.

## Tuning the buffer size: throughput vs. memory vs. jitter tolerance

This is a genuine parameter to reason about, but it's worth being precise about what it does and doesn't buy you, rather than reaching for the stock "bigger buffer = more throughput" line — that line is wrong for *this* pipeline.

**It doesn't change steady-state throughput here.** The consumer is a fixed 50ms/item and is the only bottleneck; the producer in this exercise does no real work of its own. So the total time to process `itemCount` items is `itemCount × 50ms` no matter what the buffer size is — verified directly: an unbuffered channel (`boundedBufferSize = 0`) and the size-2 default both completed the 20-item run in ~4.9s under `-race`. A buffer in front of a consumer that's uniformly slower than the producer doesn't make the consumer any faster; it just changes how much unconsumed work is allowed to queue up while waiting for the consumer.

**Where a buffer *does* help throughput** is when the producer is bursty or per-item production cost varies — e.g. it occasionally stalls (a network call, a lock, GC pause) but is otherwise fast. A buffer lets the consumer keep draining previously-produced items during that stall instead of going idle waiting on the next `ch <- i`. With a perfectly steady producer like this exercise's, there's no stall to absorb, so the buffer's only visible effect is the gap size, not the runtime.

**What the buffer size actually trades:**

- **Memory / staleness vs. jitter tolerance.** A larger buffer tolerates more producer burstiness before the producer has to block, at the cost of allowing more unconsumed (and therefore "stale" — representing older, not-yet-acted-on state) items to sit in memory at once.
- **The gap bound scales directly with buffer size**, and independent of `itemCount` — which is exactly the property the naive `itemCount`-sized buffer violates. Measured directly against `TestRunPipelineAppliesBackpressure` (`itemCount = 20`, assertion `maxGap <= 5`):

  | `boundedBufferSize` | max gap observed | test result |
  |---|---|---|
  | 0 (unbuffered) | 1 | PASS |
  | 2 (reference solution) | 3 | PASS |
  | 3 | 4 | PASS |
  | 4 | 5 | PASS (right at the edge) |
  | 5 | 6 | **FAIL** |
  | 6 | 7 | **FAIL** |

  The pattern is `maxGap == boundedBufferSize + 1` (buffer slots plus the one item the consumer is actively holding), and it's exactly linear — confirming the gap bound depends only on the buffer size, never on `itemCount`, unlike the naive version's `make(chan int, itemCount)`.
- **Unbuffered (`boundedBufferSize = 0`) is a valid, more extreme point on the same knob**, not a structurally different design — it's still "a small, fixed-size channel between producer and consumer," just at the size-0 end. It gives the tightest possible bound on how far the producer can get ahead (at most one item is ever "in flight" beyond what the consumer is processing), at the cost of zero tolerance for any producer jitter: every single send synchronizes directly with a receive.

In short: pick the buffer size based on how much producer burstiness you need to absorb and how much staleness/memory you can tolerate, not based on an assumption that a bigger buffer makes the pipeline faster — here it doesn't, because the consumer is the bottleneck either way.

## Key takeaways

- `make(chan int, itemCount)` isn't just "not bounded enough" — it's a bug whose worst-case memory footprint scales linearly with the input size, which is precisely what a bounded pipeline exists to avoid.
- The fix is not "add synchronization" (there's no race here — the naive version's tests even pass `TestRunPipelineProcessesAllItems`); it's sizing the channel deliberately, since Go channel buffer size *is* the backpressure control.
- `synctest.Test`'s fake clock makes `TestRunPipelineAppliesBackpressure` deterministic — it fast-forwards once every goroutine is durably blocked (e.g. asleep inside `SlowConsume`), so the gap measurement doesn't flake based on machine load.
- Buffer size is a real tuning knob, but only for absorbing producer burstiness/jitter and controlling memory/staleness — not for raw throughput when the consumer is a uniform bottleneck, which it is in this exercise. Don't over-claim what a bigger buffer gets you.
- If you need genuinely minimal producer lead, an unbuffered channel (buffer size 0) is a legitimate endpoint of the same "small, bounded channel" idea, not a separate architecture.
