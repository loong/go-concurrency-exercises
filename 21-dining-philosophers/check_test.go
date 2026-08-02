//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"testing"
	"time"
)

// dineWithTimeout runs Dine(numPhilosophers, mealsToEat) on its own
// goroutine and waits for it to finish, bounded by a real-clock
// timeout.
//
// This deliberately does NOT use testing/synctest: the naive
// implementation in main.go doesn't just run slowly, it deadlocks -
// every philosopher goroutine ends up durably blocked forever,
// waiting on a fork that will never be released. synctest.Test
// detects exactly that condition ("every goroutine in the bubble is
// durably blocked") and reacts by panicking with "deadlock: ...",
// which would crash the whole test binary and fail every test in
// this file, not just this one - a confusing result for an exercise
// that is *about* deadlock.
//
// Instead, we run the simulation on a plain goroutine and signal
// completion over a channel. If Dine deadlocks, that goroutine simply
// never sends and is leaked - but that's harmless: go test doesn't
// wait around for leaked goroutines, it moves on as soon as the test
// function returns, so the timeout below produces a clean, fast,
// readable failure instead of a hang or a panic.
func dineWithTimeout(t *testing.T, numPhilosophers, mealsToEat int, timeout time.Duration) int32 {
	t.Helper()

	done := make(chan int32, 1)
	go func() {
		done <- Dine(numPhilosophers, mealsToEat)
	}()

	select {
	case total := <-done:
		return total
	case <-time.After(timeout):
		t.Fatalf("deadlock: dinner did not complete within %s - philosophers are stuck "+
			"waiting on forks that will never be released (every philosopher grabbed their "+
			"left fork first, so everyone is waiting on their neighbor's right fork)", timeout)
		return 0
	}
}

// TestDineCompletesWithoutDeadlock is the key test. It relies on the
// well-known reliability of the "everyone grabs their left fork
// first" race actually manifesting under Go's scheduler: when all N
// philosopher goroutines start at roughly the same time, they very
// reliably all succeed in picking up their left fork before any of
// them tries for their right fork, so the naive version deadlocks on
// essentially every single run. This test times out (and fails) on
// the naive implementation, and passes quickly once fork acquisition
// order no longer permits a circular wait.
func TestDineCompletesWithoutDeadlock(t *testing.T) {
	const numPhilosophers = 5
	const mealsToEat = 10

	total := dineWithTimeout(t, numPhilosophers, mealsToEat, 2*time.Second)

	want := int32(numPhilosophers * mealsToEat)
	if total != want {
		t.Errorf("Dine(%d, %d) = %d meals eaten, want %d", numPhilosophers, mealsToEat, total, want)
	}
}

// TestDineWithVaryingTableSizes stress-tests the fix across a few
// different table sizes (including the edge case of just two
// philosophers sharing two forks) to make sure the fix generalizes
// instead of being hardcoded to one specific N.
func TestDineWithVaryingTableSizes(t *testing.T) {
	const mealsToEat = 5

	for _, numPhilosophers := range []int{2, 5, 8} {
		numPhilosophers := numPhilosophers
		t.Run("", func(t *testing.T) {
			total := dineWithTimeout(t, numPhilosophers, mealsToEat, 2*time.Second)

			want := int32(numPhilosophers * mealsToEat)
			if total != want {
				t.Errorf("Dine(%d, %d) = %d meals eaten, want %d", numPhilosophers, mealsToEat, total, want)
			}
		})
	}
}

// Run these tests with `go test -race` too - a correct fix only ever
// touches each Fork's mutex and the shared meals-eaten counter through
// atomic.AddInt32, so it should be inherently race-free.
