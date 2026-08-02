//////////////////////////////////////////////////////////////////////
//
// Given is a batch job processor. It is handed a slice of Jobs, each
// of which is run by calling RunJob (see mockjobs.go). RunJob
// simulates some unreliable unit of work - a call to a flaky
// downstream service, say - by sleeping for a fixed amount of time
// and then either succeeding or failing with ErrJobFailed, depending
// on the Job's ShouldFail field.
//
// The naive implementation below runs every job sequentially, one at
// a time, on the calling goroutine, recording each outcome as it
// goes. This is correct - every job's Result ends up in the returned
// slice exactly once - but it leaves all the available concurrency on
// the table: with jobs that are independent of one another, there is
// no reason to wait for job N to finish before starting job N+1.
//
// Simply firing off one goroutine per job (as in the fan-out/fan-in
// exercise) would work here too, but it is wasteful when jobs are
// numerous or when the number of things that may run at once needs to
// be capped - for example to avoid overwhelming a downstream service
// with more concurrent requests than it can handle. What you want
// instead is a worker pool: a small, FIXED number of long-lived
// worker goroutines that pull jobs off a shared queue one at a time,
// for as long as there is work left to do.
//
// Your task is to change ProcessJobs so that it starts a fixed-size
// pool of worker goroutines (say, numWorkers of them) that read Jobs
// off a shared jobs channel and run RunJob on each one, writing a
// Result back for every job they process. A coordinating goroutine
// must close the results channel once every worker has finished (a
// sync.WaitGroup is the natural tool for that), so that the caller
// can drain results into the returned slice without knowing in
// advance how many workers there are or when they'll be done.
//
// Every job must be captured exactly once: no job skipped, no job
// processed twice, and no data race on the way. The order of Results
// in the returned slice does NOT need to match the order of Jobs in
// the input. The function signature must stay the same:
//
//     func ProcessJobs(jobs []Job) []Result
//
// so that it remains a drop-in replacement for the sequential version
// below.
//

package main

import (
	"fmt"
	"time"
)

// Result is the outcome of running a single Job: its ID, and either
// nil (success) or ErrJobFailed.
type Result struct {
	JobID int
	Err   error
}

// ProcessJobs is supposed to run every job in jobs using a small,
// fixed-size pool of worker goroutines (so many jobs can be in flight
// at once without spawning one goroutine per job), collecting every
// job's outcome - success or failure - into the returned slice, one
// Result per job, regardless of how many jobs fail along the way.
// Right now it runs every job sequentially, one at a time, on the
// calling goroutine, which is correct but leaves all that available
// concurrency on the table.
func ProcessJobs(jobs []Job) []Result {
	results := make([]Result, 0, len(jobs))
	for _, j := range jobs {
		err := RunJob(j)
		results = append(results, Result{JobID: j.ID, Err: err})
	}
	return results
}

func main() {
	jobs := make([]Job, 0, 20)
	for i := 0; i < 20; i++ {
		jobs = append(jobs, Job{
			ID:         i,
			ShouldFail: i%3 == 0,
		})
	}

	start := time.Now()
	results := ProcessJobs(jobs)
	elapsed := time.Since(start)

	var succeeded, failed int
	for _, r := range results {
		if r.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	fmt.Printf("Processed %d jobs: %d succeeded, %d failed\n", len(results), succeeded, failed)
	fmt.Printf("Took %s\n", elapsed)
}
