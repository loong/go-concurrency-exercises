# Priority Worker Pool: Urgent Jobs Shouldn't Wait Behind a Backlog

Given is a job scheduler with exactly one worker goroutine (deliberately
just one: with a single worker, the order jobs complete in is fully
determined by the order they're picked up, which is what makes this
exercise testable without any ambiguity from multiple workers racing
to grab the next job). The scheduler is supposed to run submitted jobs
so that, whenever the worker is about to start a new one and more than
one job is currently waiting, it always picks the HIGHEST-priority job
waiting - so an urgent job submitted after a pile of low-priority ones
doesn't have to sit behind all of them.

This builds on the [worker-pool exercise](../11-worker-pool): that one
hands every job the same treatment and only cares about draining a
shared queue with several workers running in parallel, in whatever
order jobs happen to arrive. Here there's only one worker, but the
queue itself has to be priority-aware: a plain FIFO channel can't
reorder what's already sitting inside it, so as soon as a job is queued
behind others its priority stops mattering. The current implementation
is exactly that - a single channel of `Job`s that the one worker drains
strictly in the order jobs were sent, with no regard for `Priority` at
all.

Your task is to reimplement `Scheduler` so that it keeps waiting jobs
in an internal priority queue (`container/heap` is the natural tool)
guarded by a mutex, instead of a plain channel. Whenever the worker
finishes a job and needs to pick up the next one, it must take the
highest-`Priority` job currently in the queue; ties are broken by
earliest submission time, i.e. FIFO among jobs of equal priority. When
the queue is empty the worker has to block efficiently - a
`sync.Cond`, or a small signaling channel, either is fine, as long as
it's correct and doesn't busy-poll in a tight spin loop - until
`Submit` adds something and wakes it up.

The exported API must stay exactly the same, so `Scheduler` remains a
drop-in replacement for the version below:

```go
func NewScheduler() *Scheduler
func (s *Scheduler) Submit(job Job)
func (s *Scheduler) Completed() <-chan int
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
