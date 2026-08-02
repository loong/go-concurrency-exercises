# Healing Unhealthy Goroutines: A Steward That Restarts a Wedged Ward

Given is a `NewSteward` that is supposed to watch over a long-running
"ward" goroutine (see `mockward.go` for the test double standing in
for it) through the heartbeat it pulses, and transparently restart it
if it ever goes quiet for too long - but right now it does nothing of
the sort.

A ward is any goroutine matching this shape:

```go
type StartGoroutineFn func(done <-chan struct{}, pulseInterval time.Duration) (heartbeat <-chan struct{})
```

Starting a ward gives it a `done` channel it should stop on, and how
often (`pulseInterval`) it's expected to pulse on the `heartbeat`
channel it returns. Real wards can wedge: hang or deadlock and stop
pulsing forever, without ever exiting and without honoring `done`
being closed. Right now, if the one ward a steward is watching wedges,
the steward's own heartbeat goes silent right along with it, forever.

Your task is to implement `NewSteward` so it actually monitors and
heals the ward it wraps:

```go
func NewSteward(timeout time.Duration, ward StartGoroutineFn) StartGoroutineFn
```

`NewSteward` returns a `StartGoroutineFn` - a steward has the exact
same shape as the thing it watches, so stewards compose (a steward can
watch a steward watching a ward, and so on). When that returned
function is called with `(done, pulseInterval)`, it must:

- Start a ward generation by calling `ward(wardDone, pulseInterval)`
  with a fresh, **steward-owned** `wardDone` - never the steward's own
  incoming `done` passed straight through.
- Forward every pulse the current generation's heartbeat sends onto
  the steward's own returned heartbeat.
- Track the time elapsed since the last forwarded pulse. If more than
  `timeout` elapses with no pulse from the current generation, close
  that generation's `wardDone` (telling it to stop - even though a
  truly wedged ward may never actually honor that, so the steward must
  **not** block waiting for it to) and immediately start a brand-new
  generation with a brand-new `wardDone`, continuing to forward its
  pulses from then on.
- If the steward's own incoming `done` is closed, close whatever the
  current generation's `wardDone` is and stop everything, including no
  longer sending on its own heartbeat.

## The mock ward

`mockward.go` (do not edit) provides a `MockWard` you'll see used in
`check_test.go`:

```go
func NewMockWard(pulsesBeforeWedge int) *MockWard
func (w *MockWard) Start(done <-chan struct{}, pulseInterval time.Duration) <-chan struct{}
func (w *MockWard) Generations() int
func (w *MockWard) Dones() []<-chan struct{}
```

`(*MockWard).Start` has the exact shape of `StartGoroutineFn`, so it
can be passed straight in as `NewSteward(timeout, ward.Start)`. Each
generation it starts pulses normally for `pulsesBeforeWedge` pulses,
then wedges: it goes silent and stops reacting to `done` for the rest
of that generation's life, simulating a real deadlock. `Generations()`
reports how many separate generations have been started in total, so
a test (or you, in `main`) can confirm a restart genuinely happened
rather than guessing from pulse timing alone. `Dones()` reports the
actual `done` channel each generation was started with, in order, so
a test can also confirm each generation got its own fresh,
steward-owned `wardDone` rather than the steward's own incoming `done`
reused unchanged across every generation.

## Test your solution

To complete this exercise, you must pass the tests:
```
go test
go test --race
```
