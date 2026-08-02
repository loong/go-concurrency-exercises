# Clean Inactive Sessions to Prevent Memory Overflow — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `05-session-cleaner/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`SessionManager` stores sessions in a plain `map[string]Session` with no
synchronization and no expiry. Two things need fixing:

1. **Memory growth.** Sessions are added but never removed, so a
   long-running process eventually runs out of memory. You need a
   cleaner that removes any session that hasn't been touched (created
   or updated) for more than 5 seconds — and the exercise explicitly
   asks for it to happen "anytime between 5 and 7 seconds" after the
   last update, i.e. it doesn't have to be instant, but it can't be
   indefinitely delayed either.
2. **Races.** `CreateSession`, `GetSessionData`, and
   `UpdateSessionData` all read/write the shared `sessions` map with
   no locking at all. Under concurrent access (see
   `TestSessionManagerConcurrentAccess` in `check_test.go`, which
   hammers all three methods from many goroutines under `-race`)
   that's an immediate data race — and once you add a background
   cleaner goroutine into the mix, it becomes a *third* uncoordinated
   writer to the same map.

## Why the naive version is wrong

Running the untouched starting `main.go` against the current
`check_test.go`:

- `TestSessionManagersCleaner` and `TestSessionManagersCleanerAfterUpdate`
  fail outright — nothing ever removes an old session, so
  `GetSessionData` keeps returning it forever instead of
  `ErrSessionNotFound`.
- `TestSessionManagerConcurrentAccess` fails under `go test -race`
  with a concurrent map read/write data race — `CreateSession` and
  `UpdateSessionData` both do unsynchronized `m.sessions[id] = ...`
  from different goroutines.

## A constraint you can't design around: `testing/synctest`

The current `check_test.go` uses Go's `testing/synctest` package to
run the timing-sensitive tests (`TestSessionManagersCreationAndUpdate`,
`TestSessionManagersCleaner`, `TestSessionManagersCleanerAfterUpdate`)
on a fake clock inside a "bubble", so `time.Sleep(7 * time.Second)`
finishes instantly instead of taking 7 real seconds.

`synctest.Test` has a hard rule: when the function passed to it
returns, **every goroutine it spawned (transitively) must also have
exited.** If any goroutine spawned inside the bubble is still alive —
even if it's just quietly blocked waiting on a channel or a ticker —
`synctest.Test` panics:

```
panic: deadlock: main bubble goroutine has exited but blocked goroutines remain
```

This matters a lot for this exercise, because the "obvious" design —
start a `time.Ticker`-driven goroutine in `NewSessionManager` that
loops forever, sweeping expired sessions every couple of seconds — is
exactly the shape that trips this rule. None of the three
`synctest`-based tests ever calls a `Stop()`/`Close()` on the
manager, so a forever-running sweeper goroutine is still alive (durably
blocked on the ticker's channel) when the test function returns, and
the whole test panics rather than just failing an assertion. This was
verified directly: a ticker-based sweeper (Approach 2 below) makes
`TestSessionManagersCreationAndUpdate` panic with exactly that message,
even though the sweeper's own logic is otherwise correct.

So for *this* test suite, the cleaner can't be an unconditional,
never-exiting background goroutine — it has to be something that
either doesn't spawn a persistent goroutine at all, or bounds each
goroutine's lifetime to a single session's timeout. Approach 1 below
does the latter and is the recommended solution.

## Approach 1: per-session `time.AfterFunc` timer (recommended)

Instead of one long-lived sweeper goroutine, give each session its
own timer that fires once, exactly 5 seconds after the session was
last touched. `UpdateSessionData` resets (not replaces) that same
timer, so there is at most one pending callback per session — and,
crucially, each callback's underlying goroutine is only alive for the
`AfterFunc`'s own duration, so nothing outlives a `synctest` bubble
that never advances the clock again.

```go
package main

import (
	"errors"
	"log"
	"sync"
	"time"
)

// sessionTTL is how long a session may go without being updated
// before it is reclaimed.
const sessionTTL = 5 * time.Second

// SessionManager keeps track of all sessions from creation, updating
// to destroying.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]Session
}

// Session stores the session's data
type Session struct {
	Data        map[string]interface{}
	lastUpdated time.Time
	timer       *time.Timer
}

// NewSessionManager creates a new sessionManager
func NewSessionManager() *SessionManager {
	m := &SessionManager{
		sessions: make(map[string]Session),
	}
	return m
}

// expire is scheduled via time.AfterFunc to fire sessionTTL after the
// most recent (Create|Update)SessionData call for sessionID. Because
// UpdateSessionData resets the same timer instead of creating a new
// one, at most one of these callbacks is ever pending per session.
func (m *SessionManager) expire(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return
	}

	// Guard against a callback that lost the race with a concurrent
	// update: if the session was touched again after this timer was
	// scheduled, leave it alone (Reset in UpdateSessionData already
	// pushed the deadline out further).
	if time.Since(session.lastUpdated) < sessionTTL {
		return
	}

	delete(m.sessions, sessionID)
}

// CreateSession creates a new session and returns the sessionID
func (m *SessionManager) CreateSession() (string, error) {
	sessionID, err := MakeSessionID()
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[sessionID] = Session{
		Data:        make(map[string]interface{}),
		lastUpdated: time.Now(),
		timer:       time.AfterFunc(sessionTTL, func() { m.expire(sessionID) }),
	}

	return sessionID, nil
}

// ErrSessionNotFound returned when sessionID not listed in
// SessionManager
var ErrSessionNotFound = errors.New("SessionID does not exists")

// GetSessionData returns data related to session if sessionID is
// found, errors otherwise
func (m *SessionManager) GetSessionData(sessionID string) (map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return session.Data, nil
}

// UpdateSessionData overwrites the old session data with the new one
func (m *SessionManager) UpdateSessionData(sessionID string, data map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	// Renew expiry: push the *same* timer out instead of leaking a
	// fresh one, so exactly one AfterFunc callback is ever in flight
	// per session.
	session.timer.Reset(sessionTTL)

	m.sessions[sessionID] = Session{
		Data:        data,
		lastUpdated: time.Now(),
		timer:       session.timer,
	}

	return nil
}

func main() {
	m := NewSessionManager()
	sID, err := m.CreateSession()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Created new session with ID", sID)

	data := make(map[string]interface{})
	data["website"] = "longhoang.de"

	if err := m.UpdateSessionData(sID, data); err != nil {
		log.Fatal(err)
	}
	log.Println("Update session data, set website to longhoang.de")

	updatedData, err := m.GetSessionData(sID)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Get session data:", updatedData)
}
```

**Why this satisfies the exercise:**

- **Actually reclaims memory in the background**, independent of
  whether anyone ever calls `GetSessionData` again — unlike a design
  that only checks expiry lazily on read (which would never free a
  session nobody looks at again, defeating the whole point of the
  exercise).
- **A single mutex** (`m.mu`) guards every read and write of
  `m.sessions`, including inside the `expire` callback, so there's no
  race between a timer firing and a concurrent `Create`/`Update`/`Get`.
- **Preserving the same `*time.Timer` pointer** across
  `UpdateSessionData` calls (via `session.timer.Reset(...)`) matters:
  if you instead replaced it with a brand new timer on every update,
  the *old* timer would still fire on schedule, and its callback would
  correctly decline to delete (thanks to the `time.Since(...) <
  sessionTTL` guard) — but nothing would ever reschedule a new check
  for the session's *new* deadline, silently reintroducing the memory
  leak the exercise is about, while every existing test still passes.
  This is the kind of bug that's easy to write and easy to miss.
- **The 5–7 second window** is satisfied trivially: the timer fires
  at exactly `lastUpdated + 5s`, which is always inside `[5s, 7s]`.
- **No goroutine outlives the `synctest` bubble** in a way the
  bubble's leak check can see: each `AfterFunc` timer's callback
  goroutine only exists momentarily when it fires (or not at all, if
  it never fires within the test), so there's no persistent
  goroutine blocking on a channel when the test function returns.

**Verified:** copied into a scratch directory (never touching the
live `05-session-cleaner/`), run against the repo's current
`check_test.go` — `go vet ./...` clean, `go test -race ./...` and
`go test -race -count=3 ./...` both pass all four tests:
`TestSessionManagersCreationAndUpdate`, `TestSessionManagersCleaner`,
`TestSessionManagersCleanerAfterUpdate`, and the heavy concurrent
`-race` stress test (`TestSessionManagerConcurrentAccess`, 20
sessions × 3 goroutines × 2000 iterations each).

## Approach 2: `time.Ticker`-driven background sweeper (classic design — fails *this* test suite)

This is the design most people reach for first, and it's the
textbook-correct way to build a session cleaner in a real service: one
goroutine, started once in `NewSessionManager`, wakes up on a ticker
every couple of seconds, takes the lock, and deletes anything past its
TTL:

```go
const sessionTTL = 5 * time.Second
const cleanupInterval = 2 * time.Second

type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]Session
	stop     chan struct{}
}

func NewSessionManager() *SessionManager {
	m := &SessionManager{
		sessions: make(map[string]Session),
		stop:     make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			m.mu.Lock()
			for id, s := range m.sessions {
				if now.Sub(s.lastUpdated) > sessionTTL {
					delete(m.sessions, id)
				}
			}
			m.mu.Unlock()
		case <-m.stop:
			return
		}
	}
}

// Stop terminates the background sweeper so the manager (and its
// goroutine) can be cleanly torn down, e.g. in a real server's
// shutdown path, or in a test that calls it explicitly.
func (m *SessionManager) Stop() {
	close(m.stop)
}
```

This one goroutine per manager, plus a bounded 2-second sweep
interval, is arguably *simpler* to reason about than per-session
timers, and it's the shape you'd want in production if you also wire
up `Stop()` to your service's graceful-shutdown path.

**The real tradeoff between the two designs** is timer-count versus
sweep-latency, not correctness: Approach 1 keeps one live
`*time.Timer` per session, so N concurrent sessions means N entries in
the runtime's timer heap, and a burst of sessions expiring together
produces a burst of callback goroutines all contending for the same
mutex (the concurrency stress test alone creates roughly 40,000 such
timers in well under a second — they simply never fire because the
test finishes and the process exits first). Approach 2 holds exactly
one goroutine and one ticker no matter how many sessions exist, at the
cost of up to `cleanupInterval` (here, 2s) of extra retention latency
beyond the TTL before a stale session is actually removed. For a
handful of sessions the per-session-timer cost is negligible and you
get tighter expiry; at very high session counts the flat one-goroutine
sweep cost of Approach 2 (in a context where you control the test
harness and can call `Stop()`) may be preferable.

**It does not pass this repository's `check_test.go` as written,
verified directly:** none of the `synctest`-based tests ever call
`Stop()`, so `cleanupLoop`'s goroutine is still alive — durably
blocked in the `select` — when the test function returns. That trips
`synctest`'s leak check and the test panics:

```
--- FAIL: TestSessionManagersCreationAndUpdate (0.00s)
panic: deadlock: main bubble goroutine has exited but blocked goroutines remain [recovered, repanicked]
...
goroutine NN [select (durable), synctest bubble 1]:
    SessionManager.cleanupLoop(...)
```

This is included specifically because it's an instructive contrast:
the ticker design isn't *wrong* in general, but it's incompatible with
being spawned unconditionally inside a test that uses `synctest.Test`
and never tears the manager down. If you use this shape in your own
solution, either don't run it under `synctest`, or make sure every
test that constructs a `SessionManager` also calls `Stop()` before
returning.

(You may also notice a comment in `check_test.go` — `// Note that the
cleaner is only running every 5s` — inside `TestSessionManagersCleaner`.
Checking `git show HEAD:5-session-cleaner/check_test.go` confirms this
comment predates the `synctest` conversion: it's already present in
the version of the test that uses real `time.Sleep` calls instead of
a fake clock, i.e. it was written with a ticker-based sweeper in mind.
It no longer describes a hard requirement of the current, `synctest`-based
assertions — per-session `time.AfterFunc` timers, as in Approach 1,
satisfy those assertions without running afoul of `synctest`'s
goroutine-leak check.)

## Key takeaways

- A cleaner that only expires sessions **lazily, on read** (e.g.
  checking `lastUpdated` inside `GetSessionData` and deleting if
  stale) will pass simple tests that always re-fetch the session
  afterward, but it never frees memory for sessions nobody looks at
  again — which is the exact problem the exercise exists to solve.
  Don't mistake "passes the assertions" for "solves the stated
  problem."
- Any code path that touches the shared `sessions` map —
  `CreateSession`, `GetSessionData`, `UpdateSessionData`, *and* your
  cleaner's own delete logic — must go through the same
  `sync.Mutex`/`sync.RWMutex`. A cleaner that isn't synchronized with
  the rest of the manager just adds a third unsynchronized writer.
- When resetting a per-session timer on update, reuse the existing
  `*time.Timer` via `Reset` rather than discarding it for a new one —
  otherwise the orphaned old timer still fires, the "already renewed,
  ignore" guard correctly no-ops it, and nothing takes its place,
  silently breaking cleanup for that session.
- If your test suite uses `testing/synctest`, a goroutine that runs
  forever (a `time.Ticker` loop with no exit condition, started
  in a constructor and never stopped by the test) will panic the
  bubble with `deadlock: main bubble goroutine has exited but blocked
  goroutines remain` — even though the logic inside that goroutine is
  perfectly correct. Bound each goroutine's lifetime (e.g. one-shot
  `time.AfterFunc` timers) or give the caller an explicit way to stop
  it and make sure every test that spawns one uses it.
