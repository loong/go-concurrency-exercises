//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// recvWithTimeout reads one value from ch, failing the test if nothing
// arrives within d. It's a hang-safety-net, not the primary assertion:
// a correct implementation delivers essentially instantly, so d only
// needs to be generous enough to never flake on a busy machine.
func recvWithTimeout(t *testing.T, ch <-chan Event, d time.Duration) (Event, bool) {
	t.Helper()

	select {
	case e, ok := <-ch:
		return e, ok
	case <-time.After(d):
		return Event{}, false
	}
}

// TestPubSubBasicDelivery checks that two independent subscribers each
// receive every published event, in order. This passes against the
// naive implementation too, as long as both subscribers keep reading
// promptly (which they do here).
func TestPubSubBasicDelivery(t *testing.T) {
	bus := NewEventBus()

	subA := bus.Subscribe()
	subB := bus.Subscribe()

	events := []Event{
		{Topic: "orders", Data: "1"},
		{Topic: "orders", Data: "2"},
		{Topic: "orders", Data: "3"},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, e := range events {
			bus.Publish(e)
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish did not return in time")
	}

	for i, want := range events {
		gotA, ok := recvWithTimeout(t, subA, time.Second)
		if !ok {
			t.Fatalf("subscriber A: timed out waiting for event %d", i)
		}
		if gotA != want {
			t.Errorf("subscriber A: event %d = %+v, want %+v", i, gotA, want)
		}

		gotB, ok := recvWithTimeout(t, subB, time.Second)
		if !ok {
			t.Fatalf("subscriber B: timed out waiting for event %d", i)
		}
		if gotB != want {
			t.Errorf("subscriber B: event %d = %+v, want %+v", i, gotB, want)
		}
	}
}

// TestUnsubscribeStopsDelivery is the key test proving Unsubscribe
// works: after a subscriber unsubscribes, it must not receive events
// published afterwards, while every other subscriber keeps receiving
// them normally. This does not even compile against the naive
// starting point (there is no Unsubscribe method yet) - that's
// expected, since adding it is exactly the missing piece of the task.
func TestUnsubscribeStopsDelivery(t *testing.T) {
	bus := NewEventBus()

	chA := bus.Subscribe()
	chB := bus.Subscribe()

	var mu sync.Mutex
	var gotA, gotB []Event

	stopA := make(chan struct{})
	stopB := make(chan struct{})

	go func() {
		defer close(stopA)
		for e := range chA {
			mu.Lock()
			gotA = append(gotA, e)
			mu.Unlock()
		}
	}()
	go func() {
		defer close(stopB)
		for e := range chB {
			mu.Lock()
			gotB = append(gotB, e)
			mu.Unlock()
		}
	}()

	event1 := Event{Topic: "orders", Data: "1"}
	event2 := Event{Topic: "orders", Data: "2"}

	bus.Publish(event1)
	// Give both background readers a moment to drain event1 before we
	// unsubscribe A, so the outcome doesn't depend on scheduling luck.
	time.Sleep(50 * time.Millisecond)

	bus.Unsubscribe(chA)

	bus.Publish(event2)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(gotA) != 1 {
		t.Errorf("subscriber A received %d events after unsubscribing, want 1 (only the pre-unsubscribe event): %+v", len(gotA), gotA)
	} else if gotA[0] != event1 {
		t.Errorf("subscriber A's only event = %+v, want %+v", gotA[0], event1)
	}

	want := []Event{event1, event2}
	if len(gotB) != len(want) {
		t.Fatalf("subscriber B received %d events, want %d: %+v", len(gotB), len(want), gotB)
	}
	for i := range want {
		if gotB[i] != want[i] {
			t.Errorf("subscriber B event %d = %+v, want %+v", i, gotB[i], want[i])
		}
	}
}

// TestPublishNeverBlocksOnStalledSubscriber is the key non-hanging
// test. chStalled deliberately is never read from - simulating a dead
// or stuck subscriber. A correct Publish drops events for a full,
// unread subscriber channel instead of blocking, so all Publish calls
// must return promptly regardless of how many events pile up for
// chStalled. Against the naive implementation, which does a blocking
// unbuffered send to every subscriber in turn, Publish permanently
// hangs on the very first send to chStalled - so `done` below never
// fires and this test times out (an easy-to-spot failure, not a
// crash).
func TestPublishNeverBlocksOnStalledSubscriber(t *testing.T) {
	bus := NewEventBus()

	chFast := bus.Subscribe()
	chStalled := bus.Subscribe()
	_ = chStalled // deliberately never read from

	go func() {
		for range chFast {
			// drain as fast as possible
		}
	}()

	const numEvents = 20 // more than any reasonable per-subscriber buffer

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < numEvents; i++ {
			bus.Publish(Event{Topic: "orders", Data: fmt.Sprintf("event-%d", i)})
		}
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Publish blocked: a stalled subscriber that never reads its channel " +
			"must not prevent Publish from returning for every other call/subscriber")
	}
}

// TestPubSubConcurrentAccess hammers Subscribe, Unsubscribe, and
// Publish from many goroutines at once. It makes no assertions beyond
// "doesn't panic or race" - run with `go test -race`.
func TestPubSubConcurrentAccess(t *testing.T) {
	bus := NewEventBus()

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				bus.Publish(Event{Topic: "stress", Data: "x"})
			}
		}()

		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ch := bus.Subscribe()
				// Drain a little so a full buffer doesn't matter here.
				select {
				case <-ch:
				default:
				}
				bus.Unsubscribe(ch)
			}
		}()

		go func() {
			defer wg.Done()
			ch := bus.Subscribe()
			for j := 0; j < iterations; j++ {
				select {
				case <-ch:
				default:
				}
			}
			bus.Unsubscribe(ch)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent Subscribe/Unsubscribe/Publish stress test did not finish in time")
	}
}
