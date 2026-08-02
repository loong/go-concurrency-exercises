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

// makeSignalChannels returns n fresh, open <-chan struct{} signal
// channels, none of which have been closed yet.
func makeSignalChannels(n int) []chan struct{} {
	chans := make([]chan struct{}, n)
	for i := range chans {
		chans[i] = make(chan struct{})
	}

	return chans
}

// asReadOnly converts a []chan struct{} into the []<-chan struct{}
// shape or's variadic parameter expects.
func asReadOnly(chans []chan struct{}) []<-chan struct{} {
	ro := make([]<-chan struct{}, len(chans))
	for i, c := range chans {
		ro[i] = c
	}

	return ro
}

// TestOrFiresOnAnyChannel is the key test: it closes a channel in the
// MIDDLE of the slice (index 3 of 5, not index 0) and asserts that or
// still unblocks promptly. The naive implementation only ever watches
// channels[0], which is never closed here, so <-combined would block
// forever. This test deliberately runs on the REAL clock (not inside
// synctest.Test) and guards the blocking receive with a select +
// time.After: inside a synctest bubble, a goroutine that never
// unblocks makes every goroutine "durably blocked" with no fake time
// left to advance, which synctest reports as a fatal deadlock panic
// that would crash the whole test binary rather than failing this
// test cleanly. A correct implementation watches every channel and
// unblocks as soon as any one of them closes.
func TestOrFiresOnAnyChannel(t *testing.T) {
	chans := makeSignalChannels(5)

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(chans[3])
	}()

	start := time.Now()
	combined := or(asReadOnly(chans)...)

	select {
	case <-combined:
		elapsed := time.Since(start)
		if elapsed >= 300*time.Millisecond {
			t.Errorf("or() took %s to fire after ch3 closed at ~50ms; "+
				"want well under 300ms - looks like or() isn't watching "+
				"every channel it was given", elapsed)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("or() did not fire within 300ms after channels[3] closed - " +
			"looks like or() isn't watching every channel it was given (only channels[0]?)")
	}
}

// TestOrFiresOnFirstChannel sanity-checks the other direction: closing
// channels[0] must still make or fire promptly. This is the one case
// the naive implementation accidentally gets right (it only watches
// channels[0]), so this test passes both before and after the fix -
// it just guards against the fix accidentally breaking that case.
func TestOrFiresOnFirstChannel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		chans := makeSignalChannels(4)

		go func() {
			time.Sleep(30 * time.Millisecond)
			close(chans[0])
		}()

		start := time.Now()
		combined := or(asReadOnly(chans)...)
		<-combined
		elapsed := time.Since(start)

		if elapsed >= 100*time.Millisecond {
			t.Errorf("or() took %s to fire after ch0 closed at ~30ms; want well under 100ms", elapsed)
		}
	})
}

// TestOrHandlesManyChannels stress-tests the recursive/many-channel
// path specifically: with only 2 or 3 input channels, a broken
// recursive step might still happen to work by accident, but with 20
// channels the divide-and-conquer recursion has to actually thread
// every level's select correctly for the signal buried in the middle
// of the slice to propagate all the way back out. Like
// TestOrFiresOnAnyChannel above, this runs on the real clock with a
// select + time.After guard instead of synctest.Test, since the naive
// implementation hanging forever here would otherwise crash the test
// binary with a synctest deadlock panic instead of failing cleanly.
func TestOrHandlesManyChannels(t *testing.T) {
	const n = 20
	const closeIndex = 12

	chans := makeSignalChannels(n)

	go func() {
		time.Sleep(40 * time.Millisecond)
		close(chans[closeIndex])
	}()

	start := time.Now()
	combined := or(asReadOnly(chans)...)

	select {
	case <-combined:
		elapsed := time.Since(start)
		if elapsed >= 300*time.Millisecond {
			t.Errorf("or() with %d channels took %s to fire after channels[%d] closed at ~40ms; "+
				"want well under 300ms", n, elapsed, closeIndex)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("or() with %d channels did not fire within 300ms after channels[%d] closed", n, closeIndex)
	}
}

// TestOrHandlesEdgeCases checks the two boundary cases explicitly:
// zero input channels (nothing to wait on - the returned channel must
// never close) and exactly one input channel (a trivial passthrough -
// closing it must close the returned channel promptly).
func TestOrHandlesEdgeCases(t *testing.T) {
	t.Run("zero channels", func(t *testing.T) {
		combined := or()
		if combined == nil {
			t.Fatal("or() with no channels returned a nil channel")
		}

		select {
		case <-combined:
			t.Error("or() with no input channels closed on its own; it should never close")
		case <-time.After(20 * time.Millisecond):
			// Expected: nothing to react to, so it never fires.
		}
	})

	t.Run("one channel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := make(chan struct{})

			go func() {
				time.Sleep(20 * time.Millisecond)
				close(ch)
			}()

			start := time.Now()
			combined := or(ch)
			<-combined
			elapsed := time.Since(start)

			if elapsed >= 100*time.Millisecond {
				t.Errorf("or() with a single channel took %s to fire after it closed at ~20ms; "+
					"want well under 100ms", elapsed)
			}
		})
	})
}
