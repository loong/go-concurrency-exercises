package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// CallLatency is how long a single successful call to FlakyAPI.Call takes.
const CallLatency = 100 * time.Millisecond

// ErrTooManyConcurrentRequests is returned by FlakyAPI.Call when more than
// maxConcurrent calls are in flight at the same instant.
var ErrTooManyConcurrentRequests = errors.New("too many concurrent requests")

// FlakyAPI simulates a downstream service with a strict limit on how many
// requests it can handle at the same time. If more than maxConcurrent calls
// to Call are in flight simultaneously, the excess calls immediately fail
// with ErrTooManyConcurrentRequests instead of being queued. Calls that are
// accepted take CallLatency to complete.
//
// FlakyAPI also tracks the high-water mark of concurrent in-flight calls it
// has ever observed, via HighWaterMark, purely for test instrumentation -
// a real API wouldn't expose this.
type FlakyAPI struct {
	mu            sync.Mutex
	current       int
	highWaterMark int
	maxConcurrent int
}

// NewFlakyAPI returns a FlakyAPI that rejects calls once more than
// maxConcurrent of them are in flight at once.
func NewFlakyAPI(maxConcurrent int) *FlakyAPI {
	return &FlakyAPI{maxConcurrent: maxConcurrent}
}

// Call simulates a single request to the downstream API for req. If
// accepting this call would push the number of in-flight calls above
// maxConcurrent, it returns ErrTooManyConcurrentRequests immediately.
// Otherwise it blocks for CallLatency and returns a canned response.
func (a *FlakyAPI) Call(req string) (string, error) {
	a.mu.Lock()
	a.current++
	if a.current > a.highWaterMark {
		a.highWaterMark = a.current
	}
	tooMany := a.current > a.maxConcurrent
	a.mu.Unlock()

	if tooMany {
		a.mu.Lock()
		a.current--
		a.mu.Unlock()

		return "", ErrTooManyConcurrentRequests
	}

	time.Sleep(CallLatency)

	a.mu.Lock()
	a.current--
	a.mu.Unlock()

	return fmt.Sprintf("response-for-%s", req), nil
}

// HighWaterMark returns the largest number of calls to Call that this
// FlakyAPI has ever observed in flight at the same instant, whether or not
// those calls were ultimately accepted or rejected.
func (a *FlakyAPI) HighWaterMark() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.highWaterMark
}
