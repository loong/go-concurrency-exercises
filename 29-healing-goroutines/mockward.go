//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"sync"
	"time"
)

// MockWard is a StartGoroutineFn-shaped ward for testing a Steward.
// Every generation it starts pulses normally on each tick of
// pulseInterval, but after pulsesBeforeWedge pulses it "wedges": it
// simulates a genuinely deadlocked goroutine by going silent and no
// longer reacting to its done channel being closed, for the rest of
// that generation's lifetime.
//
// Generations lets a test observe how many separate generations were
// actually started, so a restart can be asserted directly instead of
// being inferred from pulse timing alone. Dones records the actual
// done channel each generation was started with, so a test can also
// confirm each generation got its own fresh, steward-owned wardDone
// rather than the steward's own incoming done passed straight through
// unchanged to every generation.
type MockWard struct {
	pulsesBeforeWedge int

	mu          sync.Mutex
	generations int
	dones       []<-chan struct{}
}

// NewMockWard creates a MockWard whose every generation pulses
// pulsesBeforeWedge times before wedging.
func NewMockWard(pulsesBeforeWedge int) *MockWard {
	return &MockWard{pulsesBeforeWedge: pulsesBeforeWedge}
}

// Generations reports how many separate generations of this ward have
// been started via Start, across its entire lifetime.
func (w *MockWard) Generations() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.generations
}

// Dones reports the done channel each generation was actually started
// with, in the order generations were started. Comparing these
// against the done a test itself created and passed to the steward
// lets the test confirm each generation received its own fresh,
// steward-owned wardDone - not the steward's own incoming done
// reused, unchanged, across every generation.
func (w *MockWard) Dones() []<-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]<-chan struct{}(nil), w.dones...)
}

// Start begins a new generation of the ward. It has the exact shape
// of StartGoroutineFn, so it can be passed directly as the ward
// argument to NewSteward (e.g. NewSteward(timeout, ward.Start)).
//
// The returned generation pulses once per pulseInterval until it has
// pulsed pulsesBeforeWedge times, then wedges: its goroutine simply
// returns, without closing heartbeat (a closed channel would look
// like an infinite stream of pulses to a reader - the opposite of a
// deadlock) and without ever looking at done again. From the outside
// this is indistinguishable from a real deadlock: the heartbeat goes
// silent forever, and closing done has no visible effect.
func (w *MockWard) Start(done <-chan struct{}, pulseInterval time.Duration) <-chan struct{} {
	w.mu.Lock()
	w.generations++
	w.dones = append(w.dones, done)
	w.mu.Unlock()

	heartbeat := make(chan struct{}, 1)

	go func() {
		ticker := time.NewTicker(pulseInterval)
		defer ticker.Stop()

		pulses := 0
		for {
			select {
			case <-done:
				return

			case <-ticker.C:
				pulses++

				select {
				case heartbeat <- struct{}{}:
				default:
				}

				if pulses >= w.pulsesBeforeWedge {
					// Wedged: simulate a deadlock by simply exiting
					// instead of continuing to serve the ticker or
					// done - a real deadlocked goroutine would never
					// come back either, no matter what its caller
					// does.
					return
				}
			}
		}
	}()

	return heartbeat
}
