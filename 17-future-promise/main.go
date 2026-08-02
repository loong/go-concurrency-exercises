//////////////////////////////////////////////////////////////////////
//
// Future is supposed to represent an asynchronous, memoized result:
// creating one should kick off ComputeExpensive (see mockcompute.go)
// in the background and return immediately, and Get() should block
// until the result is ready, be safely callable from multiple
// goroutines at once, and only ever trigger ONE underlying call to
// ComputeExpensive no matter how many times Get() is called or from
// how many goroutines.
//
// Right now NewFuture is not async at all - it calls ComputeExpensive
// synchronously, on the calling goroutine, before returning - so
// creating a Future blocks the caller for the full 150ms up front,
// defeating the entire point of a future (you can't do other work
// while it's computing).
//
// Your task is to fix Future so that:
//
//   - NewFuture(key string) *Future kicks off ComputeExpensive(key)
//     in its own goroutine and returns near-instantly.
//   - Get() int blocks until the result is ready (e.g. via a channel
//     that's closed once the result is stored, or a
//     sync.WaitGroup/sync.Once) and is safe to call concurrently
//     from many goroutines, and multiple times, always returning the
//     same cached result without triggering any additional calls to
//     ComputeExpensive.
//
// The signatures must stay the same:
//
//     func NewFuture(key string) *Future
//     func (f *Future) Get() int
//

package main

import (
	"fmt"
	"time"
)

// Future represents the result of an asynchronous computation that
// may not have finished yet.
type Future struct {
	result int
}

// NewFuture starts computing the result for key and returns a Future
// representing it.
func NewFuture(key string) *Future {
	return &Future{result: ComputeExpensive(key)}
}

// Get returns the result, blocking until it is ready.
func (f *Future) Get() int {
	return f.result
}

func main() {
	start := time.Now()
	f := NewFuture("report-42")
	constructTime := time.Since(start)

	fmt.Printf("NewFuture returned after %s\n", constructTime)

	result := f.Get()
	fmt.Printf("Result: %d (total elapsed %s)\n", result, time.Since(start))
}
