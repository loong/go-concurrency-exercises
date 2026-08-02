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

// UnitDuration is how long a single, healthy work unit takes to
// simulate.
const UnitDuration = 400 * time.Millisecond

// StallDuration is how long a *stalled* unit blocks for: one single,
// un-checkpointed wait, standing in for a worker wedged on something
// unresponsive downstream (a hung network call, a deadlock, a stuck
// disk read, ...) that offers no opportunity to check back in at all
// until it finally gives up - unlike a healthy unit, which
// cooperatively checks in every pulseInterval along the way.
const StallDuration = 24 * time.Hour

var (
	stallMu    sync.Mutex
	stallIndex = -1
)

// SetStallUnit configures the (0-based) work-unit index that
// SimulateUnit should simulate a stall for. Pass a negative index, or
// one that is out of range for the job's work units, to disable
// stalling entirely - every unit then just runs at its normal
// UnitDuration. Safe for concurrent use; concurrent callers that don't
// care about stalling should all pass a negative index so they don't
// fight over this shared, package-level configuration.
func SetStallUnit(index int) {
	stallMu.Lock()
	stallIndex = index
	stallMu.Unlock()
}

func isStallUnit(index int) bool {
	stallMu.Lock()
	defer stallMu.Unlock()
	return index == stallIndex
}

// SimulateUnit simulates performing one full unit of work at position
// unitIndex, respecting done throughout.
//
// Under normal conditions it takes UnitDuration, split into
// pulseInterval-sized slices - after every slice except the very
// last, it calls checkIn() to give the caller a chance to report that
// it is still actively working (e.g. by pulsing a heartbeat) before
// continuing. If checkIn returns false, SimulateUnit stops early and
// returns false itself.
//
// If unitIndex was configured via SetStallUnit to stall, none of that
// happens: SimulateUnit instead blocks for one single, un-checkpointed
// StallDuration wait - checkIn is never called - simulating a worker
// wedged on an unresponsive step with no chance to check back in
// before it (eventually, if ever) returns.
//
// SimulateUnit returns true if the unit ran to completion, or false if
// done fired (or checkIn asked to stop) before that happened.
func SimulateUnit(done <-chan struct{}, unitIndex int, pulseInterval time.Duration, checkIn func() bool) bool {
	if isStallUnit(unitIndex) {
		timer := time.NewTimer(StallDuration)
		defer timer.Stop()

		select {
		case <-timer.C:
			return true
		case <-done:
			return false
		}
	}

	remaining := UnitDuration
	for remaining > 0 {
		slice := pulseInterval
		if slice > remaining {
			slice = remaining
		}

		timer := time.NewTimer(slice)
		select {
		case <-timer.C:
		case <-done:
			timer.Stop()
			return false
		}
		timer.Stop()

		remaining -= slice
		if remaining > 0 {
			if !checkIn() {
				return false
			}
		}
	}

	return true
}
