# Pipeline: Multi-Stage Number Processing — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `08-pipeline/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`Pipeline` is supposed to run numbers through three conceptual stages —
`generator` (emits the input numbers), `square` (squares each one), and
`keepEven` (keeps only the even results) — as a true streaming
pipeline: each stage its own goroutine, connected by channels, so that
`square` can start working on item 1 while `generator` is still
producing item 2, and so on down the line.

The given implementation does none of that. Every stage operates on a
plain `[]int` and returns a plain `[]int`:

```go
func generator(done <-chan struct{}, nums ...int) []int {
	return nums
}

func square(done <-chan struct{}, nums []int) []int {
	out := make([]int, len(nums))
	for i, n := range nums {
		SimulateWork()
		out[i] = n * n
	}
	return out
}

func keepEven(done <-chan struct{}, nums []int) []int {
	var out []int
	for _, n := range nums {
		SimulateWork()
		if n%2 == 0 {
			out = append(out, n)
		}
	}
	return out
}

func Pipeline(done <-chan struct{}, nums ...int) []int {
	return keepEven(done, square(done, generator(done, nums...)))
}
```

The task is to turn `generator`, `square`, and `keepEven` into stages
with these exact signatures, each running in its own goroutine and
reading/writing channels instead of slices:

```go
func generator(done <-chan struct{}, nums ...int) <-chan int
func square(done <-chan struct{}, in <-chan int) <-chan int
func keepEven(done <-chan struct{}, in <-chan int) <-chan int
```

`Pipeline`'s own signature — `func Pipeline(done <-chan struct{}, nums ...int) []int` —
must stay the same, since it's what callers and tests depend on.

## Why the naive version is wrong

As with the fan-out/fan-in exercise, "wrong" here isn't about producing
bad output — the naive, batch-per-stage version is fully correct. It
passes `TestPipelineCorrectness`, because `keepEven(square(generator(nums)))`
computes exactly the right answer regardless of whether the stages
overlap, and it passes `TestPipelineConcurrentUse` too, since each
concurrent call gets its own independent slices with no shared state to
race on.

What it fails is throughput and responsiveness:

```
--- FAIL: TestPipelineStagesOverlap (0.00s)
    check_test.go:79: Pipeline took 400ms to process 20 items; want well under 250ms - looks like generator, square, and keepEven are running one at a time over the whole batch instead of overlapping as a pipeline
--- FAIL: TestPipelineStopsEarly (0.00s)
    check_test.go:117: Pipeline took 2s to return after done was closed while processing 100 items; want well under 200ms - looks like the pipeline keeps working through the entire input instead of stopping once done fires
```

Two distinct defects, both consequences of materializing the whole
input as a slice at every stage:

1. **No overlap.** `generator` must finish producing every item before
   `square` sees any of them (`square` doesn't even start until
   `generator` has *returned*), and `square` must finish squaring every
   item before `keepEven` starts. With 20 items and `WorkLatency` paid
   per item per stage, that's roughly `3 * 20 * WorkLatency` — the
   stages run back-to-back instead of concurrently.
2. **No early exit.** `done` is accepted as a parameter but never
   actually looked at anywhere in the bodies of `generator`, `square`,
   or `keepEven` — there's no channel operation to `select` it against.
   Once the naive `Pipeline` starts, it grinds through all 100 items
   across all three stages no matter what, even though the caller gave
   up after the first `WorkLatency`.

## Approach 1: Channel-based stages, each watching `done` on every operation

Each stage is its own goroutine that creates its output channel,
processes its input (or the literal `nums` for `generator`) one item
at a time, and closes the output channel via `defer` when there's
nothing left to send. Every blocking channel operation — both
*receiving* from upstream and *sending* downstream — is wrapped in a
`select` alongside `<-done`, so a stage can never be stuck forever on
an operation nobody will complete.

```go
func generator(done <-chan struct{}, nums ...int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for _, n := range nums {
			SimulateWork()

			select {
			case out <- n:
			case <-done:
				return
			}
		}
	}()

	return out
}

func square(done <-chan struct{}, in <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for {
			select {
			case n, ok := <-in:
				if !ok {
					return
				}

				SimulateWork()

				select {
				case out <- n * n:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()

	return out
}

func keepEven(done <-chan struct{}, in <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for {
			select {
			case n, ok := <-in:
				if !ok {
					return
				}

				SimulateWork()

				if n%2 != 0 {
					continue
				}

				select {
				case out <- n:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()

	return out
}

func Pipeline(done <-chan struct{}, nums ...int) []int {
	numsCh := generator(done, nums...)
	squaresCh := square(done, numsCh)
	evensCh := keepEven(done, squaresCh)

	var out []int
	for n := range evensCh {
		out = append(out, n)
	}

	return out
}
```

Walking through the shape:

- **Overlap** falls straight out of using channels instead of slices:
  `generator` sends item 1 the moment it's ready, `square` can receive
  and start working on it immediately (no need to wait for `generator`
  to finish item 2, let alone the whole input), and the same relation
  holds between `square` and `keepEven`. Three stages running
  concurrently, connected by unbuffered channels, is exactly what a
  pipeline is.
- **`done` is checked on every single blocking point**, not just some
  of them: `square` and `keepEven` `select` on their *receive* from
  `in` (`case n, ok := <-in` vs. `case <-done`) and separately `select`
  on their *send* to `out` (`case out <- ...` vs. `case <-done`).
  `generator` only has a send to guard (its "input" is the literal
  `nums` slice, already fully in hand). Guarding both directions is
  what lets a stage unstick itself the instant `done` closes, whether
  it was blocked waiting for upstream or blocked waiting for
  downstream — it never depends on a neighboring stage noticing first.
- **Closing `out` via `defer`** on every exit path — normal exhaustion
  of `in` (`!ok`) or early return via `done` — is what lets downstream
  `range`/`select`-on-receive loops terminate instead of blocking
  forever, and it's what lets `Pipeline`'s final `for n := range evensCh`
  return once the chain has unwound.
- Because `done` is closed once, by the caller (`defer close(done)` in
  `main`, or the test's `close(done)`), and every stage watches the
  *same* channel directly, all three goroutines notice cancellation
  independently and at roughly the same time — no stage has to wait
  for its neighbor to react first.

## Approach 2: Draining upstream before exiting on `done`

A variant worth knowing, even though it isn't strictly required by
*this* exercise: instead of abandoning `in` the instant `done` fires,
`square` and `keepEven` first drain whatever is left on their input
channel before returning.

```go
func square(done <-chan struct{}, in <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for {
			select {
			case n, ok := <-in:
				if !ok {
					return
				}

				SimulateWork()

				select {
				case out <- n * n:
				case <-done:
					drain(in)
					return
				}
			case <-done:
				drain(in)
				return
			}
		}
	}()

	return out
}

func keepEven(done <-chan struct{}, in <-chan int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for {
			select {
			case n, ok := <-in:
				if !ok {
					return
				}

				SimulateWork()

				if n%2 != 0 {
					continue
				}

				select {
				case out <- n:
				case <-done:
					drain(in)
					return
				}
			case <-done:
				drain(in)
				return
			}
		}
	}()

	return out
}

// drain reads every remaining item from in until in is exhausted - a
// "don't leave the upstream sender blocked" cleanup pattern.
func drain(in <-chan int) {
	for range in {
		SimulateWork()
	}
}
```

(`generator` and `Pipeline` are unchanged from Approach 1 — `generator`
has no upstream `in` to drain, since its "input" is the literal `nums`
slice it already holds.)

The tradeoff this illustrates: `select`-on-every-operation (Approach 1)
is the leaner, more responsive design, but it implicitly relies on
*every* upstream stage also being well-behaved and watching the same
`done` — which happens to be true here, since all three stages are
ours and all three select on the same shared channel. In a pipeline
where a downstream consumer doesn't control (or can't fully trust) its
producer — a defensive variant of the same shutdown problem that Go's
"Pipelines and cancellation" pattern is concerned with — abandoning
`in` the moment `done` fires can leave the producer goroutine
permanently blocked on a send that will now never be received, i.e. a
goroutine leak. Draining `in` to completion before returning guarantees
the upstream sender always gets to finish (or itself notice `done` and
stop producing) — at the real cost of consuming the *entire remaining
input*, paying `SimulateWork()` for every item drained, before the
stage is allowed to exit. For a pipeline with a lot of remaining input
still upstream, that's not "slightly longer" — it's the stage staying
alive and doing (discarded) work for as long as it takes upstream to
run dry.

Both approaches pass the full test suite here, because every stage in
this particular pipeline already checks `done` on its own sends — there
is no untrusted upstream to protect against. Reach for Approach 1 when
you own (or trust) every stage in the pipeline and want the fastest,
simplest shutdown. Reach for the drain pattern when a stage's input may
come from a producer that isn't guaranteed to watch the same
cancellation signal, and leaving it blocked forever would be a real
leak rather than a theoretical one.

## Key takeaways

- Like the fan-out/fan-in exercise, the naive version here has a
  throughput/responsiveness bug, not a correctness bug —
  `TestPipelineCorrectness` passes against it, while
  `TestPipelineStagesOverlap` and `TestPipelineStopsEarly` fail because
  the batch-per-stage design has no way to overlap work or observe
  `done`.
- A pipeline stage is: create an output channel, spawn a goroutine that
  reads its input and writes its output, `defer close()` the output
  channel on every exit path. Chaining stages is just passing one
  stage's output channel as the next stage's input.
- `select` alongside `<-done` is needed on *every* blocking channel
  operation a stage performs — both receiving from upstream and
  sending downstream — not just one or the other. Guarding only the
  send (or only the receive) leaves a gap where a stage can still block
  indefinitely.
- Closing `done` once, and having every stage watch that same shared
  channel, means every stage can detect cancellation independently,
  with no stage depending on another to react first — that's what
  makes the immediate-abandon approach safe in this particular
  pipeline.
- Draining upstream input before exiting is a defensive pattern for
  when you can't guarantee an upstream producer is also watching
  `done` — it trades a little extra shutdown latency (and some
  now-wasted work) for a hard guarantee against leaking the producer
  goroutine.
