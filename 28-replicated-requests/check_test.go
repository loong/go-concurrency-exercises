//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// TestFetchFastestReturnsFastestValue checks that FetchFastest returns
// the value (and lack of error) from the fastest replica, and that it
// does so in roughly the fastest replica's own latency - not after
// waiting for the slower ones too. It then uses the mocks'
// observability hook to check, within a short bound after the winner
// returned, that every losing replica actually observed its done
// being closed and exited early - this is what catches the naive bug,
// since the naive version answers correctly but leaks. Uses
// testing/synctest's fake clock so both timing assertions are
// deterministic rather than flaky.
func TestFetchFastestReturnsFastestValue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fast := NewMockReplica("fast", 10*time.Millisecond)
		slow1 := NewMockReplica("slow1", 100*time.Millisecond)
		slow2 := NewMockReplica("slow2", 200*time.Millisecond)
		slow3 := NewMockReplica("slow3", 300*time.Millisecond)

		done := make(chan struct{})

		start := time.Now()
		got, err := FetchFastest(done, fast.Replica, slow1.Replica, slow2.Replica, slow3.Replica)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "fast" {
			t.Fatalf("FetchFastest returned %q, want %q (the fastest replica's value)", got, "fast")
		}
		if elapsed >= 50*time.Millisecond {
			t.Errorf("FetchFastest took %s, want close to the fastest replica's 10ms latency, "+
				"not the slower replicas' latency too", elapsed)
		}

		// Give the losers a short moment (in fake time, well short of
		// even the second-fastest loser's 100ms latency) to react to
		// losing the race before checking whether they noticed at all.
		time.Sleep(5 * time.Millisecond)

		for _, loser := range []*MockReplica{slow1, slow2, slow3} {
			if !loser.ObservedCancellation() {
				t.Errorf("replica %q never observed its done channel being closed - "+
					"it is going to keep running all the way to its full artificial "+
					"latency instead of stopping as soon as it lost the race", loser.name)
			}
			if loser.RanToCompletion() {
				t.Errorf("replica %q ran to completion instead of being cancelled early", loser.name)
			}
		}

		// Let any still-leaked goroutine (from a naive implementation
		// that never cancels its losers) actually run out its
		// artificial latency and finish before this synctest bubble
		// closes: synctest requires every goroutine started inside
		// the bubble to have exited by the time this function
		// returns, or it panics with a deadlock report.
		time.Sleep(400 * time.Millisecond)
	})
}

// TestFetchFastestReturnsWinnerError checks that FetchFastest
// propagates the winning replica's own error alongside its value,
// rather than discarding it (e.g. `return res.value, nil` instead of
// `return res.value, res.err`).
func TestFetchFastestReturnsWinnerError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		wantErr := errors.New("boom")
		fast := NewMockReplica("fast", 10*time.Millisecond).WithError(wantErr)
		slow := NewMockReplica("slow", 100*time.Millisecond)

		done := make(chan struct{})

		got, err := FetchFastest(done, fast.Replica, slow.Replica)

		if got != "fast" {
			t.Fatalf("FetchFastest returned %q, want %q (the fastest replica's value)", got, "fast")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("FetchFastest returned err=%v, want the fastest replica's own error %v - "+
				"the winner's error must be returned alongside its value, not discarded", err, wantErr)
		}

		// Give the loser time to react to losing the race before this
		// synctest bubble closes.
		time.Sleep(150 * time.Millisecond)
	})
}

// TestFetchFastestClosedDoneCancelsEarly checks that passing an
// already-closed done returns promptly with an error and no winner,
// without waiting on any replica - and that every in-flight replica is
// also told to stop in that case, not just when a replica wins the
// race. This is what catches an implementation that only wires
// cancellation into the "a replica won" branch of its select and
// forgets it on the "caller's done fired" branch.
func TestFetchFastestClosedDoneCancelsEarly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r1 := NewMockReplica("a", 50*time.Millisecond)
		r2 := NewMockReplica("b", 100*time.Millisecond)

		done := make(chan struct{})
		close(done)

		start := time.Now()
		got, err := FetchFastest(done, r1.Replica, r2.Replica)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("expected an error when done is already closed before calling "+
				"FetchFastest, got value %q with nil error", got)
		}
		if got != "" {
			t.Errorf("expected no winner value when cancelled up front, got %q", got)
		}
		if elapsed >= 5*time.Millisecond {
			t.Errorf("FetchFastest took %s with an already-closed done, want a near-instant return", elapsed)
		}

		// Give the replicas a short moment (in fake time, well short of
		// even the faster one's 50ms latency) to react to the caller's
		// done firing before checking whether they noticed at all.
		time.Sleep(5 * time.Millisecond)

		for _, r := range []*MockReplica{r1, r2} {
			if !r.ObservedCancellation() {
				t.Errorf("replica %q never observed its done channel being closed after the caller's "+
					"done fired with no winner - it is going to keep running all the way to its full "+
					"artificial latency instead of stopping as soon as the caller cancelled", r.name)
			}
			if r.RanToCompletion() {
				t.Errorf("replica %q ran to completion instead of being cancelled early", r.name)
			}
		}

		// Same reasoning as in TestFetchFastestReturnsFastestValue:
		// give any replica goroutines still running in the background
		// (naive implementation) time to finish before this synctest
		// bubble closes, so they don't trip synctest's deadlock check.
		time.Sleep(150 * time.Millisecond)
	})
}

// TestFetchFastestNoReplicas checks the degenerate case of calling
// FetchFastest with no replicas at all.
func TestFetchFastestNoReplicas(t *testing.T) {
	done := make(chan struct{})
	if _, err := FetchFastest(done); err == nil {
		t.Fatal("expected an error when no replicas are given, got nil")
	}
}

// TestFetchFastestConcurrentSafety hammers FetchFastest with many
// concurrent calls, each racing several mock replicas, to catch data
// races (run with `go test -race`).
func TestFetchFastestConcurrentSafety(t *testing.T) {
	const calls = 20

	var wg sync.WaitGroup
	wg.Add(calls)

	for i := 0; i < calls; i++ {
		go func() {
			defer wg.Done()

			r1 := NewMockReplica("x", 2*time.Millisecond)
			r2 := NewMockReplica("y", 5*time.Millisecond)
			r3 := NewMockReplica("z", 8*time.Millisecond)

			done := make(chan struct{})
			_, _ = FetchFastest(done, r1.Replica, r2.Replica, r3.Replica)
		}()
	}

	wg.Wait()
}
