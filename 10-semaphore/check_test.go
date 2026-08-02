//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func testReqs(n int) []string {
	reqs := make([]string, n)
	for i := range reqs {
		reqs[i] = fmt.Sprintf("request-%d", i)
	}

	return reqs
}

// TestFetchAllCorrectness checks that every request comes back with a
// successful (non-error) result when the API has plenty of headroom,
// so concurrency limiting can never cause a rejection here. It passes
// against both the naive implementation and a semaphore-bounded one.
func TestFetchAllCorrectness(t *testing.T) {
	api := NewFlakyAPI(100)
	reqs := testReqs(10)

	got := FetchAll(api, reqs)

	if len(got) != len(reqs) {
		t.Fatalf("expected %d results, got %d", len(reqs), len(got))
	}

	for i, res := range got {
		if strings.HasPrefix(res, "ERROR:") {
			t.Errorf("result for %q = %q, want a successful response", reqs[i], res)
		}
		if want := fmt.Sprintf("response-for-%s", reqs[i]); res != want {
			t.Errorf("result for %q = %q, want %q", reqs[i], res, want)
		}
	}
}

// TestFetchAllRespectsSemaphoreLimit is the key test: it builds an API
// that only tolerates 3 concurrent calls before it starts rejecting
// them, then fetches 12 requests through FetchAll. It asserts that the
// API's own high-water mark never exceeded 2 - i.e. FetchAll's own
// self-imposed limit kicked in comfortably below the API's rejection
// threshold - and that no result came back as an error.
//
// This fails against the naive "fire everything at once" version:
// with 12 goroutines all calling api.Call simultaneously, the
// high-water mark shoots up to around 12 and several results come
// back with "ERROR: too many concurrent requests". It passes once
// FetchAll bounds concurrency with its own semaphore to something
// strictly below the API's limit of 3.
func TestFetchAllRespectsSemaphoreLimit(t *testing.T) {
	const apiMaxConcurrent = 3
	const wantMaxHighWaterMark = 2

	api := NewFlakyAPI(apiMaxConcurrent)
	reqs := testReqs(12)

	got := FetchAll(api, reqs)

	if len(got) != len(reqs) {
		t.Fatalf("expected %d results, got %d", len(reqs), len(got))
	}

	if hwm := api.HighWaterMark(); hwm > wantMaxHighWaterMark {
		t.Errorf("API high-water mark = %d, want <= %d; FetchAll is not "+
			"bounding its own concurrency strictly below the API's limit of %d",
			hwm, wantMaxHighWaterMark, apiMaxConcurrent)
	}

	for i, res := range got {
		if strings.HasPrefix(res, "ERROR:") {
			t.Errorf("result for %q = %q; a properly bounded FetchAll should "+
				"never trip the API's own rejection logic", reqs[i], res)
		}
	}
}

// TestFetchAllStillConcurrent guards against the trivial "fix" of
// making FetchAll process requests one at a time, which would
// technically keep the API's high-water mark low but defeats the
// entire point of a semaphore: bounding concurrency, not eliminating
// it. With an API that has generous headroom (so no calls are ever
// rejected), 10 requests at CallLatency each take 1s run
// sequentially; a semaphore-bounded but still-concurrent FetchAll
// should finish in a small fraction of that.
//
// synctest.Test runs the body on a fake clock that jumps forward as
// soon as every goroutine in the bubble is durably blocked, so this
// assertion is exact and doesn't flake on a busy machine.
func TestFetchAllStillConcurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := NewFlakyAPI(100)
		reqs := testReqs(10)

		start := time.Now()
		got := FetchAll(api, reqs)
		elapsed := time.Since(start)

		if len(got) != len(reqs) {
			t.Fatalf("expected %d results, got %d", len(reqs), len(got))
		}

		const sequentialTime = 10 * CallLatency
		const budget = 600 * time.Millisecond

		if elapsed >= budget {
			t.Errorf("FetchAll took %s (sequential would take %s); want well "+
				"under %s - looks like requests are being made one at a time "+
				"instead of concurrently", elapsed, sequentialTime, budget)
		}
	})
}
