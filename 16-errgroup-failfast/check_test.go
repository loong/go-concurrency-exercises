//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// TestGroupRunsAllTasksToCompletion checks that every task passed to
// Go actually runs to completion, and that Wait reports a non-nil
// error when at least one task failed. This is a correctness
// baseline that already holds for the naive sequential
// implementation too (it also runs every task, just one at a time) -
// that's fine, it just confirms the basics before the concurrency
// and race tests raise the bar.
func TestGroupRunsAllTasksToCompletion(t *testing.T) {
	var g Group

	var mu sync.Mutex
	count := 0

	const n = 10
	failing := map[int]bool{2: true, 5: true, 8: true}
	expectedErrs := make(map[error]bool)

	for i := 0; i < n; i++ {
		i := i
		var taskErr error
		if failing[i] {
			taskErr = fmt.Errorf("task %d failed", i)
			expectedErrs[taskErr] = true
		}

		g.Go(func() error {
			mu.Lock()
			count++
			mu.Unlock()
			return taskErr
		})
	}

	err := g.Wait()

	mu.Lock()
	got := count
	mu.Unlock()

	if got != n {
		t.Errorf("expected all %d tasks to run, only %d ran", n, got)
	}

	if err == nil {
		t.Fatal("expected Wait to return a non-nil error, got nil")
	}
	if !expectedErrs[err] {
		t.Errorf("Wait returned unexpected error: %v", err)
	}
}

// TestGroupRunsConcurrently is the key test: it asserts that Go
// actually launches tasks concurrently instead of running them
// synchronously. Ten tasks that each sleep 100ms take 1s if run one
// at a time, but well under that if run concurrently. synctest.Test
// runs the body on a fake clock that jumps forward as soon as every
// goroutine in the bubble is durably blocked, so this assertion is
// exact and doesn't flake on a busy machine.
func TestGroupRunsConcurrently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Group

		const n = 10
		start := time.Now()

		for i := 0; i < n; i++ {
			g.Go(func() error {
				time.Sleep(100 * time.Millisecond)
				return nil
			})
		}

		err := g.Wait()
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		const sequentialTime = n * 100 * time.Millisecond
		const budget = 300 * time.Millisecond

		if elapsed >= budget {
			t.Errorf("Wait took %s (sequential would take %s); want well under %s - "+
				"looks like Go is running tasks one at a time instead of concurrently",
				elapsed, sequentialTime, budget)
		}
	})
}

// TestGroupFirstErrorRaceSafe stress-tests the "first error wins"
// bookkeeping with many tasks that all fail immediately (no sleep, so
// they race to finish and to record their error at roughly the same
// time). Run with `go test -race` to catch any unsynchronized access
// to the shared "first error" field. The naive sequential
// implementation is - by construction - never concurrent, so it
// won't trip the race detector here; this test guards the concurrent
// fix, not just the naive bug.
func TestGroupFirstErrorRaceSafe(t *testing.T) {
	var g Group

	const n = 50
	expectedErrs := make(map[error]bool, n)

	for i := 0; i < n; i++ {
		err := fmt.Errorf("task %d failed", i)
		expectedErrs[err] = true

		g.Go(func() error {
			return err
		})
	}

	got := g.Wait()

	if got == nil {
		t.Fatal("expected Wait to return a non-nil error, got nil")
	}
	if !expectedErrs[got] {
		t.Errorf("Wait returned an error that doesn't match any task's error: %v", got)
	}
}
