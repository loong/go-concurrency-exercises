# Priority Worker Pool: Urgent Jobs Shouldn't Wait Behind a Backlog — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `24-priority-worker-pool/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a `Scheduler` with exactly one worker goroutine, backed by a single buffered channel:

```go
type Scheduler struct {
	jobs chan Job
	done chan int
}

func (s *Scheduler) worker() {
	for job := range s.jobs {
		time.Sleep(20 * time.Millisecond) // simulated work
		s.done <- job.ID
	}
}

func (s *Scheduler) Submit(job Job) {
	s.jobs <- job
}
```

`Scheduler` must:

- Keep waiting jobs in an internal priority queue (`container/heap` is the natural tool) guarded by a mutex, instead of a plain channel.
- Whenever the worker finishes a job and needs to pick up the next one, take the **highest-`Priority`** job currently queued.
- Break ties by earliest submission time — FIFO among jobs of equal priority.
- Block the worker efficiently (no busy-spin) when the queue is empty, waking as soon as `Submit` adds something.
- Keep the exported API unchanged: `func NewScheduler() *Scheduler`, `func (s *Scheduler) Submit(job Job)`, `func (s *Scheduler) Completed() <-chan int`.

## Why the naive version is wrong

`s.jobs` is a plain FIFO channel. Once a job is sent into it, its position relative to every other queued job is frozen — a job's only lever over how soon it runs is *when it was submitted*, never how important it is. A priority-10 job submitted while five priority-1 jobs are already sitting in the channel still has to wait behind all five, because the channel has no way to look past what's already queued and reorder it.

Verified: running the current `check_test.go` against the naive `main.go` in a throwaway scratch copy fails `TestSchedulerPrioritizesUrgentJobs`:

```
--- FAIL: TestSchedulerPrioritizesUrgentJobs (0.15s)
    check_test.go:119: job 7 (priority 10) completed at position 6 in [1 2 3 4 5 6 7]; want it within the first two completions, i.e. right after whatever was already running - not stuck behind the low-priority backlog (jobs 2-6)
FAIL
```

Job 7 (priority 10) completes dead last, exactly as the naive FIFO channel guarantees. The other two correctness tests (`TestSchedulerCompletesAllJobs`, `TestSchedulerTiesBrokenByArrivalOrder`) pass against the naive version too, since a strict FIFO channel is trivially "every job runs once" and "ties broken by arrival order" — it only fails the one test that actually exercises reordering by priority.

## A note on the test file: no `testing/synctest` here

Every other exercise's test file in this repo that deals with timing uses `testing/synctest` to fake the clock and run instantly. `24-priority-worker-pool/check_test.go` deliberately does **not** — `NewScheduler` starts a worker goroutine that loops forever (`for job := range s.jobs` in the naive version, and the equivalent unbounded `for { ... }` loop in a fixed version) with no shutdown API at all. `synctest.Test` requires every goroutine reachable from the bubble to eventually block or exit so the fake clock can determine the bubble is idle; a goroutine that runs forever with no way to signal "I'm done" can never let the bubble settle, so `synctest` simply doesn't work here.

Instead, the tests use real-clock waits sized against `jobLatency = 20 * time.Millisecond` (mirroring the worker's simulated-work sleep), with a `select` + a 2-second `time.After` safety net around every read from `Completed()` — not because a correct scheduler could ever deadlock (it can't: the worker always makes progress), but purely as a bound on how long a *broken* implementation gets to keep the test alive.

## Approach 1: `container/heap` + `sync.Cond`

```go
package main

import (
	"container/heap"
	"sync"
	"time"
)

// Job is a unit of work to run. Higher Priority means more important.
type Job struct {
	ID       int
	Priority int // higher number = more important
}

// queuedJob wraps a Job with the sequence number it arrived with, so
// ties in Priority can be broken by earliest submission (FIFO among
// equal priorities).
type queuedJob struct {
	job Job
	seq uint64
}

// jobHeap is a container/heap max-heap on Priority, with ties broken
// by the lower (earlier) seq.
type jobHeap []queuedJob

func (h jobHeap) Len() int { return len(h) }
func (h jobHeap) Less(i, j int) bool {
	if h[i].job.Priority != h[j].job.Priority {
		return h[i].job.Priority > h[j].job.Priority // max-heap on Priority
	}
	return h[i].seq < h[j].seq // FIFO among ties
}
func (h jobHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *jobHeap) Push(x any) { *h = append(*h, x.(queuedJob)) }

func (h *jobHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Scheduler runs submitted jobs on a single worker, always picking the
// HIGHEST-priority job currently waiting whenever the worker is about
// to start a new one, with ties broken by submission order. Waiting
// jobs live in an internal heap-backed priority queue guarded by mu;
// cond signals the worker when the queue goes from empty to
// non-empty, so it can block without busy-polling.
type Scheduler struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   jobHeap
	nextSeq uint64

	done chan int // completed job IDs, in the order they finished
}

// NewScheduler creates a Scheduler and starts its single worker.
func NewScheduler() *Scheduler {
	s := &Scheduler{
		done: make(chan int, 100),
	}
	s.cond = sync.NewCond(&s.mu)
	go s.worker()
	return s
}

func (s *Scheduler) worker() {
	for {
		s.mu.Lock()
		for len(s.queue) == 0 {
			s.cond.Wait()
		}
		qj := heap.Pop(&s.queue).(queuedJob)
		s.mu.Unlock()

		time.Sleep(20 * time.Millisecond) // simulated work
		s.done <- qj.job.ID
	}
}

// Submit enqueues a job to be run.
func (s *Scheduler) Submit(job Job) {
	s.mu.Lock()
	heap.Push(&s.queue, queuedJob{job: job, seq: s.nextSeq})
	s.nextSeq++
	s.mu.Unlock()
	s.cond.Signal()
}

// Completed returns the channel of completed job IDs, in completion
// order.
func (s *Scheduler) Completed() <-chan int {
	return s.done
}
```

Design notes:

- **`heap.Pop` and the `len(s.queue) == 0` check both happen while `mu` is held.** That's what makes "highest priority currently queued" an atomic, race-free statement — the worker's pick and any concurrent `Submit`'s push can never interleave in a way that lets the worker observe a stale view of the queue.
- **`for len(s.queue) == 0 { s.cond.Wait() }` is a loop, not an `if`.** `sync.Cond.Wait()` can (rarely) return without a corresponding `Signal` — the classic spurious-wakeup hazard — and more importantly, with only one worker here it's not actually needed for correctness beyond defensive style, but it's the idiomatic, always-correct pattern for consuming from a `Cond`-guarded queue and costs nothing.
- **`cond.Signal()` is called *after* `mu.Unlock()`.** This is a minor optimization, not a correctness requirement here (`sync.Cond` explicitly allows signaling with or without holding the lock) — it just avoids momentarily waking the worker only for it to immediately block again on the mutex.
- **The tie-break is encoded directly in `Less`**: compare `Priority` first (descending, for a max-heap), and only fall back to `seq` (ascending) when priorities are equal. Since `nextSeq` is only ever read/written under `mu`, and `Submit` assigns it in submission order, equal-priority jobs come out of the heap in exactly the order they were submitted.
- **`jobHeap.Pop()`/`Push()` operate on the slice directly** — this is the standard `container/heap` contract: the heap package calls these to move the last/first element, and the caller (`Submit`/`worker`) always goes through `heap.Push`/`heap.Pop`, never appends/truncates the slice directly, so heap invariants are never bypassed.

**Verified**: copied this exercise into a throwaway scratch directory, confirmed the naive `main.go` fails `TestSchedulerPrioritizesUrgentJobs` (see above), then dropped in this solution. `gofmt -l` is clean, `go vet ./...` is clean, and `go test -race -count=10 ./...` passes repeatably — including `TestSchedulerRace`, which submits 50 jobs from 50 concurrent goroutines to stress-test the heap/mutex pairing.

## Approach 2: fixed priority-tiered channels (alternative, lighter-weight for a small fixed set of levels)

A genuinely different mechanism worth knowing about when priorities come from a **small, fixed set of levels** rather than a continuous range: instead of one heap ordered by an arbitrary `Priority` value, keep one plain FIFO channel per priority tier, and have the worker scan the tiers from highest to lowest on every pick.

```go
package main

import (
	"fmt"
	"time"
)

// Job is a unit of work to run. Higher Priority means more important.
type Job struct {
	ID       int
	Priority int // higher number = more important
}

// numLevels bounds the fixed set of priority tiers this design
// supports: one dedicated channel per priority value 0..numLevels-1.
const (
	numLevels = 32
	levelBuf  = 256
)

// Scheduler runs submitted jobs on a single worker, always picking the
// HIGHEST-priority job currently waiting. Waiting jobs live in one
// buffered channel per priority tier (levels[p] holds jobs of
// Priority p); the worker scans tiers high-to-low on every pick, so
// within a tier plain channel FIFO handles the tie-break, and signal
// lets it block instead of busy-polling when every tier is empty.
type Scheduler struct {
	levels [numLevels]chan Job
	signal chan struct{}
	done   chan int
}

func NewScheduler() *Scheduler {
	s := &Scheduler{
		signal: make(chan struct{}, 1),
		done:   make(chan int, 100),
	}
	for p := range s.levels {
		s.levels[p] = make(chan Job, levelBuf)
	}
	go s.worker()
	return s
}

// tryPick does one non-blocking pass over the tiers from highest
// priority to lowest, returning the first job it finds (if any).
func (s *Scheduler) tryPick() (Job, bool) {
	for p := numLevels - 1; p >= 0; p-- {
		select {
		case job := <-s.levels[p]:
			return job, true
		default:
		}
	}
	return Job{}, false
}

func (s *Scheduler) worker() {
	for {
		job, ok := s.tryPick()
		if !ok {
			<-s.signal // block until Submit wakes us; no busy-poll
			continue
		}

		time.Sleep(20 * time.Millisecond) // simulated work
		s.done <- job.ID
	}
}

func (s *Scheduler) Submit(job Job) {
	p := job.Priority
	if p < 0 {
		p = 0
	}
	if p >= numLevels {
		p = numLevels - 1
	}
	s.levels[p] <- job

	select {
	case s.signal <- struct{}{}:
	default: // a wake-up is already pending; no need to queue another
	}
}

func (s *Scheduler) Completed() <-chan int {
	return s.done
}
```

Design notes and honest tradeoffs versus Approach 1:

- **This is only "highest priority *observed during one scan*," not "highest priority currently queued" — and that's a real, weaker guarantee than Approach 1's.** `tryPick` does a non-blocking pass through 31 channels one at a time; there is no lock held across the whole scan. If a priority-10 job is submitted at the exact moment `tryPick` has already scanned past tier 10 (say it's currently checking tier 3), that job simply waits for the *next* `tryPick` call rather than preempting the one in progress. Approach 1 holds `mu` across the entire pop, so its pick is genuinely atomic with respect to concurrent submissions. This exercise's own tests don't expose the gap — every test enqueues the whole backlog a full `jobLatency` (20ms) before the pick that matters, which is much larger than one scan takes — but it's a real difference a reader should know about before reaching for this design under tighter timing.
- **Fixed tiers means fixed priority *range*, silently, not just "doesn't scale."** `Submit` clamps any `Priority` outside `[0, numLevels-1]` into the nearest boundary tier. That's not a graceful degradation — a job submitted with `Priority: 32` and one with `Priority: 1000` both land in the same top tier and get FIFO'd against each other as if they had equal priority, silently losing whatever distinction the caller intended. This design is only appropriate when the full set of priority values in use is known ahead of time and small (e.g. "low/normal/high/urgent"), not for an open-ended or continuous priority range — that's exactly what Approach 1's heap handles correctly instead.
- **Cost shape is the mirror image of Approach 1's**: `numLevels` buffered channels of `levelBuf` capacity each are allocated up front — a fixed ~130KB per `Scheduler` here — regardless of how many jobs are ever actually queued, versus the heap's slice that only grows with actual queued jobs. And `Submit` on this design *blocks* if a single tier's buffer fills up, where the heap-based `Submit` never blocks (an `append` under a mutex, not a channel send). So this alternative trades away the heap's O(log n) push/pop and unbounded/never-blocking `Submit` for simpler code, at the cost of a fixed memory footprint and a `Submit` that can stall under sustained same-tier load.
- **The `signal` channel is a single-slot "maybe something's ready" flag, not a per-job wake-up.** A buffered channel of capacity 1 with a non-blocking `select`/`default` send from `Submit` means multiple submissions between two worker wake-ups collapse into a single pending signal — that's fine, because the worker always re-scans *all* tiers on `tryPick`, not just the tier that triggered the signal, so nothing is missed; it just avoids an unbounded backlog of no-op wake-ups.

**Verified**: same scratch-directory protocol, in a separate throwaway copy from Approach 1. `gofmt -l` is clean, `go vet ./...` is clean, and `go test -race -count=10 ./...` passes repeatably, including `TestSchedulerRace` and `TestSchedulerPrioritizesUrgentJobs`.

## Key takeaways

- A plain FIFO channel can express "process everything," but never "reorder what's already queued by some property of the item" — once a value is sent, its position is frozen. Priority scheduling needs a data structure that can be reordered after items are already waiting, which is exactly what a mutex-guarded heap (or any structure you can re-scan/re-sort under a lock) gives you and a channel doesn't.
- `container/heap` needs a type implementing `sort.Interface` plus `Push`/`Pop` operating on the underlying slice — encode the tie-break (submission order) directly in `Less` alongside the primary sort key (priority) rather than as a separate mechanism.
- `sync.Cond` is the standard tool for "block until some shared, mutex-guarded condition becomes true, then re-check the condition in a loop (never an `if`) once woken" — it composes naturally with a mutex-guarded heap because the same lock protects both the data structure and the condition being waited on.
- A design that looks "lighter-weight" (plain channels, no `container/heap`) can trade away a correctness property that isn't obvious until you look for it: fixed priority-tiered channels only give you "highest priority observed in one non-atomic scan," not "highest priority currently queued" — a genuine, if narrow, gap versus a properly locked heap. Don't reach for the simpler design without naming that gap explicitly.
- `testing/synctest` requires every goroutine reachable from the test's bubble to eventually block or exit so the fake clock can detect idleness; a component with a worker goroutine that runs forever and has no shutdown API (as here) is fundamentally incompatible with it, and the test must fall back to real-clock waits with generous timeouts instead.
