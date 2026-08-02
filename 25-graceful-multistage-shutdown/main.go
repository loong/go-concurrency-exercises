//////////////////////////////////////////////////////////////////////
//
// Given is a small, fixed-size pool of worker goroutines started by
// Start. Each worker reads items off a shared jobs channel and calls
// the caller-supplied process function on every item it receives,
// until jobs is closed. This is NOT about reacting to an OS signal
// like Ctrl-C (see the SIGINT exercise for that) - there is no signal
// involved here at all. It's about a purely internal handshake: once
// the producer has closed jobs, how does the pool tell the caller
// "every job that was ever submitted has now been fully processed,
// you may safely move on"?
//
// That matters because callers often want to tear something down
// right after the pool finishes - for example closing a downstream
// file, database connection, or network socket that process writes
// results into. Doing that teardown too early, while a worker is
// still mid-call to process on some job it already pulled off jobs,
// silently corrupts or drops whatever that in-flight call was about
// to do.
//
// Start is supposed to return a `done` channel that only closes once
// every job ever sent to `jobs` has been FULLY processed - i.e. it's
// safe for a caller to wait on `done` and then treat the whole
// pipeline as gracefully, completely finished. Right now it does
// nothing of the sort: the returned done channel is closed
// immediately, before any worker has necessarily even started, let
// alone processed a single job - so a caller who waits on it and
// then, say, tears down whatever process was writing results into,
// can easily do so while jobs are still silently being worked on in
// the background, silently dropping/losing whatever those in-flight
// calls to process were about to do.
//
// Your task is to fix Start so that the returned done channel closes
// ONLY once jobs has been closed AND every worker goroutine has fully
// returned from its `range jobs` loop (i.e. every worker has finished
// calling process on its last-received item). A sync.WaitGroup,
// incremented once per worker before it starts and marked Done when
// it returns from ranging over jobs, plus a small goroutine that
// calls wg.Wait() and then closes done, is the natural tool for this.
// The function signature must stay the same:
//
//     func Start(jobs <-chan int, process func(item int)) <-chan struct{}
//
// so that it remains a drop-in replacement for the naive version
// below.
//

package main

import (
	"fmt"
	"time"
)

const numWorkers = 4

// Start launches numWorkers worker goroutines that read from jobs and
// call process on each item until jobs is closed. It's supposed to
// return a `done` channel that only closes once every job that was
// ever sent to `jobs` has been FULLY processed - i.e. it's safe for a
// caller to wait on `done` and then treat the whole pipeline as
// gracefully, completely finished. Right now it does nothing of the
// sort: the returned done channel is closed immediately, before any
// worker has necessarily even started, let alone processed a single
// job - so a caller who waits on it and then, say, tears down
// whatever `process` was writing results into, can easily do so while
// jobs are still silently being worked on in the background, silently
// dropping/losing whatever those in-flight calls to process were
// about to do.
func Start(jobs <-chan int, process func(item int)) <-chan struct{} {
	for i := 0; i < numWorkers; i++ {
		go func() {
			for item := range jobs {
				process(item)
			}
		}()
	}

	done := make(chan struct{})
	close(done)
	return done
}

func main() {
	const numJobs = 20

	jobs := make(chan int)

	go func() {
		for i := 0; i < numJobs; i++ {
			jobs <- i
		}
		close(jobs)
	}()

	done := Start(jobs, func(item int) {
		// A little artificial latency makes it obvious, when
		// running this by hand, that "shutdown complete" below
		// can print well before every "processed N" line has.
		time.Sleep(10 * time.Millisecond)
		fmt.Println("processed", item)
	})

	<-done
	fmt.Println("shutdown complete")
}
