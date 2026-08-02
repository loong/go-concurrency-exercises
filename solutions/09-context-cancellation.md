# Context Cancellation & Propagation: A Request Chain That Ignores Its Deadline — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `09-context-cancellation/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`HandleRequest` is supposed to call three downstream layers —
`CallLayerA`, `CallLayerB`, and `CallLayerC` (see `mocklayers.go`) — in
sequence and concatenate their results, honoring the caller's `ctx` so
that a cancelled or expired context makes the whole chain give up
promptly instead of grinding through all three 300ms (`LayerLatency`)
layers regardless.

Each layer (`callLayer` in `mocklayers.go`) is itself well-behaved and
context-aware: it `select`s between its own `LayerLatency` timer and
`ctx.Done()`, and returns early with `ctx.Err()` the instant the
context it's given is cancelled or times out. That machinery is
useless, though, if `HandleRequest` never hands the layers the real
`ctx`:

```go
func HandleRequest(ctx context.Context) (string, error) {
	a, err := CallLayerA(context.Background())
	if err != nil {
		return "", err
	}

	b, err := CallLayerB(context.Background())
	if err != nil {
		return "", err
	}

	c, err := CallLayerC(context.Background())
	if err != nil {
		return "", err
	}

	return a + b + c, nil
}
```

The task is to fix `HandleRequest` so it:

1. passes `ctx` (not `context.Background()`) into each layer call, so
   the caller's deadline/cancellation actually reaches them, and
2. stops the chain promptly once `ctx` is done or a layer has already
   failed, instead of continuing on to the next layer regardless.

The signature — `func HandleRequest(ctx context.Context) (string, error)` —
must stay exactly the same.

## Why the naive version is wrong

Every layer call is hardcoded to `context.Background()`, which is never
cancelled and has no deadline. So no matter what timeout or
cancellation the caller attached to `ctx`, each of the three layers
always runs its full, uninterrupted `LayerLatency` (300ms), and
`HandleRequest` always takes roughly `3 * LayerLatency` = 900ms end to
end:

```
--- FAIL: TestHandleRequestRespectsTimeout (0.00s)
    check_test.go:60: HandleRequest returned no error; want an error from the 200ms timeout expiring (took 900ms)
--- FAIL: TestHandleRequestCancelledMidway (0.00s)
    check_test.go:112: HandleRequest returned no error after cancellation; want an error
    check_test.go:117: HandleRequest took 900ms to return after cancellation, want well under 500ms - looks like it kept running later layers instead of giving up promptly once ctx was cancelled
```

`TestHandleRequestSucceedsWithAmpleTimeout` still passes against the
naive version — with a 2s timeout and only 900ms of real work, nothing
ever actually times out, so the bug doesn't manifest on the happy path.
That's exactly the trap: code that ignores its context can look correct
in easy/local testing and only misbehave once something upstream is
actually slow or a caller sets a tight deadline.

There is only one defect, not two: `context.Background()` in place of
`ctx`. Look again at the naive code — the `if err != nil { return "", err }`
check after each call is already there, and it's already correct; the
naive version does *not* actually "press on to the next layer after a
failure," despite what the exercise's introductory comment and README
say about it. That description is aspirational about the *symptom*, not
literally true of this code as written: with every layer pinned to
`context.Background()`, a layer can never observe a cancelled or
expired context to fail on in the first place, so the correct
early-return check simply never has anything to fire on. The one-line
propagation bug is entirely sufficient to explain both failing tests —
fixing `context.Background()` → `ctx` is the whole fix, and the
existing error checks do the rest for free (see Approach 1 below).

## Approach 1: Propagate `ctx`, check for done/failure between each layer

```go
func HandleRequest(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	a, err := CallLayerA(ctx)
	if err != nil {
		return "", err
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	b, err := CallLayerB(ctx)
	if err != nil {
		return "", err
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	c, err := CallLayerC(ctx)
	if err != nil {
		return "", err
	}

	return a + b + c, nil
}
```

Walking through the fix:

- **Every layer call gets the real `ctx`**, not `context.Background()`.
  Since `callLayer` already `select`s on `ctx.Done()` internally, this
  alone is what lets a timeout or cancellation cut a layer call short
  instead of it sleeping out its full 300ms regardless.
- **`if err != nil { return "", err }` after each call** stops the
  chain the moment a layer itself reports failure (which, once `ctx` is
  actually propagated, is exactly what happens when `ctx` expires or is
  cancelled mid-layer: `callLayer` returns `"", ctx.Err()`). This is
  what turns "layer B failed" into "don't even attempt layer C."
- **The extra `ctx.Err()` checks between calls** are pure defense in
  depth, not load-bearing for this exercise: the minimal fix is
  actually just the three `context.Background()` → `ctx` swaps, with
  the naive code's error-check structure otherwise untouched — that
  alone passes the full suite (`if err != nil` after each call is
  already correct; it just needed a `ctx` capable of ever producing an
  error to check). The extra checks catch the narrow window *between*
  one layer finishing and the next one starting, before that next
  layer's own internal `select` would have caught it — worth having in
  a longer or more complex chain, where that window matters more, and
  it's exactly what the exercise description calls out as the
  general technique: "check `ctx.Err()` (or the error returned by the
  previous layer) between calls." Here, they're a robustness habit
  layered on top of a fix that would work without them.
- Returning immediately with `err` (which, when it originates from a
  timed-out/cancelled `ctx`, is `ctx.Err()` — `context.DeadlineExceeded`
  or `context.Canceled`) is what satisfies
  `TestHandleRequestRespectsTimeout`'s `errors.Is(err, context.DeadlineExceeded)`
  check.

This is a strictly sequential chain — each layer's result feeds into
the final concatenation only after the previous one has completed —
and the fix is correspondingly simple: thread the one `ctx` through
every call, and stop as soon as anything says "done." There isn't a
meaningfully different second approach worth shipping here: a
fan-out/`errgroup`-style design that kicked off `CallLayerA`,
`CallLayerB`, and `CallLayerC` concurrently would change the exercise's
actual semantics (three independent, concurrently-run layers instead
of a sequential chain) and would break `TestHandleRequestCancelledMidway`,
which specifically expects layer B to still be in flight — not already
finished — 350ms in; running everything concurrently would let all
three complete in a single `LayerLatency` (300ms) window, before the
test's cancellation at 350ms ever fires.

## Key takeaways

- Passing `context.Background()` (or any context you constructed
  yourself) instead of a received `ctx` is a common, easy-to-miss way
  to silently break cancellation — the function *looks* context-aware
  (it takes a `ctx` parameter) while actually ignoring it completely.
  Grep for `context.Background()` deep inside a function that also
  takes a `ctx` argument; it's almost always a bug.
- A context-aware dependency only helps if the context it's *given* is
  the real one. `callLayer`'s own `select` on `ctx.Done()` was already
  correct and unchanged throughout this exercise — the entire bug was
  one layer up, in what `HandleRequest` chose to pass down.
- Checking `ctx.Err()` (or the previous call's error) between
  sequential dependent calls, and returning immediately, is what turns
  "the deadline expired" into "stop doing further work," rather than
  merely "notice the deadline expired after the fact."
- A test that passes on the happy path (ample timeout, nothing actually
  slow) doesn't prove context propagation is correct —
  `TestHandleRequestSucceedsWithAmpleTimeout` passed against the fully
  broken naive version. The real test is a tight deadline or a
  cancellation mid-chain, which is exactly what
  `TestHandleRequestRespectsTimeout` and `TestHandleRequestCancelledMidway`
  exercise.
