package main

import (
	"errors"
	"time"
)

// JobLatency is how long a single call to RunJob takes. It stands in for
// whatever expensive work a real job would do (calling an API, touching
// a disk, crunching numbers, ...).
const JobLatency = 80 * time.Millisecond

// ErrJobFailed is returned by RunJob for any Job whose ShouldFail field
// is set.
var ErrJobFailed = errors.New("job failed")

// Job is a single unit of work to be processed by the worker pool.
type Job struct {
	ID         int
	ShouldFail bool
}

// RunJob simulates doing the work described by j. It always takes
// JobLatency to run, and deterministically returns ErrJobFailed when
// j.ShouldFail is true, nil otherwise - so tests can check correctness
// without needing a real backend.
func RunJob(j Job) error {
	time.Sleep(JobLatency)

	if j.ShouldFail {
		return ErrJobFailed
	}

	return nil
}
