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

// jobLatency mirrors the simulated work duration in Scheduler.worker,
// so tests can reason about timing without depending on the exact
// constant used inside main.go.
const jobLatency = 20 * time.Millisecond

// readCompletions reads exactly n job IDs off sched.Completed(), in
// the order they arrive, failing the test if any read doesn't show up
// within a generous timeout. That timeout is a safety net against a
// hang - the scheduler's single, never-shutdown worker always makes
// progress, so there's no deadlock risk here, only a hard bound on how
// long a broken implementation gets to keep the test alive.
func readCompletions(t *testing.T, sched *Scheduler, n int) []int {
	t.Helper()

	got := make([]int, 0, n)
	for i := 0; i < n; i++ {
		select {
		case id := <-sched.Completed():
			got = append(got, id)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for completion %d/%d", i+1, n)
		}
	}

	return got
}

func indexOf(ids []int, id int) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

// TestSchedulerCompletesAllJobs checks that every submitted job
// eventually completes exactly once. It makes no assumption about
// ordering, so it passes against both the naive FIFO scheduler and a
// priority-aware one.
func TestSchedulerCompletesAllJobs(t *testing.T) {
	sched := NewScheduler()

	const n = 10
	priorities := []int{5, 1, 3, 9, 1, 7, 2, 2, 8, 4}
	for i := 0; i < n; i++ {
		sched.Submit(Job{ID: i + 1, Priority: priorities[i]})
	}

	got := readCompletions(t, sched, n)

	seen := make(map[int]bool, n)
	for _, id := range got {
		if seen[id] {
			t.Errorf("job %d completed more than once", id)
		}
		seen[id] = true
	}

	for i := 0; i < n; i++ {
		id := i + 1
		if !seen[id] {
			t.Errorf("job %d never completed", id)
		}
	}
}

// TestSchedulerPrioritizesUrgentJobs is the key test: it asserts that
// a high-priority job submitted while a backlog of low-priority jobs
// is already waiting jumps the queue instead of waiting behind all of
// them.
//
// Job 1 is submitted first, so the single worker picks it up almost
// immediately and is "busy" running it for jobLatency (20ms). Shortly
// after (well before job 1's 20ms is up), five low-priority jobs
// (2-6) are submitted, followed by one high-priority job (7). By the
// time job 7 is submitted, jobs 2-7 are genuinely queued and waiting,
// not raced for.
//
// On the naive FIFO scheduler, job 7 completes dead last (7th, after
// jobs 2-6). On a correct priority scheduler, job 7 completes as soon
// as whatever was already running finishes - i.e. it appears at index
// 0 or 1 in the completion order, never after any of jobs 2-6. The
// assertion below checks exactly that property rather than pinning
// down an exact position for job 1, since whether job 1 or job 7 is
// picked up first depends on timing that isn't worth pinning down
// precisely - only that job 7 is not stuck behind the backlog.
func TestSchedulerPrioritizesUrgentJobs(t *testing.T) {
	sched := NewScheduler()

	sched.Submit(Job{ID: 1, Priority: 1})
	time.Sleep(5 * time.Millisecond) // let the worker pick job 1 up

	for id := 2; id <= 6; id++ {
		sched.Submit(Job{ID: id, Priority: 1})
	}
	sched.Submit(Job{ID: 7, Priority: 10})

	got := readCompletions(t, sched, 7)

	idx := indexOf(got, 7)
	if idx < 0 {
		t.Fatalf("job 7 never completed; order was %v", got)
	}
	if idx > 1 {
		t.Errorf("job 7 (priority 10) completed at position %d in %v; "+
			"want it within the first two completions, i.e. right after whatever "+
			"was already running - not stuck behind the low-priority backlog (jobs 2-6)",
			idx, got)
	}
}

// TestSchedulerTiesBrokenByArrivalOrder checks that jobs of equal
// priority are run in the order they were submitted (FIFO among ties)
// rather than being reordered arbitrarily by the priority queue.
//
// All jobs here, including job 1, share the same priority, so it
// doesn't matter whether the worker has already picked job 1 up by
// the time jobs 2-6 are submitted: the expected completion order is
// 1..6 either way, since a same-priority job already queued (or
// already running) always precedes one submitted later.
func TestSchedulerTiesBrokenByArrivalOrder(t *testing.T) {
	sched := NewScheduler()

	sched.Submit(Job{ID: 1, Priority: 5})
	time.Sleep(5 * time.Millisecond) // let the worker pick job 1 up

	for id := 2; id <= 6; id++ {
		sched.Submit(Job{ID: id, Priority: 5})
	}

	got := readCompletions(t, sched, 6)

	want := []int{1, 2, 3, 4, 5, 6}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("completion order = %v, want %v (same-priority jobs must finish "+
				"in submission order)", got, want)
		}
	}
}

// TestSchedulerRace stress-tests the scheduler with many jobs and
// mixed priorities submitted concurrently from several goroutines, to
// catch data races on the internal queue (run with `go test -race`).
func TestSchedulerRace(t *testing.T) {
	sched := NewScheduler()

	const n = 50
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(id int) {
			sched.Submit(Job{ID: id, Priority: id % 5})
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}

	seen := make(map[int]bool, n)
	for i := 0; i < n; i++ {
		select {
		case id := <-sched.Completed():
			seen[id] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for completion %d/%d", i+1, n)
		}
	}

	if len(seen) != n {
		t.Errorf("expected %d distinct completed jobs, got %d", n, len(seen))
	}
}
