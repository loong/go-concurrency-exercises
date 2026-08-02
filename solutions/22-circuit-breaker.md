# Protect a Flaky Payment Gateway with a Circuit Breaker — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `22-circuit-breaker/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a `CircuitBreaker` that wraps a flaky `PaymentGateway` (see `mockgateway.go`) and does no protection at all:

```go
type CircuitBreaker struct {
	gateway *PaymentGateway
}

func (cb *CircuitBreaker) Execute(amountCents int) error {
	return cb.gateway.Charge(amountCents)
}
```

`CircuitBreaker` must implement the classic three-state pattern, safe for concurrent use:

- **Closed** (initial state): calls pass through to `gateway.Charge`. Each failure increments a consecutive-failure counter; a success resets it to 0. After **5 consecutive failures**, the breaker trips to **Open**.
- **Open**: calls fail immediately with `ErrCircuitOpen`, without touching the gateway at all, for a cooldown of **2 seconds**. After the cooldown elapses, the breaker moves to **Half-Open**.
- **Half-Open**: exactly one trial call is let through. Success resets fully to **Closed** (failure counter back to 0). Failure sends it back to **Open** and restarts the cooldown.

The `CircuitBreaker` type and the `Execute(amountCents int) error` signature must stay identical, and a sentinel `var ErrCircuitOpen = errors.New(...)` must exist for callers to check against with `errors.Is`.

## Why the naive version is wrong

`Execute` is a pure pass-through: it never looks at how many times the gateway has recently failed, and never remembers that it's supposed to be protecting anything. Concretely:

- There is no failure counter, no state, and no cooldown deadline anywhere in `CircuitBreaker` — just a `gateway` field.
- Every single call reaches `gateway.Charge`, whether the gateway is healthy or has been failing for the last hour. A struggling downstream service never gets any relief; it keeps getting hammered at exactly the moment it can least handle it.
- `ErrCircuitOpen` is declared but never returned by anything.

Verified: running the current `check_test.go` against this naive `main.go` in a throwaway scratch copy fails both state-machine tests:

```
--- FAIL: TestCircuitOpensAfterThreshold (0.00s)
    check_test.go:42: expected ErrCircuitOpen on 6th call, got payment gateway: connection down
--- FAIL: TestCircuitHalfOpenRecovery (0.00s)
    check_test.go:67: expected breaker to be open, got payment gateway: connection down
```

(`TestCircuitConcurrentSafety` happens to pass even against the naive version — it only asserts a loose upper bound on gateway call count, which a plain pass-through trivially satisfies.)

## Approach 1: mutex-protected state machine

```go
package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCircuitOpen is returned by Execute when the circuit breaker is
// open (or half-open with its trial slot taken) and is fail-fasting
// without reaching the gateway.
var ErrCircuitOpen = errors.New("circuit breaker is open")

type circuitState int

const (
	stateClosed circuitState = iota
	stateOpen
	stateHalfOpen
)

const (
	failureThreshold = 5
	cooldownPeriod   = 2 * time.Second
)

// CircuitBreaker wraps a PaymentGateway and protects it from being
// hammered with load while it is failing, using the classic
// Closed -> Open -> Half-Open state machine.
type CircuitBreaker struct {
	gateway *PaymentGateway

	mu               sync.Mutex
	state            circuitState
	consecutiveFails int
	openedAt         time.Time
	halfOpenInFlight bool
}

// NewCircuitBreaker creates a new CircuitBreaker wrapping the given
// gateway.
func NewCircuitBreaker(gateway *PaymentGateway) *CircuitBreaker {
	return &CircuitBreaker{
		gateway: gateway,
		state:   stateClosed,
	}
}

// Execute attempts to charge amountCents through the wrapped gateway,
// applying circuit-breaker protection.
func (cb *CircuitBreaker) Execute(amountCents int) error {
	if !cb.allow() {
		return ErrCircuitOpen
	}

	err := cb.gateway.Charge(amountCents)
	cb.afterCall(err)
	return err
}

// allow decides, under lock, whether this call may proceed to the
// gateway. It also performs the Open -> Half-Open transition once the
// cooldown has elapsed, and reserves the single half-open trial slot.
func (cb *CircuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		return true

	case stateOpen:
		if time.Since(cb.openedAt) < cooldownPeriod {
			return false
		}
		// Cooldown elapsed: move to half-open and let this call be
		// the single trial.
		cb.state = stateHalfOpen
		cb.halfOpenInFlight = true
		return true

	case stateHalfOpen:
		if cb.halfOpenInFlight {
			// Trial slot already taken; fail fast.
			return false
		}
		cb.halfOpenInFlight = true
		return true
	}

	return false
}

// afterCall records the outcome of a call that was allowed through to
// the gateway and updates the state machine accordingly.
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateHalfOpen:
		cb.halfOpenInFlight = false
		if err == nil {
			cb.state = stateClosed
			cb.consecutiveFails = 0
		} else {
			cb.state = stateOpen
			cb.openedAt = time.Now()
		}

	default: // stateClosed (stateOpen can't reach here: allow() rejects it)
		if err == nil {
			cb.consecutiveFails = 0
		} else {
			cb.consecutiveFails++
			if cb.consecutiveFails >= failureThreshold {
				cb.state = stateOpen
				cb.openedAt = time.Now()
			}
		}
	}
}

func main() {
	gateway := NewPaymentGateway()
	cb := NewCircuitBreaker(gateway)

	gateway.SetFailing(true)

	for i := 1; i <= 6; i++ {
		err := cb.Execute(100)
		fmt.Println("call", i, "->", err)
	}

	fmt.Println("total calls that reached the gateway:", gateway.Calls())
}
```

Design notes:

- **One mutex guards the whole state machine** — `state`, `consecutiveFails`, `openedAt`, and `halfOpenInFlight` are only ever read or written while `cb.mu` is held. That makes every transition (checking the counter *and* flipping the state *and* stamping the deadline) a single indivisible step, which is exactly what a compound state machine like this needs.
- **`allow()` does double duty**: besides gatekeeping, it's also where the Open → Half-Open transition and the half-open trial-slot reservation happen, both under the same lock acquisition — there's no window where two goroutines could both observe "cooldown elapsed" and both think they got the one trial slot.
- **The gateway call itself happens outside the lock.** `Execute` calls `cb.gateway.Charge` between `allow()` and `afterCall()`, neither of which holds `cb.mu` while the (potentially slow) network call is in flight. This is important: holding the breaker's own lock across the call to the wrapped service would serialize *all* callers on the breaker itself, defeating half of the point of a circuit breaker (letting healthy-state traffic run concurrently).
- **`afterCall`'s `default` branch relies on a real invariant, not just tidiness**: a call can only reach `afterCall` having been let through by `allow()`, and `allow()` only returns `true` for `stateClosed` or `stateHalfOpen` — never `stateOpen`. So the `switch` only needs to distinguish "was this the half-open trial" from "was this an ordinary closed-state call"; there's no third case to handle.

**Verified**: copied the exercise into a throwaway scratch directory, confirmed the naive `main.go` fails `TestCircuitOpensAfterThreshold` and `TestCircuitHalfOpenRecovery` as shown above, then dropped in this solution. `go test -race -count=1 ./...` passes, and was repeated 5 times in a row with no flakes — including `TestCircuitHalfOpenRecovery`, which runs inside `synctest.Test` and sleeps past the 2-second cooldown, and `TestCircuitConcurrentSafety`, which hammers `Execute` from 50 goroutines at once under `-race`.

## Approach 2: lock-free state machine with `sync/atomic` (alternative)

A genuinely different primitive choice: represent `state` as an `int32`, the failure counter as an `int32`, the cooldown deadline as an `int64` (UnixNano), and the half-open trial guard as an `int32` flag — all independent atomics, no `sync.Mutex` anywhere.

```go
package main

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

const (
	stateClosed int32 = iota
	stateOpen
	stateHalfOpen
)

const (
	failureThreshold = 5
	cooldownPeriod   = 2 * time.Second
)

// CircuitBreaker wraps a PaymentGateway and protects it from being
// hammered with load while it is failing, using the classic
// Closed -> Open -> Half-Open state machine. Unlike the mutex
// approach, every field here is an independent atomic - there is no
// single lock that makes a "state + counter + deadline" transition
// happen as one indivisible step, so every transition has to be built
// out of careful CAS ordering instead.
type CircuitBreaker struct {
	gateway *PaymentGateway

	state            int32 // atomic: stateClosed / stateOpen / stateHalfOpen
	consecutiveFails int32 // atomic
	openedAtNano     int64 // atomic: UnixNano() timestamp of the last trip to Open
	halfOpenInFlight int32 // atomic 0/1: guards the single half-open trial slot
}

func NewCircuitBreaker(gateway *PaymentGateway) *CircuitBreaker {
	return &CircuitBreaker{gateway: gateway, state: stateClosed}
}

func (cb *CircuitBreaker) Execute(amountCents int) error {
	if !cb.allow() {
		return ErrCircuitOpen
	}
	err := cb.gateway.Charge(amountCents)
	cb.afterCall(err)
	return err
}

func (cb *CircuitBreaker) allow() bool {
	for {
		switch atomic.LoadInt32(&cb.state) {
		case stateClosed:
			return true

		case stateOpen:
			openedAt := atomic.LoadInt64(&cb.openedAtNano)
			if time.Since(time.Unix(0, openedAt)) < cooldownPeriod {
				return false
			}

			// Cooldown elapsed. Claim the trial slot BEFORE
			// publishing the Open -> Half-Open state change - if we
			// flipped cb.state first and only then set
			// halfOpenInFlight, a second goroutine could observe the
			// new stateHalfOpen with the flag still at its old value
			// and believe it, too, had claimed the trial slot,
			// handing out two "single" trials at once.
			if !atomic.CompareAndSwapInt32(&cb.halfOpenInFlight, 0, 1) {
				continue // someone else is already mid-transition; retry
			}
			if !atomic.CompareAndSwapInt32(&cb.state, stateOpen, stateHalfOpen) {
				atomic.StoreInt32(&cb.halfOpenInFlight, 0)
				continue
			}
			return true

		case stateHalfOpen:
			if atomic.CompareAndSwapInt32(&cb.halfOpenInFlight, 0, 1) {
				return true
			}
			return false // trial slot already taken
		}
	}
}

func (cb *CircuitBreaker) afterCall(err error) {
	switch atomic.LoadInt32(&cb.state) {
	case stateHalfOpen:
		if err == nil {
			atomic.StoreInt32(&cb.consecutiveFails, 0)
			atomic.StoreInt32(&cb.state, stateClosed)
			atomic.StoreInt32(&cb.halfOpenInFlight, 0)
		} else {
			// Publish the new deadline and state BEFORE releasing the
			// flag - releasing first would let a racing caller see
			// stateHalfOpen with the flag already free and grab a
			// brand new "trial" before the breaker has actually
			// finished re-opening.
			atomic.StoreInt64(&cb.openedAtNano, time.Now().UnixNano())
			atomic.StoreInt32(&cb.state, stateOpen)
			atomic.StoreInt32(&cb.halfOpenInFlight, 0)
		}

	default: // stateClosed
		if err == nil {
			atomic.StoreInt32(&cb.consecutiveFails, 0)
			return
		}
		fails := atomic.AddInt32(&cb.consecutiveFails, 1)
		if fails >= failureThreshold {
			// Same ordering concern: set the deadline before the
			// state that makes it visible, so nobody can observe
			// stateOpen next to a stale (or zero-value) openedAtNano
			// and wrongly conclude the cooldown already elapsed.
			atomic.StoreInt64(&cb.openedAtNano, time.Now().UnixNano())
			atomic.CompareAndSwapInt32(&cb.state, stateClosed, stateOpen)
		}
	}
}

func main() {
	gateway := NewPaymentGateway()
	cb := NewCircuitBreaker(gateway)

	gateway.SetFailing(true)

	for i := 1; i <= 6; i++ {
		err := cb.Execute(100)
		fmt.Println("call", i, "->", err)
	}

	fmt.Println("total calls that reached the gateway:", gateway.Calls())
}
```

Design notes and honest tradeoffs versus Approach 1:

- **This is meaningfully trickier to get right than the mutex version, and that's the real point of comparing them.** With a mutex, "check the cooldown, flip the state, stamp the deadline" is one critical section — there's no way for another goroutine to observe a half-finished transition. With independent atomics, every one of those compound transitions has to be manually decomposed into an ordering that never lets another goroutine see, e.g., the new `state` without the `openedAtNano` that's supposed to go with it, or a released trial-slot flag before the state it was guarding has actually settled. Each of the three code comments in `allow`/`afterCall` above marks a spot where getting the write order backwards silently reintroduces a race — a second concurrent "trial", or a spuriously-early Half-Open transition — that plain `go test -race` is not guaranteed to catch, since it only flags actual concurrent read/write races, not logic bugs in what order two atomics get published.
- **Retrying via a spin loop (`for { ... continue }`) replaces blocking.** `allow()` never calls anything that sleeps or parks; on the rare contended transition boundary (the instant the cooldown expires with more than one caller present) a goroutine just re-reads and retries. That window is tiny in practice, but it is genuinely a busy-wait, unlike the mutex version where a blocked goroutine yields the CPU via `sync.Mutex.Lock`.
- **No fundamental correctness gap remains once the ordering rules above are followed** — every compound transition here was deliberately built store-before-publish (deadline before state, flag-claim before state-flip, state-settle before flag-release) specifically to close the gaps a naive atomic port would have. But that's exactly the caveat: it took three separate, easy-to-miss ordering decisions to get there, versus zero for the mutex, which gets all of this for free from a single `Lock()`/`Unlock()` pair.
- **When it's worth it:** atomics avoid lock contention and OS-level blocking/wakeup entirely, which can matter under very high call rates. For a breaker like this — where `allow()`/`afterCall()` are already cheap, uncontended in the common (`stateClosed`) case, and the actual expensive work (`gateway.Charge`) happens outside any lock either way — the mutex version's simplicity is very likely the better default; reach for the atomic version only if profiling actually shows lock contention on the breaker itself.

**Verified**: same scratch-directory protocol as Approach 1 — dropped this version into the same throwaway copy in place of the mutex version, confirmed `go build ./...` is clean, and ran `go test -race -count=1 ./...` five times in a row with no flakes, covering the same three tests (including the `synctest`-driven half-open recovery test and the 50-goroutine concurrent-safety test).

## Key takeaways

- A circuit breaker's state (current phase, failure counter, cooldown deadline, and the half-open trial guard) is a single **compound** piece of state — every transition needs to change more than one of these fields together, consistently. A `sync.Mutex` guarding all of them gives you that atomicity for free, in one critical section per call.
- Keep the (potentially slow) call to the wrapped service **outside** the lock. The breaker's own lock should only ever be held for the cheap bookkeeping (`allow`/`afterCall`), never for however long `gateway.Charge` takes — otherwise the breaker itself becomes the new global bottleneck it was meant to prevent.
- `sync/atomic` can replace a mutex for this kind of state machine, but only if you're deliberate about *publish order* for every compound transition — deadline before state, flag-claim before state-flip, state-settle before flag-release. Getting any one of those backwards reopens exactly the kind of race a mutex closes automatically, and `-race` will not point you at the bug because it's a logic-ordering issue, not a raw unsynchronized-access issue.
- When in doubt for a small, infrequently-contended state machine like this one, prefer the mutex: it is easier to read, easier to review, and easier to prove correct than an equivalent hand-rolled atomic version — reach for atomics only when there's a measured reason to.
