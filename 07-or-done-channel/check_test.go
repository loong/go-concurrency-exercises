//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"testing"
	"time"
)

// TestOrDoneForwardsValues checks that orDone still does its one job
// while done stays open: values read from the wrapped channel must be
// the same sequential, increasing values StartMetricStream emits.
// This passes against both the naive passthrough and a correct
// implementation.
func TestOrDoneForwardsValues(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	out := orDone(done, StartMetricStream())

	var got []int
	for i := 0; i < 5; i++ {
		v, ok := <-out
		if !ok {
			t.Fatalf("out closed early after %d value(s), want at least 5", i)
		}
		got = append(got, v)
	}

	for i := 1; i < len(got); i++ {
		if got[i] != got[i-1]+1 {
			t.Fatalf("expected sequential increasing values, got %v", got)
		}
	}
}

// orDoneStopsPromptly is the shared body of the "does orDone actually
// stop leaking" check: it wires orDone up to a channel that will
// never receive a value (so any unconditional read on it blocks
// forever), starts a goroutine that tries to read from the wrapped
// channel, closes done, and then asserts - via a short, real-time
// safety-net timeout - that the wrapped channel closes promptly
// instead of the reader hanging forever.
//
// c is never written to, so the only way `out` can produce anything
// is by observing done and closing itself. Against the naive
// passthrough (`return c`), out IS c: closing done does nothing to
// it, nobody ever sends on c, and the read below hangs until the
// safety-net timeout fires - which is exactly the leak this exercise
// is about.
func orDoneStopsPromptly(t *testing.T) {
	t.Helper()

	done := make(chan struct{})
	c := make(chan int) // never written to: an unconditional read blocks forever

	out := orDone(done, c)

	result := make(chan bool, 1)
	go func() {
		_, ok := <-out
		result <- ok
	}()

	close(done)

	select {
	case ok := <-result:
		if ok {
			t.Errorf("expected out to be closed (ok == false) once done fires, got a value instead")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("orDone did not stop promptly after done was closed - the forwarding goroutine appears to be leaked, blocked forever on a receive/send that will never happen")
	}
}

// TestOrDoneStopsPromptlyOnDone is the key test: it proves that
// closing done actually unblocks and shuts down orDone's forwarding
// goroutine, rather than leaving it stuck forever.
func TestOrDoneStopsPromptlyOnDone(t *testing.T) {
	orDoneStopsPromptly(t)
}

// TestOrDoneNoGoroutineLeakRace repeats the shutdown scenario a
// number of times so that, run with `go test -race`, it has a
// reasonable chance of catching a data race in a select-based
// shutdown path that only shows up under contention.
func TestOrDoneNoGoroutineLeakRace(t *testing.T) {
	for i := 0; i < 20; i++ {
		orDoneStopsPromptly(t)
	}
}

// orDoneStopsPromptlyMidForward covers a gap orDoneStopsPromptly
// can't: it never writes to c, so an implementation only has to react
// to done while sitting idle at the outer receive. That's not enough
// - the forwarding send onto out can block just as long if it isn't
// also select-guarded on done.
//
// Here c is actively, continuously written to (like StartMetricStream
// does). The test drains exactly one value to confirm forwarding is
// alive, sleeps briefly so the goroutine loops back and pulls the
// next value off c, then closes done while nobody is reading out.
//
// An implementation that guards the receive but not the send, e.g.
//
//	case v := <-c:
//	    out <- v // unconditional - doesn't watch done
//
// passes orDoneStopsPromptly (c there never sends, so this code path
// never runs) but fails here: once it already has a value in hand,
// its bare send blocks regardless of done, and whatever eventually
// reads out next gets that stale, post-done value instead of out
// being closed. This is exactly what main.go demonstrates by printing
// "unexpected: got a value after done closed".
func orDoneStopsPromptlyMidForward(t *testing.T) {
	t.Helper()

	done := make(chan struct{})
	c := make(chan int)

	go func() {
		v := 0
		for {
			v++
			select {
			case c <- v:
			case <-done:
				return
			}
		}
	}()

	out := orDone(done, c)

	<-out
	time.Sleep(20 * time.Millisecond)

	close(done)

	select {
	case v, ok := <-out:
		if ok {
			t.Errorf("expected out to be closed (ok == false) once done fires, got stale value %d instead - the forwarding send must also select on done", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("orDone did not stop promptly after done was closed - the forwarding goroutine appears to be leaked, blocked on a send that will never be read")
	}
}

// TestOrDoneStopsPromptlyMidForward proves that done still shuts
// orDone down promptly even when a value pulled from c is already in
// flight toward out at the moment done closes.
func TestOrDoneStopsPromptlyMidForward(t *testing.T) {
	orDoneStopsPromptlyMidForward(t)
}

// TestOrDoneNoGoroutineLeakMidForwardRace repeats the mid-forward
// shutdown scenario a number of times so that, run with `go test
// -race`, it has a reasonable chance of catching a data race in a
// select-based shutdown path that only shows up under contention.
func TestOrDoneNoGoroutineLeakMidForwardRace(t *testing.T) {
	for i := 0; i < 20; i++ {
		orDoneStopsPromptlyMidForward(t)
	}
}
