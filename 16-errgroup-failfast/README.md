# Your Own errgroup: Concurrent Tasks with First-Error Capture

Given is a `Group` type meant to mimic the shape of
`golang.org/x/sync/errgroup`'s `Group` (implemented here from scratch,
using only the standard library - no importing that package). `Go`
registers a task; `Wait` blocks until every registered task has
finished and returns the first error encountered, if any.

The current implementation doesn't do what it's supposed to: `Go`
calls the given function immediately, on the calling goroutine,
blocking until it returns - so tasks registered via `Go` run one after
another instead of concurrently, and the shared "first error" field is
read and written without any synchronization even though real usage
always calls `Go` from multiple goroutines racing against `Wait`.

Your task is to fix `Group` so that:

- `Go(f func() error)` launches `f` in its own goroutine immediately
  and returns without blocking, tracking the goroutine via an internal
  `sync.WaitGroup`.
- `Wait() error` blocks until every goroutine launched via `Go` has
  finished, and returns the first error encountered - captured safely
  from concurrently running goroutines (e.g. via `sync.Once` or a
  mutex) even if several tasks fail around the same time, without
  introducing a data race.

No `context`/cancellation support is needed - keep the scope to the
plain `Go`/`Wait` pair, matching real errgroup's base `Group` without
`WithContext`. The signatures must stay the same:

```go
func (g *Group) Go(f func() error)
func (g *Group) Wait() error
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
