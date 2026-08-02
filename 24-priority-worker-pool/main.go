//////////////////////////////////////////////////////////////////////
//
// Given is a job scheduler with exactly one worker goroutine (that's
// deliberate: with a single worker, the order jobs complete in is
// fully determined by the order they're picked up, which makes this
// exercise testable without any ambiguity from multiple workers
// racing to grab the next job). The scheduler is supposed to run
// submitted jobs so that, whenever the worker is about to start a new
// one and more than one job is currently waiting, it always picks the
// HIGHEST-priority job waiting - so an urgent job submitted after a
// pile of low-priority ones doesn't have to sit behind all of them.
//
// This is a step up from the worker-pool exercise (see
// 11-worker-pool): that one hands every job the same treatment and
// only cares about draining a shared queue with several workers in
// parallel, in whatever order jobs happen to arrive. Here there's only
// one worker, but the queue itself has to be priority-aware: a plain
// FIFO channel can't reorder what's already sitting in it, so as soon
// as a job is queued behind others its priority stops mattering.
//
// The naive implementation below is exactly that: a single unbuffered
// (well, buffered, but that doesn't help) channel of Jobs that the one
// worker drains strictly in the order jobs were sent, with no regard
// for Priority at all. A job's only hope of running soon is to have
// been submitted early - however important it is.
//
// Your task is to reimplement Scheduler so that it keeps waiting jobs
// in an internal priority queue (container/heap is the natural tool)
// guarded by a mutex, instead of a plain channel. Whenever the worker
// finishes a job and needs to pick up the next one, it must take the
// highest-Priority job currently in the queue; ties are broken by
// earliest submission time, i.e. FIFO among jobs of equal priority.
// When the queue is empty the worker has to block efficiently (a
// sync.Cond, or a small signaling channel, either is fine - just don't
// busy-poll in a tight spin loop) until Submit adds something and
// wakes it up.
//
// The exported API must stay exactly the same, so Scheduler remains a
// drop-in replacement for the version below:
//
//     func NewScheduler() *Scheduler
//     func (s *Scheduler) Submit(job Job)
//     func (s *Scheduler) Completed() <-chan int
//

package main

import (
	"fmt"
	"time"
)

// Job is a unit of work to run. Higher Priority means more important.
type Job struct {
	ID       int
	Priority int // higher number = more important
}

// Scheduler is supposed to run submitted jobs on a single worker,
// always picking the HIGHEST-priority job currently waiting whenever
// the worker is about to start a new one - so an urgent job submitted
// after a pile of low-priority ones doesn't have to wait behind all of
// them. Right now it's a plain FIFO: a single buffered channel that
// the one worker drains strictly in the order jobs were sent, with no
// regard for Priority at all.
type Scheduler struct {
	jobs chan Job
	done chan int // completed job IDs, in the order they finished
}

// NewScheduler creates a Scheduler and starts its single worker.
func NewScheduler() *Scheduler {
	s := &Scheduler{
		jobs: make(chan Job, 100),
		done: make(chan int, 100),
	}
	go s.worker()
	return s
}

func (s *Scheduler) worker() {
	for job := range s.jobs {
		time.Sleep(20 * time.Millisecond) // simulated work
		s.done <- job.ID
	}
}

// Submit enqueues a job to be run.
func (s *Scheduler) Submit(job Job) {
	s.jobs <- job
}

// Completed returns the channel of completed job IDs, in completion
// order.
func (s *Scheduler) Completed() <-chan int {
	return s.done
}

func main() {
	s := NewScheduler()

	for i := 1; i <= 5; i++ {
		s.Submit(Job{ID: i, Priority: 1})
	}

	time.Sleep(5 * time.Millisecond)
	s.Submit(Job{ID: 6, Priority: 10})

	for i := 0; i < 6; i++ {
		id := <-s.Completed()
		fmt.Printf("job %d completed\n", id)
	}
}
