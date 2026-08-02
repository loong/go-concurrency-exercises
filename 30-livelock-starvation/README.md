# Livelock & Starvation: Two Failure Modes Beyond Deadlock

Deadlock (see [21-dining-philosophers](../21-dining-philosophers)) isn't
the only way concurrent code can fail to make progress. This exercise
covers two related but *distinct* failure modes, one small deterministic
scenario each, in the same package:

- **Livelock**: goroutines are actively running - burning CPU, never
  blocked - but making no forward progress, because they keep reacting to
  each other in lockstep. Unlike deadlock, nobody is stuck; everybody is
  busy. That's what makes it sneakier to notice from the outside (CPU
  usage looks "healthy") and harder to diagnose.
- **Starvation**: a goroutine is perpetually denied a resource because of
  an unfair scheduling/granting policy, even though the system as a whole
  keeps making progress and neither deadlock nor livelock is occurring.
  Everyone else is fine; one specific goroutine just never gets its turn.

## Part 1 - Livelock: `RunLivelockDemo`

Two workers, A and B, each need brief exclusive access to one shared
resource, up to `attempts` times each. Given is:

```go
func RunLivelockDemo(attempts int) (aProgress, bProgress int)
```

On contention, a worker that fails to acquire the resource backs off
briefly and retries, rather than blocking indefinitely - that's supposed
to be the *fix* for the thundering-herd problem of two goroutines
hammering a lock. But the naive implementation below backs off for the
exact same fixed duration every single time, with **no randomness at
all**. Both workers start their attempt loop at the same instant, so
whenever they collide, they both wait out the identical delay and then
retry at the exact same instant again - a perfect, self-sustaining
lockstep. They collide, back off, collide, back off, forever: `aProgress`
and `bProgress` both stay at (or near) zero even after `attempts` is
fairly large, because neither of them ever manages to be the sole
contender in a round.

(There's a bounded overall retry budget under the hood so the naive
version is guaranteed to eventually return instead of spinning forever -
that's scaffolding, not the fix. Running `go run .` directly will churn
for several real seconds printing no progress for either worker before
that budget is exhausted - that's the bug this half of the exercise is
about. The graded artifact is the test suite, which uses `testing/synctest`
so it doesn't have to burn any real wall-clock time waiting on it.)

Each attempt round runs through a fixed, five-step protocol - **this
protocol is required scaffolding, not part of the bug and not part of
the fix; keep both its shape and its exact `probeWindow` value as-is**
(`check_test.go`'s minimum-elapsed-time check is calibrated against
that value):

1. Announce as a contender for this round.
2. Sleep `probeWindow` so every contender gets a chance to announce
   before anyone reads.
3. Snapshot how many contenders announced this round.
4. Sleep `probeWindow` again so nobody withdraws before every contender
   has taken its own snapshot.
5. Withdraw, then act on the step-3 snapshot: exactly one contender
   means an uncontested win; more than one means a collision, and every
   colliding worker backs off before retrying.

It's this protocol - not a discardable side mechanism - that makes
contention between the two workers deterministic and reproducible under
`testing/synctest`, on every run, regardless of the surrounding
implementation. The **one** thing that has to change from the naive
version is step 5's backoff duration: give each worker its own
independently-seeded source of randomized jitter instead of the
identical fixed duration shared by both. Once the two workers' backoff
durations are no longer perfectly synchronized, the lockstep breaks on
its own: sooner or later one of them retries while the other is still
backed off, wins the resource alone, and from there both workers
accumulate steady real progress.

## Part 2 - Starvation: `Dispatcher`

```go
type Dispatcher struct{ /* unexported fields, your choice */ }

func NewDispatcher() *Dispatcher
func (d *Dispatcher) SubmitHighPriority(job int)
func (d *Dispatcher) SubmitLowPriority(job int)
func (d *Dispatcher) RunDispatchCycles(n int) (highCompleted, lowCompleted int)
```

`SubmitHighPriority` and `SubmitLowPriority` enqueue a job onto the
dispatcher's high- or low-priority queue. `RunDispatchCycles(n)` runs `n`
dispatch cycles; each cycle picks exactly one currently-queued job and
runs it to completion, recording it in `highCompleted` or `lowCompleted`
depending on which queue it came from, based on whatever is queued *at
that instant* - `Submit*` calls may happen before or between calls to
`RunDispatchCycles`.

The naive policy below is strict, never-aging priority: a cycle drains a
high-priority job if one is queued, full stop, and only ever looks at the
low-priority queue when the high-priority queue is completely empty. That
sounds reasonable - urgent work first - but it means that as long as
*something* keeps the high-priority queue non-empty (a steady trickle of
new high-priority submissions, or just a big enough backlog submitted up
front), a low-priority job sitting right behind it can wait forever. The
dispatcher is always doing useful work, so this is neither deadlock nor
livelock - that's exactly the point of contrasting it with Part 1. It's a
policy problem, not a synchronization bug.

Your task: make `RunDispatchCycles` fair. A low-priority job that's
waiting must be guaranteed to run within a *bounded* number of cycles, no
matter how deep the high-priority backlog is - for example, guarantee at
least 1 out of every K cycles goes to a low-priority job if one is
waiting, regardless of backlog size (weighted round-robin / aging).
High-priority jobs must still get the large majority of cycles when both
queues stay busy for the whole run - concretely, at least two thirds of
completed cycles must go to high-priority work in that situation; only
the starvation has to go away.

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
