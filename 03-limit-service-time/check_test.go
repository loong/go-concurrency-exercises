//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// The following two tests never leave a goroutine still running past the
// end of the test: FastProcessCompletes's process finishes well inside the
// quota and self-cancels, and PremiumUser_NeverKilled's premium branch
// calls process() synchronously with no timeout at all. That makes them
// safe to run in a synctest bubble - the fake clock advances instantly to
// each Sleep's end, and no goroutine is left needing a tick of fake time
// that never comes.

func TestHandleRequest_FreeUser_FastProcessCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		u := &User{ID: 1, IsPremium: false}
		done := false
		process := func() {
			time.Sleep(1 * time.Second)
			done = true
		}

		if ok := HandleRequest(process, u); !ok {
			t.Error("expected free user's process under 10s to complete, but it was killed")
		}
		if !done {
			t.Error("expected process to have run to completion")
		}
	})
}

func TestHandleRequest_PremiumUser_NeverKilled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		u := &User{ID: 3, IsPremium: true}
		done := false
		process := func() {
			time.Sleep(11 * time.Second)
			done = true
		}

		if ok := HandleRequest(process, u); !ok {
			t.Error("expected premium user's process to never be killed regardless of duration")
		}
		if !done {
			t.Error("expected process to have run to completion")
		}
	})
}

// Every test below this point involves a request that gets killed while
// process() is still running. Go has no way to forcibly stop a goroutine,
// so HandleRequest returning early always leaves that goroutine still
// asleep in the background. A synctest bubble only advances its fake clock
// while every goroutine is durably blocked, and it stops advancing the
// moment the bubble's root goroutine (the test function) returns - so a
// test that returns right after asserting on the kill leaves that straggler
// permanently blocked, and synctest.Test panics with a deadlock rather than
// letting the test finish. These stay on the real clock, same as the two
// genuinely concurrent tests further down (which have their own,
// mutex-related reason to stay there).

func TestHandleRequest_FreeUser_SlowProcessKilled(t *testing.T) {
	t.Parallel()
	u := &User{ID: 2, IsPremium: false}
	process := func() {
		time.Sleep(11 * time.Second)
	}

	if ok := HandleRequest(process, u); ok {
		t.Error("expected free user's process over 10s to be killed")
	}
}

// --- Advanced level: 10s quota accumulated across a free user's requests ---

func TestHandleRequest_FreeUser_AccumulatedTimeExceeded(t *testing.T) {
	t.Parallel()
	u := &User{ID: 4, IsPremium: false}

	// Uses 4s of the user's 10s lifetime quota.
	if ok := HandleRequest(func() { time.Sleep(4 * time.Second) }, u); !ok {
		t.Fatal("expected first 4s request to succeed")
	}

	// Uses another 4s, for 8s accumulated.
	if ok := HandleRequest(func() { time.Sleep(4 * time.Second) }, u); !ok {
		t.Fatal("expected second 4s request to succeed")
	}

	// Only 2s of quota remains, so a 4s request should be killed ~2s in,
	// not allowed to run the full 4s.
	start := time.Now()
	if ok := HandleRequest(func() { time.Sleep(4 * time.Second) }, u); ok {
		t.Error("expected third request to be killed once accumulated quota is exceeded")
	}
	if elapsed := time.Since(start); elapsed < 1*time.Second || elapsed > 3*time.Second {
		t.Errorf("expected request to be killed ~2s in, took %v", elapsed)
	}

	// Quota is now fully spent; a further request should be killed immediately
	// rather than waiting for any remaining timeout.
	start = time.Now()
	if ok := HandleRequest(func() { time.Sleep(1 * time.Second) }, u); ok {
		t.Error("expected request to be killed immediately once quota is fully used")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("expected immediate kill with no quota left, took %v", elapsed)
	}
}

func TestHandleRequest_FreeUser_QuotaIsPerUser(t *testing.T) {
	t.Parallel()
	u1 := &User{ID: 5, IsPremium: false}
	u2 := &User{ID: 6, IsPremium: false}

	// Exhaust u1's quota: 5s used, then a 6s request only has 5s left and
	// gets killed partway through.
	if ok := HandleRequest(func() { time.Sleep(5 * time.Second) }, u1); !ok {
		t.Fatal("expected u1's first 5s request to succeed")
	}
	if ok := HandleRequest(func() { time.Sleep(6 * time.Second) }, u1); ok {
		t.Error("expected u1's second request to be killed once its quota is exceeded")
	}

	// u2 has never made a request, so it should have a fresh 10s quota
	// unaffected by u1's usage.
	if ok := HandleRequest(func() { time.Sleep(3 * time.Second) }, u2); !ok {
		t.Error("expected u2's quota to be independent of u1's usage")
	}
}

// The following two tests fire genuinely concurrent requests at the same
// free user and rely on real mutex contention while one goroutine sleeps.
// They are intentionally left on the real clock (not synctest): a bubble
// only advances its fake clock once every goroutine is "durably blocked",
// and blocking on a sync.Mutex does not count as durably blocked. With one
// goroutine parked on HandleRequest's per-user mutex while another is
// durably blocked inside its own timeout wait, the bubble's clock would
// never advance and synctest.Test would hang rather than fail fast.

func TestHandleRequest_FreeUser_ConcurrentRequestsDoNotDoubleSpendQuota(t *testing.T) {
	t.Parallel()
	u := &User{ID: 7, IsPremium: false}

	// Fire two 6s requests at the same free user at once. Together they need
	// 12s, more than the 10s quota, so checking "is there time left?" and
	// spending it can't be two separate steps - a request must reserve its
	// share of the quota atomically, or both goroutines can read the quota
	// as available before either has recorded its usage.
	const concurrentRequests = 2
	results := make([]bool, concurrentRequests)
	var wg sync.WaitGroup
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = HandleRequest(func() { time.Sleep(6 * time.Second) }, u)
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, ok := range results {
		if ok {
			succeeded++
		}
	}
	if succeeded >= concurrentRequests {
		t.Errorf("expected at most one of %d concurrent 6s requests to succeed on a 10s quota, got %d succeed (results=%v, TimeUsed=%v)",
			concurrentRequests, succeeded, results, u.TimeUsed)
	}
}

// TestHandleRequest_FreeUser_ConcurrentRequestNotRejectedOnEarlierEarlyFinish
// asserts that a concurrent request is judged against the quota the earlier
// one *actually* used, not the worst case it could have used.
//
// A request that reserves its entire remaining quota up front (so a sibling
// can be rejected without waiting for it to finish) has no way to know in
// advance that the earlier request will only take 1s out of its 10s
// allotment - it must assume the worst case (it might run all the way to
// its own timeout) to stay safe. That means a second request arriving while
// the first is still in flight gets rejected even though, in reality,
// plenty of quota was available. This test only passes for a design that
// makes the second request wait for an accurate answer instead of guessing.
func TestHandleRequest_FreeUser_ConcurrentRequestNotRejectedOnEarlierEarlyFinish(t *testing.T) {
	t.Parallel()
	u := &User{ID: 9, IsPremium: false}

	var wg sync.WaitGroup
	var firstOK, secondOK bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		firstOK = HandleRequest(func() { time.Sleep(1 * time.Second) }, u)
	}()

	// Give the first request time to start and begin its 1s run, but start
	// the second well before it finishes - this is the overlap window where
	// a worst-case reservation would still be holding the whole quota.
	time.Sleep(200 * time.Millisecond)

	wg.Add(1)
	go func() {
		defer wg.Done()
		secondOK = HandleRequest(func() { time.Sleep(2 * time.Second) }, u)
	}()

	wg.Wait()

	if !firstOK {
		t.Fatal("expected the first 1s request to succeed")
	}
	if !secondOK {
		t.Error("expected the second request to succeed once the first's actual (short) usage is known - " +
			"1s + 2s is well within the 10s quota, so rejecting it means the decision was made on a worst-case " +
			"guess instead of the first request's real, now-known usage")
	}
}
