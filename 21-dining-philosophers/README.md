# Dining Philosophers: Deadlock Avoidance

Five philosophers sit around a round table. Between each adjacent pair
of philosophers sits exactly one fork, so there are as many forks as
philosophers. To eat, a philosopher needs to pick up BOTH the fork to
their left and the fork to their right - only then can they eat, after
which they put both forks back down so their neighbors can use them.

`Dine` is supposed to let every philosopher finish all of their meals
without the table ever grinding to a halt. But the current
implementation always acquires forks in the same order for every
philosopher - left fork first, then right fork - which is exactly the
textbook setup for deadlock: if every philosopher picks up their left
fork at roughly the same time, every single one of them ends up
holding one fork while waiting forever for their right fork, which is
being held by their neighbor who is, in turn, waiting for THEM to
release the fork they're holding. It's a perfect circle of waiting, so
nobody ever eats again.

Your task is to fix `Dine` so it can never deadlock, using a standard,
well-known deadlock-avoidance strategy. The simplest and most
idiomatic fix here is **resource ordering**: change fork acquisition
so every philosopher always locks the lower-indexed fork of their two
forks first, regardless of which one happens to be their "left" fork
and which their "right". Because every philosopher then agrees on a
single global order for acquiring any two forks, the circular-wait
condition that causes the deadlock can never arise.

(An equally valid alternative is an arbitrator/semaphore that only
allows `numPhilosophers - 1` philosophers to attempt to pick up forks
at once, which also breaks the circular wait - either approach is
acceptable, but resource ordering is simplest to implement.)

The function signature must stay the same:

```go
func Dine(numPhilosophers, mealsToEat int) (totalMealsEaten int32)
```

Note: because of the fork-ordering bug, running `go run .` directly
will, in practice, hang forever essentially every time. That's the bug
this exercise is about - the graded artifact is the test suite, which
guards the run with a timeout instead of waiting on it forever.

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
