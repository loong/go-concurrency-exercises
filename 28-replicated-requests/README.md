# Replicated Requests: Racing Redundant Calls for Lower Tail Latency

Given is a `FetchFastest` that is supposed to send the same request to
several redundant `Replica` handlers (see `mockreplica.go`)
concurrently, and return whichever one answers first - so that no
single replica's unpredictable tail latency slows the caller down. It
already does that much correctly: it fans out to every replica and
returns the first winner's value, so a test that only checks "does it
return the right value" passes against it as-is.

The problem is what happens to the replicas that lose the race. Right
now every replica goroutine is started with a `nil` done channel, so
none of them has any way to learn that `FetchFastest` already has its
answer and has moved on. Every losing replica just keeps running -
sleeping out its full artificial latency (or whatever work it's doing)
- long after the caller stopped caring, wasting work and leaking a
goroutine per call until it eventually finishes on its own.

Your task is to fix `FetchFastest` so that as soon as a winner is
picked (or the caller-supplied `done` is closed), every other
in-flight replica is told to stop via its own `done` channel - and
make sure a replica that's already lost the race actually reacts to
that, rather than continuing to run to completion regardless.

The signatures must stay the same:

```go
type Replica func(done <-chan struct{}) (string, error)

func FetchFastest(done <-chan struct{}, replicas ...Replica) (string, error)
```

- `FetchFastest` calls every replica concurrently (one goroutine each)
  and returns the value and error from whichever one sends on its own
  result channel FIRST. Once there's a winner, ignore/discard any
  later stragglers.
- If the caller-supplied `done` is closed before any replica responds
  (including if it's already closed when `FetchFastest` is called),
  `FetchFastest` returns promptly with an error and no winner.
- There are two separate triggers that both mean "stop now" for every
  in-flight replica: a replica winning the race, and the
  caller-supplied `done` being closed before anyone won. Either way,
  every replica still running (via the `done` it receives as its own
  argument) must be told to stop instead of being left running for
  however long its artificial latency happens to be.

## Test your solution

To complete this exercise, you must pass the tests:
```
go test
go test --race
```
