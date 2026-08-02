# Worker Pool: Batch Job Processor — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `11-worker-pool/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`ProcessJobs` is handed a slice of `Job`s, each run by calling `RunJob`
(`mockjobs.go`), which sleeps for `JobLatency` (80ms) and then
succeeds or fails with `ErrJobFailed` depending on the job's
`ShouldFail` field. The naive version runs every job sequentially, one
at a time, on the calling goroutine:

```go
func ProcessJobs(jobs []Job) []Result {
	results := make([]Result, 0, len(jobs))
	for _, j := range jobs {
		err := RunJob(j)
		results = append(results, Result{JobID: j.ID, Err: err})
	}
	return results
}
```

The task is to replace this with a fixed-size pool of long-lived
worker goroutines that pull `Job`s off a shared jobs channel, run
`RunJob` on each, and write a `Result` back for every job — with a
coordinating goroutine (backed by a `sync.WaitGroup`) closing the
results channel once every worker has finished, so the caller can
drain results without knowing in advance how many workers there are or
when they'll finish. Every job must be captured exactly once (no
skips, no duplicates, no data races), though the order of `Result`s in
the returned slice does not need to match the order of the input
`Job`s. The signature — `func ProcessJobs(jobs []Job) []Result` — must
stay exactly the same.

## Why the naive version is wrong

To be precise about what's actually broken here: nothing about the
naive version is *incorrect*. It has no data race (only one goroutine
ever touches `results` or calls `RunJob`), and every job's outcome ends
up in the returned slice exactly once — the naive version's own doc
comment says as much, and the tests confirm it:

```
--- FAIL: TestProcessJobsConcurrency (0.00s)
    check_test.go:96: ProcessJobs took 1.6s (sequential would take 1.6s);
        want well under 500ms - looks like jobs are being processed one at
        a time on a small fixed-size pool instead of concurrently
FAIL
```

`TestProcessJobsCorrectness` and `TestProcessJobsRace` both pass
unmodified against the naive version above — there is no correctness
bug or race to fix. The actual
defect is pure wasted concurrency: because every `RunJob` call happens
on the same goroutine, one after another, 20 independent jobs at 80ms
each take a full 1.6s in strict lockstep, even though none of them
depends on any of the others finishing first. `TestProcessJobsConcurrency`
is the only test that catches this, and it does so on timing alone (via
`synctest`'s fake clock, so it's exact rather than flaky): it demands
the batch finish in well under the 1.6s a sequential run would take.
The fix, then, is purely about *how much work happens at once*, not
about *whether the result is correct* — which is exactly why a naive
per-goroutine fan-out (spawning all 20 goroutines directly) would also
"fix" the test, but a bounded worker pool is what the exercise actually
asks for, since real workloads often need a cap on how many jobs run
concurrently regardless of how many jobs there are.

## Approach 1: Fixed-size worker pool, fan-in through a results channel

```go
const numWorkers = 4

type Result struct {
	JobID int
	Err   error
}

func ProcessJobs(jobs []Job) []Result {
	jobCh := make(chan Job)
	resultCh := make(chan Result)

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for j := range jobCh {
				err := RunJob(j)
				resultCh <- Result{JobID: j.ID, Err: err}
			}
		}()
	}

	go func() {
		for _, j := range jobs {
			jobCh <- j
		}
		close(jobCh)
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]Result, 0, len(jobs))
	for r := range resultCh {
		results = append(results, r)
	}
	return results
}
```

Three pieces of machinery, each doing one job:

- **`jobCh`** is the shared work queue. `numWorkers` long-lived worker
  goroutines all `range` over it, so each job sent into `jobCh` gets
  picked up by exactly one worker — Go guarantees a value sent on an
  unbuffered channel is received by exactly one receiver, never
  duplicated and never silently dropped, which is what gives "every
  job captured exactly once" for free instead of needing to reason
  about it by hand.
- **A feeder goroutine** sends every job from the input slice into
  `jobCh` and then `close`s it. Closing is what lets each worker's
  `for j := range jobCh` loop terminate cleanly once the queue is
  drained, rather than blocking forever waiting for one more job.
- **`resultCh`** is the fan-in point: every worker writes its outcome
  into the same channel as it finishes a job, and the caller drains it
  with `for r := range resultCh`. A second coordinating goroutine calls
  `wg.Wait()` (blocking until all `numWorkers` workers have called
  `Done`) and then closes `resultCh` — this has to run on its own
  goroutine, because closing `resultCh` unblocks the `for r := range
  resultCh` loop below it, and that loop can't itself return until
  `resultCh` closes. Doing the `wg.Wait()` inline on the main goroutine
  instead would deadlock: it would block waiting for workers that are
  in turn blocked trying to send on `resultCh`, which nothing is
  draining yet.

Since results only ever leave a worker through `resultCh` — never
through a shared slice each worker writes into directly — there's
nothing to race on, which is why `TestProcessJobsRace` passes under
`-race` even with 50 jobs across several runs.

## Approach 2: Fixed-size worker pool, direct indexed writes into a pre-sized slice (alternative)

```go
const numWorkers = 4

type Result struct {
	JobID int
	Err   error
}

func ProcessJobs(jobs []Job) []Result {
	results := make([]Result, len(jobs))

	jobCh := make(chan Job)

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for j := range jobCh {
				err := RunJob(j)
				results[j.ID] = Result{JobID: j.ID, Err: err}
			}
		}()
	}

	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	wg.Wait()

	return results
}
```

Instead of fanning results in through a channel, this pre-sizes a
`results` slice to `len(jobs)` up front and has each worker write
straight into `results[j.ID]` as it finishes a job — no `resultCh`, no
separate closer goroutine to coordinate it, and no drain loop; the
main goroutine just feeds `jobCh` directly and blocks on `wg.Wait()`
before returning `results`.

This is race-free *for a subtler reason than Approach 1's*: it isn't
that only one goroutine ever touches `results` (many do, concurrently)
— it's that every job's `ID` is unique, so no two workers ever write to
the *same element* of the slice at the same time, and concurrent
writes to different elements of the same slice are not a data race in
Go's memory model. `go test -race` confirms this holds across several
runs of the 50-job stress test.

**This only works because of a precondition specific to this
exercise, not a general property of `ProcessJobs`.** `results[j.ID]`
requires every `Job.ID` in the input to be a distinct value in `[0,
len(jobs))` — true for both `testJobs` in `check_test.go` and the
`jobs` slice built in `main`, which both number jobs `0..n-1`
contiguously, but not something the function signature `func
ProcessJobs(jobs []Job) []Result` promises or enforces. Hand this
version a `[]Job` with IDs like 100, 200, 300, or with duplicate or
negative IDs, and it panics with an index-out-of-range (or silently
overwrites one job's result with another's) where the naive version
and Approach 1 both handle arbitrary IDs without issue, since neither
one ever uses `ID` as anything but a label to carry through to
`Result`. Treat Approach 1 as the version to reach for by default;
Approach 2 is worth knowing as a legitimate lock-free technique for
exactly the case where "job ID doubles as a dense array index" is
already guaranteed elsewhere in your system.

## Key takeaways

- Not every "naive" starting point is a correctness bug. Here it was a
  purely sequential-vs-concurrent throughput problem — recognizing that
  distinction (via a race detector and a passing correctness test)
  matters as much as recognizing an actual bug does.
- A worker pool decouples "how many jobs there are" from "how many of
  them run at once": `numWorkers` stays fixed regardless of whether
  `jobs` has 20 elements or 20,000, unlike a naive one-goroutine-per-job
  fan-out.
- `close(jobCh)` after feeding is what lets `for j := range jobCh` in
  each worker terminate; `wg.Wait()` followed by `close(resultCh)` in
  its own goroutine is what lets the *caller's* `for r := range
  resultCh` terminate. Two different channels, two different close
  points, each unblocking a different consumer.
- Fan-in through a channel (Approach 1) and direct indexed writes into
  a pre-sized slice (Approach 2) are both legitimately race-free, but
  for different reasons — single-writer-per-channel-send versus
  disjoint-memory-per-index — and the second one quietly narrows the
  function's contract to inputs where IDs are dense and unique. Know
  which guarantee you're actually relying on before you ship it.
