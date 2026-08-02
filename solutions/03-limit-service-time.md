# Limit Service Time for Free-Tier Users — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `03-limit-service-time/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is deliberately empty of any limiting logic:

```go
type User struct {
	ID        int
	IsPremium bool
	TimeUsed  int64 // in seconds
}

func HandleRequest(process func(), u *User) bool {
	process()
	return true
}
```

`HandleRequest` must:

- Let **premium** users run `process` for as long as they like — never killed.
- Let a **free** user run for up to **10 seconds total, accumulated across all of their requests** (not just 10s per call). Once a request would push a free user's *lifetime* usage over 10s, it must be killed as soon as the 10s mark is hit — not allowed to run to completion.
- Track only the time a free user's process *actually* used before being killed, not a full timeout's worth, and not the process's own (unknowable in advance) full duration.
- Be safe to call **concurrently** for the same user without double-spending the quota — two 6s requests racing against a 10s quota must not both succeed.
- Judge a concurrent request against the *actual* usage of an in-flight request, not a pessimistic "it might run forever" guess — a request must not be rejected just because a sibling is still running if that sibling turns out to finish quickly.

Go gives you no way to forcibly kill a running goroutine. The only thing you can do is **race a timeout against completion** with a goroutine + `select`, and stop waiting once the timeout wins — the process goroutine itself is left to finish in the background, orphaned.

## Why the naive version is wrong

`HandleRequest` just calls `process()` synchronously and always returns `true`. It:

- Never kills anything, so a free user's 11s job runs the full 11s and reports success.
- Never distinguishes premium from free.
- Has no per-user state to accumulate usage across calls, so quota can never be "used up."
- Isn't safe for concurrent calls (nothing serializes access to a shared counter, and there's no counter anyway).

Verified: running the current `check_test.go` against this stub fails `TestHandleRequest_FreeUser_SlowProcessKilled`, `TestHandleRequest_FreeUser_AccumulatedTimeExceeded`, `TestHandleRequest_FreeUser_QuotaIsPerUser`, and `TestHandleRequest_FreeUser_ConcurrentRequestsDoNotDoubleSpendQuota`.

## Approach 1: `context.WithTimeout` racing `process` in a goroutine, one mutex per user

```go
package main

import (
	"context"
	"sync"
	"time"
)

// User defines the UserModel. Use this to check whether a User is a
// Premium user or not
type User struct {
	ID        int
	IsPremium bool
	TimeUsed  time.Duration // accumulated processing time actually used, across all requests

	mu sync.Mutex // serializes requests for this user and guards TimeUsed
}

// freeQuota is the total processing time (across all requests) a free
// user gets before being killed.
const freeQuota = 10 * time.Second

// HandleRequest runs the processes requested by users. Returns false
// if process had to be killed
func HandleRequest(process func(), u *User) bool {
	if u.IsPremium {
		// Premium users are never limited: run synchronously, no timeout.
		process()
		return true
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	remaining := freeQuota - u.TimeUsed
	if remaining <= 0 {
		// Quota already exhausted by earlier requests: kill immediately,
		// no need to even start the process.
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), remaining)
	defer cancel()

	done := make(chan struct{})
	start := time.Now()
	go func() {
		process()
		close(done)
	}()

	select {
	case <-done:
		// Completed within quota: record the time it actually used.
		u.TimeUsed += time.Since(start)
		return true
	case <-ctx.Done():
		// Timed out: it consumed exactly the remaining quota. The
		// goroutine above is left running in the background (Go has no
		// way to forcibly kill it), but we stop waiting on it.
		u.TimeUsed += remaining
		return false
	}
}

func main() {
	RunMockServer()
}
```

Design notes:

- **Premium bypasses everything.** No goroutine, no timeout, no lock — `process()` runs inline and always returns `true`, exactly matching "premium users are never limited."
- **`TimeUsed` is a `time.Duration`**, not seconds-as-`int64`, so partial-second usage (e.g. "killed ~2s in") accumulates precisely instead of being truncated to whole seconds.
- **One `sync.Mutex` embedded per `User`** (zero-value-usable, so `&User{ID: ..., IsPremium: ...}` composite literals from the tests still work untouched) serializes *all* requests for that user — including the racing goroutine and the quota update. This is what makes the design correct under concurrency:
  - Two concurrent 6s requests against a fresh 10s quota can't both slip through a check-then-spend race, because the second request can't even start its own quota check until the first has finished its whole `HandleRequest` call (goroutine race + `TimeUsed` update) and released the lock. By the time the second checks `remaining`, the first's usage is already recorded.
  - This also makes the "don't judge on a worst-case guess" test pass for free: since the second request simply *waits* for the first's real, already-recorded usage rather than reserving worst-case quota up front, it never has to guess.
- **`remaining := freeQuota - u.TimeUsed`** computed while holding the lock is the amount of quota left *right now*. If it's `<= 0`, we don't even bother starting `process` — immediate kill, matching the "killed immediately, no waiting" expectation.
- **On timeout, `u.TimeUsed += remaining`** (not some open-ended "however long it ran") because by construction the select fires at exactly `remaining` elapsed — that's the actual time used before it was cut off, no more.
- The orphaned goroutine from a killed request keeps running to real completion in the background (unavoidable in Go), but it holds no reference to `u.mu`, so it can't block subsequent requests once `HandleRequest` has returned.

**Verified**: copied this exercise into a throwaway scratch directory, confirmed the naive stub fails the current `check_test.go`, then dropped in this solution and ran `go vet ./...` (clean) and `go test -race -count=3 ./...` — all tests pass repeatably, including the synctest-based tests and the two real-clock concurrency tests.

## Approach 2: `time.Timer` + `select`, no `context` package

Functionally identical arbitration, just built from a raw timer instead of a context — useful if you'd rather not pull in `context` for something this local.

```go
package main

import (
	"sync"
	"time"
)

type User struct {
	ID        int
	IsPremium bool
	TimeUsed  time.Duration

	mu sync.Mutex
}

const freeQuota = 10 * time.Second

func HandleRequest(process func(), u *User) bool {
	if u.IsPremium {
		process()
		return true
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	remaining := freeQuota - u.TimeUsed
	if remaining <= 0 {
		return false
	}

	done := make(chan struct{})
	start := time.Now()
	go func() {
		process()
		close(done)
	}()

	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-done:
		u.TimeUsed += time.Since(start)
		return true
	case <-timer.C:
		u.TimeUsed += remaining
		return false
	}
}

func main() {
	RunMockServer()
}
```

The only real difference from Approach 1 is the timeout mechanism: `time.NewTimer(remaining)` + `<-timer.C` instead of `context.WithTimeout` + `<-ctx.Done()`. `defer timer.Stop()` releases the timer promptly when `done` wins the race, same role `cancel()` plays for the context version. Everything about the per-user mutex, the accumulation logic, and the premium bypass is unchanged.

**Verified**: same scratch-directory protocol, `go vet ./...` clean, `go test -race -count=2 ./...` passing.

## Key takeaways

- Go cannot forcibly kill a goroutine. "Killing" a process really means racing a timeout against a `done` channel with `select` and simply walking away from the goroutine the instant the timeout wins — the orphaned goroutine keeps running to real completion, invisibly, in the background.
- Store `TimeUsed` as a `time.Duration`, not integer seconds, so partial-second accounting (a request killed 2.1s into a 4s call) stays precise instead of getting rounded away.
- For "accumulated across requests" quotas, compute `remaining := quota - TimeUsed` fresh on every call, and use *that* as the timeout — not the request's own requested duration.
- On a timeout, credit the user with exactly `remaining` (the elapsed time when the timer fired), not some looser estimate — that's the only amount of processing that provably happened before the cutoff.
- Making a whole `HandleRequest` call for a given user hold that user's mutex for its *entire* duration (check quota → race process → record usage) is what simultaneously prevents double-spending on concurrent requests *and* avoids the need for worst-case quota reservations: a second concurrent call simply waits for the first's real, already-known usage instead of guessing.
- Keep the mutex scoped **per user** (embedded in `User` as an unexported zero-value field) rather than global, so unrelated users' requests never contend with each other.
- Premium users should bypass the goroutine/timeout machinery entirely — call `process()` inline. Besides being simpler, it avoids ever needing a "timeout" value for a user who by definition has none.
