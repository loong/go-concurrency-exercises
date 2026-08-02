//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

// TestHandleRequestSucceedsWithAmpleTimeout checks the happy path: with
// a timeout comfortably larger than all three layers combined
// (3*LayerLatency = 900ms), HandleRequest should run the full chain and
// return the concatenated result with no error. This holds for both the
// naive implementation and a properly fixed one.
func TestHandleRequestSucceedsWithAmpleTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		got, err := HandleRequest(ctx)
		if err != nil {
			t.Fatalf("HandleRequest returned unexpected error: %v", err)
		}

		if want := "ABC"; got != want {
			t.Errorf("HandleRequest = %q, want %q", got, want)
		}
	})
}

// TestHandleRequestRespectsTimeout is the key test. It gives
// HandleRequest a context that times out after 200ms - less than even
// a single layer's 300ms latency. A correct implementation propagates
// ctx into every layer call and stops as soon as the context is done,
// so it should return an error (the deadline being exceeded) after
// roughly 200ms.
//
// The naive implementation ignores ctx and calls every layer with
// context.Background() instead, so all three layers run to completion
// regardless of the timeout: it takes roughly 3*LayerLatency = 900ms
// and very likely returns a successful result with no error at all -
// exactly the bug this exercise is about.
func TestHandleRequestRespectsTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := HandleRequest(ctx)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("HandleRequest returned no error; want an error from the "+
				"200ms timeout expiring (took %s)", elapsed)
		}

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("HandleRequest error = %v, want it to wrap context.DeadlineExceeded", err)
		}

		const budget = 350 * time.Millisecond
		if elapsed >= budget {
			t.Errorf("HandleRequest took %s to fail, want well under %s "+
				"(close to the 200ms timeout) - looks like ctx isn't being "+
				"propagated into the layer calls, so they run to completion "+
				"regardless of the caller's deadline", elapsed, budget)
		}
	})
}

// TestHandleRequestCancelledMidway starts HandleRequest with a
// cancellable context and cancels it partway through the chain (350ms
// in, while layer B would be in flight). A correct implementation
// notices the cancellation and returns promptly instead of continuing
// on to (or finishing) layer C.
func TestHandleRequestCancelledMidway(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		type result struct {
			val string
			err error
		}
		resCh := make(chan result, 1)

		start := time.Now()
		go func() {
			val, err := HandleRequest(ctx)
			resCh <- result{val, err}
		}()

		time.Sleep(350 * time.Millisecond)
		cancel()

		// Receiving from resCh durably blocks this goroutine on a
		// bubble channel, so the fake clock keeps advancing (running
		// out any remaining layer timers) until HandleRequest's
		// goroutine sends its result - there is no wall-clock wait
		// involved, however long that takes in fake time.
		res := <-resCh
		elapsed := time.Since(start)

		if res.err == nil {
			t.Errorf("HandleRequest returned no error after cancellation; want an error")
		}

		const budget = 500 * time.Millisecond
		if elapsed >= budget {
			t.Errorf("HandleRequest took %s to return after cancellation, want well "+
				"under %s - looks like it kept running later layers instead of "+
				"giving up promptly once ctx was cancelled", elapsed, budget)
		}
	})
}
