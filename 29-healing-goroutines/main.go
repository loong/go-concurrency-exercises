//////////////////////////////////////////////////////////////////////
//
// Given is a "ward": some long-running goroutine, monitored through
// its own heartbeat channel, that is supposed to keep pulsing for as
// long as it's alive. Real wards can wedge - hang or deadlock and
// stop pulsing forever, without ever exiting, and without honoring
// cancellation through the done channel they were given (mockward.go
// simulates exactly this so the exercise is testable).
//
// A "steward" is supposed to watch a ward's heartbeat and, if it ever
// goes silent for longer than a configured timeout, restart it: kick
// off a brand-new "generation" of the ward with a fresh done that
// only the steward itself controls, and keep forwarding whichever
// generation is currently alive onto the steward's own outward-facing
// heartbeat. Crucially, a steward exposes the exact same
// StartGoroutineFn shape as the ward it watches, so stewards compose
// - a steward watching a steward watching a ward, and so on.
//
// NewSteward below does none of that. It starts exactly ONE ward
// generation, passing its OWN incoming done straight through to the
// ward instead of handing it a steward-owned per-generation done, and
// then just hands back that ward's heartbeat channel directly as its
// own. There is no timeout tracking, no restart logic, and no
// per-generation done anywhere. The moment that one ward generation
// wedges, the steward's own returned heartbeat channel goes silent
// right along with it, forever - exactly the single point of failure
// a steward is supposed to eliminate.
//
// Your task is to make NewSteward actually monitor the ward and
// restart it on a stall:
//
//   - Start a ward generation with a fresh, steward-owned wardDone
//     (never the steward's own incoming done) - call
//     ward(wardDone, pulseInterval).
//   - Forward every pulse the current generation's heartbeat sends
//     onto the steward's own returned heartbeat.
//   - Track the time elapsed since the last forwarded pulse. If that
//     ever exceeds timeout, close the current generation's wardDone
//     (best effort - a genuinely wedged ward may never honor it, and
//     the steward must NOT block waiting for it to) and immediately
//     start a brand-new generation with a brand-new wardDone,
//     continuing to forward its pulses from then on.
//   - If the steward's own incoming done is closed, close whatever
//     the current generation's wardDone is and stop everything,
//     including no longer sending on the steward's own heartbeat.
//

package main

import (
	"fmt"
	"time"
)

// StartGoroutineFn is the shape shared by every goroutine that can be
// launched, monitored and restarted this way: given a done channel to
// signal it should stop, and how often it should pulse, it starts and
// returns a heartbeat channel that it sends on roughly every
// pulseInterval for as long as it's alive.
type StartGoroutineFn func(done <-chan struct{}, pulseInterval time.Duration) (heartbeat <-chan struct{})

// NewSteward wraps ward with monitoring: it returns a StartGoroutineFn
// with the exact same shape as ward itself, so stewards compose. The
// returned function is supposed to restart ward with a fresh
// generation whenever more than timeout elapses without a pulse -
// but this naive version does no monitoring at all.
func NewSteward(timeout time.Duration, ward StartGoroutineFn) StartGoroutineFn {
	return func(done <-chan struct{}, pulseInterval time.Duration) <-chan struct{} {
		// BUG: passes the steward's own incoming done straight
		// through to the ward, and just returns the ward's heartbeat
		// as-is. There's no steward-owned wardDone, no timeout
		// tracking, and no restart: if this one ward generation ever
		// wedges, the heartbeat channel this function just returned
		// goes silent forever right along with it.
		return ward(done, pulseInterval)
	}
}

func main() {
	const (
		pulseInterval     = 50 * time.Millisecond
		timeout           = 200 * time.Millisecond
		pulsesBeforeWedge = 4
	)

	ward := NewMockWard(pulsesBeforeWedge)
	steward := NewSteward(timeout, ward.Start)

	done := make(chan struct{})
	heartbeat := steward(done, pulseInterval)

	for i := 1; i <= 10; i++ {
		select {
		case <-heartbeat:
			fmt.Println("pulse", i)
		case <-time.After(pulseInterval * 4):
			fmt.Println("no pulse this round")
		}
	}

	close(done)
	fmt.Println("ward generations started:", ward.Generations())
}
