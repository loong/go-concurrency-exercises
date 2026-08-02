//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"testing"
	"testing/synctest"
	"time"
)

// testJobs builds n jobs with a deterministic pattern of failures:
// every third job (0-indexed) fails.
func testJobs(n int) []Job {
	jobs := make([]Job, n)
	for i := range jobs {
		jobs[i] = Job{
			ID:         i,
			ShouldFail: i%3 == 0,
		}
	}

	return jobs
}

// checkResults asserts that got contains exactly one Result per job in
// jobs, with the right success/failure outcome for each.
func checkResults(t *testing.T, jobs []Job, got []Result) {
	t.Helper()

	if len(got) != len(jobs) {
		t.Fatalf("expected %d results, got %d", len(jobs), len(got))
	}

	byID := make(map[int]Result, len(got))
	for _, r := range got {
		if _, dup := byID[r.JobID]; dup {
			t.Errorf("job %d appeared more than once in the results", r.JobID)
		}
		byID[r.JobID] = r
	}

	for _, j := range jobs {
		r, ok := byID[j.ID]
		if !ok {
			t.Errorf("missing result for job %d", j.ID)
			continue
		}

		if j.ShouldFail && r.Err != ErrJobFailed {
			t.Errorf("job %d: expected ErrJobFailed, got %v", j.ID, r.Err)
		}
		if !j.ShouldFail && r.Err != nil {
			t.Errorf("job %d: expected nil error, got %v", j.ID, r.Err)
		}
	}
}

// TestProcessJobsCorrectness checks that every job that goes in comes
// back out exactly once, with the right outcome. It makes no
// assumption about ordering or timing, so it passes against both the
// naive sequential implementation and a worker-pool one.
func TestProcessJobsCorrectness(t *testing.T) {
	jobs := testJobs(20)

	got := ProcessJobs(jobs)

	checkResults(t, jobs, got)
}

// TestProcessJobsConcurrency asserts that ProcessJobs actually
// processes jobs using a pool of worker goroutines instead of running
// them one at a time. 20 jobs at 80ms each take 1.6s sequentially; a
// pool of a handful of long-lived workers finishes in roughly
// ceil(20/numWorkers) job latencies, well under that. synctest.Test
// runs the body on a fake clock that jumps forward as soon as every
// goroutine in the bubble is durably blocked, so this assertion is
// exact and doesn't flake on a busy machine.
func TestProcessJobsConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		jobs := testJobs(20)

		start := time.Now()
		got := ProcessJobs(jobs)
		elapsed := time.Since(start)

		checkResults(t, jobs, got)

		const sequentialTime = 20 * JobLatency
		const budget = 500 * time.Millisecond

		if elapsed >= budget {
			t.Errorf("ProcessJobs took %s (sequential would take %s); "+
				"want well under %s - looks like jobs are being processed one at a time "+
				"on a small fixed-size pool instead of concurrently", elapsed, sequentialTime, budget)
		}
	})
}

// TestProcessJobsRace stress-tests ProcessJobs across several runs
// with more jobs, to catch data races on any shared state used to
// dispatch jobs and collect results (run with `go test -race`).
func TestProcessJobsRace(t *testing.T) {
	for i := 0; i < 5; i++ {
		jobs := testJobs(50)

		got := ProcessJobs(jobs)

		checkResults(t, jobs, got)
	}
}
