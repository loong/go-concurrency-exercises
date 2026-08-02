# Heartbeats: Detecting a Stalled Worker Before It's Too Late

Given is `DoWork`, which is supposed to simulate a long-running worker
that processes `workUnits` units of work one at a time (see
`mockworker.go` for the per-unit timing helper), sending one result
per completed unit on `results` and closing `results` once the whole
job is done. While it is actively working on a unit, it is also
supposed to pulse on `heartbeat` roughly every `pulseInterval`, for as
long as that unit takes - that pulse is the only way a caller
monitoring the worker can tell "still working, just slow" apart from
"wedged forever", since a unit that simply takes a long time and a
unit that has stalled completely look identical if all you can do is
wait for its result.

Right now `DoWork`'s heartbeat is basically decorative: it fires a
single pulse the instant the goroutine starts, then never again - no
matter how long an individual unit takes, or whether one has stalled
outright.

`WorkWithTimeout` is supposed to run `DoWork` and use the heartbeat to
detect a stalled worker quickly: reset a timer every time a heartbeat
OR a result arrives, and fail the moment `perPulseTimeout` elapses with
neither. Right now it does nothing like that - it starts a single flat
timer sized off the whole job at the very beginning and never resets
it, and it never even looks at `heartbeat`. That means it can't tell
"worker pulsing normally on a slow-but-fine job" apart from "worker
wedged": a healthy job that happens to run a bit long gets killed just
as dead as a genuinely stalled one, and a real stall isn't caught
within one pulse interval of it starting - only whenever that same
flat deadline happens to expire, however long that turns out to be.

Your task is to fix both:

- `DoWork` must pulse on `heartbeat` throughout each unit's work, not
  just once at startup, while still respecting `done` (stop promptly
  instead of blocking forever trying to send on a channel nobody is
  receiving from).
- `WorkWithTimeout` must select on `heartbeat`, `results`, AND a timer
  that gets reset to `perPulseTimeout` on every heartbeat or result
  received - returning a non-nil error the moment `perPulseTimeout`
  elapses with neither, instead of waiting for however long the
  stalled unit would otherwise have taken.

Keep the signatures identical:

```go
func DoWork(done <-chan struct{}, pulseInterval time.Duration, workUnits int) (heartbeat <-chan struct{}, results <-chan int)
func WorkWithTimeout(workUnits int, stallAfter int, perPulseTimeout time.Duration) ([]int, error)
```

`stallAfter < 0` or `stallAfter >= workUnits` means "no stall, run
normally" - use that as your normal-path case. A `stallAfter` in range
configures the work unit at that (0-based) index to simulate a worker
wedged on something unresponsive: see `mockworker.go`'s `SimulateUnit`
and `SetStallUnit` for exactly how that's modeled - once a unit stalls,
it never checks in again (no more heartbeats, no more results) until
it finally gives up, which - unless you catch it via the heartbeat -
takes a lot longer than any caller should have to wait.

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
