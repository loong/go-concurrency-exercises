# Protect a Flaky Payment Gateway with a Circuit Breaker

Given is a `CircuitBreaker` that wraps calls to a flaky downstream
`PaymentGateway` (see `mockgateway.go`). The gateway occasionally goes
down for a while, and right now the `CircuitBreaker` does nothing to
protect it: every call to `Execute` is simply passed straight through
to the gateway, no matter how many times in a row it has just failed.

This is a problem. When the gateway is down, every single caller still
pays the full cost of reaching out to it (and getting an error back)
instead of failing fast - and a struggling downstream service gets
hammered with even more load exactly when it can least handle it,
instead of being given room to recover.

Your task is to implement the actual circuit breaker pattern on top of
`CircuitBreaker`, safe for concurrent use from multiple goroutines:

- **Closed** (initial state): calls pass through to `gateway.Charge`.
  Each failure increments a consecutive-failure counter; a success
  resets it back to 0. After **5 consecutive failures**, the breaker
  trips to **Open**.
- **Open**: calls fail immediately with `ErrCircuitOpen`, without
  calling the gateway at all, for a cooldown period of **2 seconds**.
  After the cooldown elapses, the breaker moves to **Half-Open**.
- **Half-Open**: exactly one trial call is allowed through to the
  gateway. If it succeeds, the breaker resets to **Closed** (failure
  counter back to 0). If it fails, the breaker goes back to **Open**
  and the cooldown restarts.

Keep the `CircuitBreaker` type and the `Execute(amountCents int) error`
signature identical - add whatever unexported state you need to track
the current state, the failure counter and the cooldown deadline.
Export a sentinel `var ErrCircuitOpen = errors.New(...)` for callers to
check against.

## Test your solution

To complete this exercise, you must pass the tests:
```
go test
go test --race
```
