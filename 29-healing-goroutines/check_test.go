//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"testing"
	"testing/synctest"
	"time"
)

// TestStewardRestartsWedgedWard configures a MockWard that wedges
// after a handful of pulses, and checks that the steward keeps pulses
// flowing on its own heartbeat well past the point an unsupervised
// ward would have gone silent - and that it actually did so by
// starting more than one ward generation, not merely by luck.
func TestStewardRestartsWedgedWard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			pulseInterval     = 100 * time.Millisecond
			timeout           = 350 * time.Millisecond
			pulsesBeforeWedge = 5
			wantPulses        = 12 // more than one generation's worth
		)

		ward := NewMockWard(pulsesBeforeWedge)
		steward := NewSteward(timeout, ward.Start)

		done := make(chan struct{})
		heartbeat := steward(done, pulseInterval)

		received := 0
		deadline := time.After(5 * time.Second)

		for received < wantPulses {
			select {
			case _, ok := <-heartbeat:
				if !ok {
					t.Fatalf("steward's heartbeat channel closed unexpectedly after %d pulse(s)", received)
				}
				received++

			case <-deadline:
				t.Fatalf(
					"timed out after only %d pulse(s); an unsupervised ward would already have gone silent for good after %d pulses (~%v) - the steward does not appear to be restarting it",
					received, pulsesBeforeWedge, time.Duration(pulsesBeforeWedge)*pulseInterval,
				)
			}
		}

		close(done)
		synctest.Wait()

		gensAtShutdown := ward.Generations()
		if gensAtShutdown <= 1 {
			t.Fatalf("expected the steward to have started more than 1 ward generation, got %d - a restart does not appear to have actually happened", gensAtShutdown)
		}

		// If the steward were passing its own incoming done straight
		// through as every generation's wardDone (instead of a fresh,
		// steward-owned one per generation), closing done above would
		// look like it stopped everything - but the monitoring loop
		// would still be alive underneath and would still start a new
		// generation on the next stall. Wait out a few more timeouts
		// and confirm no further generation ever gets started once
		// done has been closed.
		time.Sleep(5 * timeout)
		synctest.Wait()

		if after := ward.Generations(); after != gensAtShutdown {
			t.Fatalf(
				"steward started %d more ward generation(s) after its own done was closed (%d -> %d); closing done must stop it entirely, not just this generation",
				after-gensAtShutdown, gensAtShutdown, after,
			)
		}
	})
}

// TestStewardUsesFreshPerGenerationDone checks that each ward
// generation is started with its own steward-owned wardDone, never
// the steward's own incoming done passed straight through unchanged.
//
// MockWard's wedge is entirely self-triggered by its own pulse
// counter - it never actually looks at whether its own wardDone was
// closed - so a steward that has fully correct restart-on-stall logic
// but reuses the caller's own done as every generation's wardDone
// would still pass TestStewardRestartsWedgedWard: restarts would
// still happen right on schedule regardless of what done value each
// generation received. This test catches that by inspecting the done
// channel each generation actually received, via MockWard.Dones.
func TestStewardUsesFreshPerGenerationDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			pulseInterval     = 100 * time.Millisecond
			timeout           = 350 * time.Millisecond
			pulsesBeforeWedge = 5
			wantGenerations   = 3
		)

		ward := NewMockWard(pulsesBeforeWedge)
		steward := NewSteward(timeout, ward.Start)

		done := make(chan struct{})
		heartbeat := steward(done, pulseInterval)

		deadline := time.After(5 * time.Second)
		for ward.Generations() < wantGenerations {
			select {
			case <-heartbeat:
			case <-deadline:
				t.Fatalf(
					"timed out waiting for %d ward generations to start; only %d started",
					wantGenerations, ward.Generations(),
				)
			}
		}

		close(done)
		synctest.Wait()

		dones := ward.Dones()
		if len(dones) < wantGenerations {
			t.Fatalf("expected at least %d recorded generation(s), got %d", wantGenerations, len(dones))
		}

		for i, d := range dones {
			if d == done {
				t.Fatalf(
					"generation %d was started with the steward's own incoming done instead of a fresh, steward-owned wardDone",
					i+1,
				)
			}
		}

		for i := 1; i < len(dones); i++ {
			if dones[i] == dones[i-1] {
				t.Fatalf(
					"generation %d was started with the same done channel as generation %d - each generation must get its own fresh wardDone",
					i+1, i,
				)
			}
		}
	})
}

// TestStewardStopsOnDone checks that closing the steward's own done
// channel stops it from forwarding any further pulses, and that doing
// so never hangs - even while a ward generation is alive and healthy.
// Along the way, it also checks that a healthy ward that never stalls
// is never restarted: a steward that churns generations on some fixed
// cadence instead of genuinely tracking "time since last pulse" would
// otherwise slip through undetected.
func TestStewardStopsOnDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			pulseInterval     = 50 * time.Millisecond
			timeout           = 500 * time.Millisecond
			pulsesBeforeWedge = 1000 // effectively never wedges in this test
			healthyPulses     = 40   // 40 * 50ms = 2s, 4x the timeout
		)

		ward := NewMockWard(pulsesBeforeWedge)
		steward := NewSteward(timeout, ward.Start)

		done := make(chan struct{})
		heartbeat := steward(done, pulseInterval)

		// Read enough pulses from a healthy ward to span several
		// multiples of timeout. A steward that only restarts on an
		// actual stall must never restart here: every pulse resets
		// its "time since last pulse" clock, so a continuously
		// pulsing ward is never judged stalled. A steward that
		// instead restarts on a fixed cadence regardless of ward
		// health - rather than genuinely tracking elapsed time since
		// the last pulse - would already have restarted several
		// times over by now.
		for i := 0; i < healthyPulses; i++ {
			select {
			case <-heartbeat:
			case <-time.After(2 * time.Second):
				t.Fatalf("timed out waiting for pulse %d from a healthy ward", i+1)
			}
		}

		if gens := ward.Generations(); gens != 1 {
			t.Fatalf(
				"expected exactly 1 ward generation after %d pulses from a ward that never stalled, got %d - the steward appears to be restarting unconditionally rather than only on an actual stall",
				healthyPulses, gens,
			)
		}

		close(done)
		synctest.Wait()

		// No further pulses should ever arrive once done is closed.
		select {
		case _, ok := <-heartbeat:
			if ok {
				t.Fatalf("received a pulse after closing done; steward should have stopped sending")
			}
			// A closed heartbeat channel is acceptable too: the
			// steward is allowed to close it on shutdown, it's just
			// not required to.
		case <-time.After(2 * time.Second):
			// No pulse and no close within a generous fake-time
			// window: exactly what we want - the steward went quiet
			// and stayed quiet.
		}
	})
}
