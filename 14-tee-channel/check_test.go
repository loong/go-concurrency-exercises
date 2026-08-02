//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// expectedSequence returns the sequence of readings StartSensor(n) is
// expected to emit: 0, 1, ..., n-1.
func expectedSequence(n int) []int {
	seq := make([]int, n)
	for i := range seq {
		seq[i] = i
	}

	return seq
}

// TestTeeDuplicatesToBothConsumers is the key test: it proves that
// EVERY value produced by `in` reaches BOTH outputs, even when one
// consumer reads much slower than the other. One goroutine drains its
// output as fast as possible; the other sleeps 2ms between reads to
// simulate a slow consumer. Wrapping the whole thing in synctest.Test
// makes the artificial sleep (and StartSensor's internal 5ms sleeps)
// resolve on a fake, deterministic clock instead of the real one, so
// the test is fast and doesn't flake under load.
//
// Against the naive Tee (`return in, in`), out1 and out2 are literally
// the same channel: the two consumer goroutines race to receive from
// it, so each of the 10 values goes to only ONE of them, never both.
// The slower consumer, in particular, tends to lose almost every race
// against the fast one, so its slice ends up far from complete. Either
// way, at least one of the two slices ends up missing values it
// should have received, so the exact equality checks below fail.
func TestTeeDuplicatesToBothConsumers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		done := make(chan struct{})
		defer close(done)

		in := StartSensor(10)
		out1, out2 := Tee(done, in)

		var fast, slow []int
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			for v := range out1 {
				fast = append(fast, v)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for v := range out2 {
				time.Sleep(2 * time.Millisecond)
				slow = append(slow, v)
			}
		}()

		wg.Wait()

		want := expectedSequence(10)

		if !reflect.DeepEqual(fast, want) {
			t.Errorf("fast consumer got %v, want %v (every value must reach every consumer)", fast, want)
		}
		if !reflect.DeepEqual(slow, want) {
			t.Errorf("slow consumer got %v, want %v (every value must reach every consumer)", slow, want)
		}
	})
}

// assertClosesPromptly reads once from ch and fails unless it observes
// ch already closed within a short, real-time safety-net window. It is
// used to detect a Tee that ignores `done` and keeps its outputs open
// (or keeps delivering stale values) instead of shutting down.
func assertClosesPromptly(t *testing.T, ch <-chan int, name string) {
	t.Helper()

	select {
	case v, ok := <-ch:
		if ok {
			t.Fatalf("expected %s to be closed once done fires, got value %d instead", name, v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("%s did not close promptly after done was closed", name)
	}
}

// teeStopsOnDoneOnce wires Tee up to a sensor that produces far more
// values than the test will ever consume, reads a couple of full
// rounds from both outputs, closes done, and asserts that both outputs
// close promptly afterwards instead of continuing to deliver values or
// hanging forever.
//
// Against the naive Tee, out1 and out2 are the same channel as `in`,
// which never learns that done was closed: it keeps sleeping 5ms and
// sending its next reading regardless, so the very next read below
// returns a real value instead of observing the channel closed - which
// assertClosesPromptly reports as a failure.
func teeStopsOnDoneOnce(t *testing.T) {
	t.Helper()

	done := make(chan struct{})
	in := StartSensor(50) // far more values than this test will ever consume

	out1, out2 := Tee(done, in)

	// Drain a couple of full rounds so both outputs have made progress
	// before we ask everything to shut down.
	<-out1
	<-out2
	<-out1
	<-out2

	close(done)

	assertClosesPromptly(t, out1, "out1")
	assertClosesPromptly(t, out2, "out2")
}

// TestTeeStopsOnDone checks that closing done causes Tee to stop
// promptly, closing both of its output channels rather than hanging
// onto them (or the goroutine feeding them) forever.
func TestTeeStopsOnDone(t *testing.T) {
	teeStopsOnDoneOnce(t)
}

// TestTeeStopsOnDoneRace repeats the shutdown scenario a number of
// times so that, run with `go test -race`, it has a reasonable chance
// of catching a data race in a select-based shutdown path that only
// shows up under contention.
func TestTeeStopsOnDoneRace(t *testing.T) {
	for i := 0; i < 20; i++ {
		teeStopsOnDoneOnce(t)
	}
}
