//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"hash/fnv"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// expectedResult mirrors the (deterministic) result-generation logic
// in ComputeExpensive, without paying for the simulated latency, so
// tests can check correctness cheaply.
func expectedResult(key string) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))

	return int(h.Sum64() % 1_000_000)
}

// TestNewFutureReturnsImmediately asserts that NewFuture kicks off the
// computation in the background and returns near-instantly, instead
// of blocking the caller for the full ComputeLatency. It fails
// against the naive implementation, which calls ComputeExpensive
// synchronously before returning.
func TestNewFutureReturnsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ResetCallCount()

		start := time.Now()
		f := NewFuture("k")
		elapsed := time.Since(start)

		if elapsed >= 10*time.Millisecond {
			t.Errorf("NewFuture took %s (ComputeExpensive takes %s); "+
				"want near-instant construction - looks like NewFuture is "+
				"computing the result synchronously instead of in the background",
				elapsed, ComputeLatency)
		}

		_ = f.Get()
	})
}

// TestFutureGetReturnsCorrectResult checks that Get returns the same
// deterministic value ComputeExpensive would directly produce for the
// same key. It passes against both the naive and a correct fixed
// implementation.
func TestFutureGetReturnsCorrectResult(t *testing.T) {
	ResetCallCount()

	f := NewFuture("k")
	got := f.Get()

	if want := expectedResult("k"); got != want {
		t.Errorf("Get() = %d, want %d", got, want)
	}
}

// TestFutureGetMemoizesAcrossManyCallers stress-tests Get from many
// concurrent goroutines and asserts that they all observe the same
// result AND that the underlying expensive computation ran exactly
// once, no matter how many callers raced to fetch it. This guards
// against a "fixed" implementation that makes NewFuture async but
// naively recomputes the result inside Get on every call instead of
// caching it. Run with `go test -race`.
func TestFutureGetMemoizesAcrossManyCallers(t *testing.T) {
	ResetCallCount()

	f := NewFuture("k")

	const callers = 20

	var (
		mu      sync.Mutex
		results = make([]int, 0, callers)
		wg      sync.WaitGroup
	)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			got := f.Get()

			mu.Lock()
			results = append(results, got)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(results) != callers {
		t.Fatalf("expected %d results, got %d", callers, len(results))
	}

	want := results[0]
	for i, got := range results {
		if got != want {
			t.Errorf("results[%d] = %d, want %d (all callers must observe the same cached result)", i, got, want)
		}
	}

	if cc := CallCount(); cc != 1 {
		t.Errorf("ComputeExpensive ran %d times across %d concurrent Get callers, want exactly 1", cc, callers)
	}
}
