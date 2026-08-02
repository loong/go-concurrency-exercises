# Worker Pool: Batch Job Processor

Given is a batch job processor. It is handed a slice of `Job`s, each of
which is run by calling `RunJob`, which simulates some unreliable unit
of work by sleeping for a fixed amount of time and then either
succeeding or failing with `ErrJobFailed`, depending on the `Job`'s
`ShouldFail` field. The current implementation, `ProcessJobs`, runs
every job sequentially, one at a time, on the calling goroutine - so
the total time grows linearly with the number of jobs, even though
each call to `RunJob` is completely independent of the others.

Simply spawning one goroutine per job would work, but it's wasteful
when jobs are numerous or when the number of things running at once
needs to be capped - for example to avoid overwhelming a downstream
service with more concurrent requests than it can handle. What you
want instead is a worker pool: a small, fixed number of long-lived
worker goroutines that pull jobs off a shared queue one at a time, for
as long as there is work left to do.

Your task is to change `ProcessJobs` so that it starts a fixed-size
pool of worker goroutines that read `Job`s off a shared jobs channel,
run `RunJob` on each one, and write a `Result` back for every job they
process. A coordinating goroutine must close the results channel once
every worker has finished, so the caller can drain results into the
returned slice without knowing in advance how many workers there are
or when they'll be done.

Every job must be captured exactly once - no job skipped, no job
processed twice, no data races - and the order of `Result`s in the
returned slice does not need to match the order of `Job`s in the
input. The function signature must stay the same:

```go
func ProcessJobs(jobs []Job) []Result
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
