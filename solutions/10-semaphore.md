# Semaphore: Bounding Parallelism Against a Rate-Limited API — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `10-semaphore/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`FetchAll` is supposed to fetch every request in `reqs` from `api`
(`FlakyAPI.Call`, see `mockapi.go`) fast, but without ever pushing more
than a small, fixed number of calls to `api.Call` in flight at once.
`FlakyAPI` doesn't queue excess calls — if more than `maxConcurrent`
calls are in flight at the same instant, it immediately fails the
excess ones with `ErrTooManyConcurrentRequests` instead of waiting for
a slot to free up.

The naive version fires off one goroutine per request with no limit
at all:

```go
func FetchAll(api *FlakyAPI, reqs []string) []string {
	results := make([]string, len(reqs))
	var wg sync.WaitGroup

	for i, r := range reqs {
		wg.Add(1)
		go func(i int, r string) {
			defer wg.Done()

			res, err := api.Call(r)
			if err != nil {
				results[i] = "ERROR: " + err.Error()
				return
			}
			results[i] = res
		}(i, r)
	}

	wg.Wait()

	return results
}
```

The task is to build your own counting semaphore from a buffered
`chan struct{}` — not `golang.org/x/sync/semaphore` or any other
pre-built one — and use it to cap how many `api.Call` invocations are
in flight simultaneously, while keeping `FetchAll` genuinely
concurrent (not serialized down to one call at a time). The signature
— `func FetchAll(api *FlakyAPI, reqs []string) []string` — must stay
exactly the same.

## Why the naive version is wrong

Every request gets its own goroutine, and every goroutine calls
`api.Call` the instant it's scheduled — there is nothing in the code
that limits how many of those calls can be in flight at the same
time. Run it against an API configured to reject anything past 3
concurrent calls (`NewFlakyAPI(3)`) with 12 requests in flight, and it
regularly blows straight through that budget:

```
--- FAIL: TestFetchAllRespectsSemaphoreLimit (0.10s)
    check_test.go:77: API high-water mark = 4, want <= 2; FetchAll is not
        bounding its own concurrency strictly below the API's limit of 3
    check_test.go:84: result for "request-0" = "ERROR: too many concurrent
        requests"; a properly bounded FetchAll should never trip the API's
        own rejection logic
    check_test.go:84: result for "request-1" = "ERROR: too many concurrent
        requests"; ...
FAIL
```

(9 of the 12 requests failed this way in that run.) The high-water
mark landed at 4 with 9 of the 12 requests coming back as errors — the
test comment's own worked example predicts
"around 12" in the worst case, since all 12 goroutines are runnable at
once, but the actual number you observe is whatever the Go scheduler
happens to let through before the first `api.Call` returns; it varies
from run to run and machine to machine. What's consistent is that it's
never bounded at all, and it's always high enough to trip the API's
rejection logic — the only fix is to put an explicit cap in `FetchAll`
itself, not to hope the scheduler is gentle.

## Approach 1: A hand-rolled counting semaphore (buffered channel), one goroutine per request

```go
const maxInFlight = 2

func FetchAll(api *FlakyAPI, reqs []string) []string {
	results := make([]string, len(reqs))
	var wg sync.WaitGroup

	sem := make(chan struct{}, maxInFlight)

	for i, r := range reqs {
		wg.Add(1)
		go func(i int, r string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := api.Call(r)
			if err != nil {
				results[i] = "ERROR: " + err.Error()
				return
			}
			results[i] = res
		}(i, r)
	}

	wg.Wait()

	return results
}
```

This keeps the naive version's shape — one goroutine per request,
`sync.WaitGroup` to know when everything's done — and adds exactly one
thing: a semaphore. `sem := make(chan struct{}, maxInFlight)` is a
buffered channel with room for `maxInFlight` values. `sem <- struct{}{}`
before the call *acquires* a slot: it succeeds immediately if the
channel isn't full, and blocks if it is. `<-sem` after the call
*releases* the slot. Since the channel can never hold more than
`maxInFlight` values at once, at most `maxInFlight` goroutines can be
past the acquire and holding a slot at any given instant — everything
past that queues up on the (blocking) send until someone releases.
Wrapping the release in `defer` means the slot is freed even if
`api.Call` were to panic.

Note this is entirely separate from `wg`: the `WaitGroup` still tracks
"has every goroutine finished," while `sem` tracks "how many goroutines
are currently between acquire and release." All `len(reqs)` goroutines
still get spawned up front — the semaphore doesn't reduce how many
goroutines exist, only how many of them can be inside the critical
section (the `api.Call`) at once.

**Why `maxInFlight = 2` specifically, and why it's not a free choice.**
`mockapi.go` rejects a call when `current > maxConcurrent`, so an API
built with `NewFlakyAPI(3)` genuinely tolerates 3 truly-simultaneous
calls — but `check_test.go`'s `TestFetchAllRespectsSemaphoreLimit`
asserts `wantMaxHighWaterMark = 2`, a strictly tighter bound than the
API's own limit. That pins the ceiling at 2. Separately,
`TestFetchAllStillConcurrent` runs 10 requests at `CallLatency` (100ms)
each through `synctest`'s fake clock and requires the whole batch to
finish in under 600ms; with a pool of effectively `maxInFlight`
concurrent slots, the batch takes roughly `ceil(10 / maxInFlight) *
100ms`, which only clears the 600ms budget once `maxInFlight >= 2` (at
`maxInFlight = 1` it's a fully sequential 1000ms run, well over
budget). Both constraints — "strictly below the API's real limit of 3"
from one test, "fast enough to prove real concurrency" from the other
— are only jointly satisfiable at `maxInFlight = 2`. It isn't a
stylistic pick; it's the one value both tests agree on.

## Approach 2: Fixed-size worker pool reading from a jobs channel (alternative)

Worth flagging up front: this approach deliberately does **not** build
a semaphore at all, which departs from the exercise's literal
instruction ("implement your OWN counting semaphore from scratch,
using a buffered channel of `struct{}`"). It's included to show that
bounding concurrency by *construction* (only ever having `maxInFlight`
goroutines that could call `api.Call`) is a real alternative to
bounding it with a *counter* that per-request goroutines acquire and
release — a useful contrast to know, and it happens to pass every test
in this exercise with the same `maxInFlight = 2`. If you're working
the exercise as written, Approach 1 is the one to submit.

```go
const maxInFlight = 2

type fetchJob struct {
	index int
	req   string
}

func FetchAll(api *FlakyAPI, reqs []string) []string {
	results := make([]string, len(reqs))

	jobs := make(chan fetchJob)

	var wg sync.WaitGroup
	wg.Add(maxInFlight)
	for w := 0; w < maxInFlight; w++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				res, err := api.Call(j.req)
				if err != nil {
					results[j.index] = "ERROR: " + err.Error()
					continue
				}
				results[j.index] = res
			}
		}()
	}

	go func() {
		for i, r := range reqs {
			jobs <- fetchJob{index: i, req: r}
		}
		close(jobs)
	}()

	wg.Wait()

	return results
}
```

This is a genuinely different design, not a cosmetic variant of
Approach 1: instead of spawning `len(reqs)` goroutines that each
compete for one of `maxInFlight` semaphore slots, it spawns exactly
`maxInFlight` long-lived worker goroutines up front and feeds them
requests over a shared, unbuffered `jobs` channel. Concurrency is
bounded by construction — there are only ever `maxInFlight` goroutines
that could possibly be inside `api.Call` — rather than by a counter
that every goroutine has to remember to acquire and release correctly.
A separate feeder goroutine sends every request into `jobs` and closes
it once done, which is what lets each worker's `for j := range jobs`
loop terminate cleanly; `wg.Wait()` (over the fixed `maxInFlight`
workers now, not over `len(reqs)` per-request goroutines) still tells
the caller when every result has been written.

Both approaches satisfy every test with the same `maxInFlight = 2`,
and both keep goroutine writes to `results[i]` race-free the same way:
each index is written by exactly one goroutine, so there's no shared
mutable state between workers even without a mutex. The choice between
them is really "one goroutine per unit of work, gated by a semaphore"
vs. "a fixed pool of workers pulling from a queue" — the same
trade-off you'll see again, at larger scale, in the worker-pool
exercise.

## Key takeaways

- A buffered `chan struct{}` is a complete counting semaphore: send to
  acquire (blocks once full), receive to release. No third-party
  package needed.
- Spawning every goroutine unconditionally and then bounding how many
  of them can be *inside the critical section* at once (Approach 1) and
  bounding how many goroutines *exist* in the first place (Approach 2)
  both cap concurrency correctly — they differ in whether the limit is
  enforced by a shared counter or by construction.
- `defer func() { <-sem }()` right after acquiring is the same pattern
  as `defer mu.Unlock()` right after locking: put the release
  immediately next to the acquire so it's never possible to forget it
  on an early return or panic.
- Picking the semaphore size isn't arbitrary busywork: here it's the
  unique integer that simultaneously satisfies "strictly below the
  API's real rejection threshold" and "fast enough to prove the work is
  still concurrent, not serialized." Both tests need to be read
  together to find it.
- The trivial "fix" of processing requests one at a time defeats the
  entire point of a semaphore (bounding concurrency, not eliminating
  it) — which is exactly why `TestFetchAllStillConcurrent` exists, and
  why "make it pass" isn't the same goal as "make it correct and still
  fast."
