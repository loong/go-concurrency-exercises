//////////////////////////////////////////////////////////////////////
//
// Given is an in-memory event bus, EventBus, that is supposed to let
// any number of independent subscribers listen for events published
// to it. Multiple goroutines subscribe, unsubscribe, and publish at
// the same time, so the bus has to stay safe under that concurrency.
//
// The naive implementation below is broken in two ways:
//
//  1. There is no way to unsubscribe. Once a caller calls Subscribe,
//     it is stuck listening forever - there's no way to tell the bus
//     "I'm done, stop sending to me and forget about my channel". A
//     caller that goes away without draining its channel leaks it
//     inside the bus forever.
//
//  2. Publish sends to every subscriber's channel one at a time with
//     a blocking, unbuffered send. If even one subscriber is slow, or
//     has stopped reading altogether, that single blocking send
//     freezes the whole Publish call - which means it freezes every
//     other subscriber too, and every future call to Publish, for as
//     long as the stalled subscriber never reads.
//
// Your task is to fix EventBus so that:
//
//  1. Subscribe returns a channel that a caller can stop listening to
//     by calling the new method Unsubscribe(ch) with that same
//     channel. After Unsubscribe returns, the bus must no longer send
//     to that channel or keep a reference to it, so it can't leak and
//     doesn't receive any event published afterwards. Subscribe,
//     Unsubscribe, and Publish must all be safe to call concurrently
//     from any number of goroutines.
//
//  2. Publish must never block indefinitely because one subscriber is
//     slow or stalled. Give each subscriber's channel a reasonable
//     buffer (e.g. 8 events) and, when that buffer is full, drop the
//     event for that subscriber only - via a non-blocking send
//     (select with a default case) - instead of blocking Publish or
//     affecting delivery to any other subscriber.
//
// The signatures below must stay exactly as they are, so existing
// callers keep working:
//
//     func NewEventBus() *EventBus
//     func (b *EventBus) Subscribe() <-chan Event
//     func (b *EventBus) Unsubscribe(ch <-chan Event)
//     func (b *EventBus) Publish(e Event)
//

package main

import (
	"fmt"
	"sync"
	"time"
)

// Event is a single message flowing through the bus.
type Event struct {
	Topic string
	Data  string
}

// EventBus is supposed to let any number of subscribers listen for
// published events, support cleanly unsubscribing so a caller who's
// done listening doesn't leak resources or keep getting values pushed
// at it, and make sure a slow or stalled subscriber can never cause
// Publish to block forever waiting on it. Right now it does none of
// that: subscribers can never unsubscribe, and Publish sends to every
// subscriber's channel one at a time with a blocking, unbuffered send
// - so a single subscriber who stops reading freezes every future
// Publish call for everyone, forever.
type EventBus struct {
	mu          sync.Mutex
	subscribers []chan Event
}

// NewEventBus creates an empty, ready-to-use EventBus.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe registers a new subscriber and returns the channel that
// events published from now on will be delivered to.
func (b *EventBus) Subscribe() <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event)
	b.subscribers = append(b.subscribers, ch)

	return ch
}

// Publish delivers e to every current subscriber.
func (b *EventBus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subscribers {
		ch <- e
	}
}

func main() {
	bus := NewEventBus()

	subA := bus.Subscribe()
	subB := bus.Subscribe()

	go func() {
		for e := range subA {
			fmt.Printf("subscriber A got: %s/%s\n", e.Topic, e.Data)
		}
	}()

	go func() {
		for e := range subB {
			fmt.Printf("subscriber B got: %s/%s\n", e.Topic, e.Data)
		}
	}()

	for i := 0; i < 3; i++ {
		bus.Publish(Event{Topic: "orders", Data: fmt.Sprintf("event-%d", i)})
		time.Sleep(10 * time.Millisecond)
	}

	// Nothing ever closes subA/subB in this naive version, so the
	// reader goroutines above never actually return - there's no way
	// to unsubscribe and shut them down cleanly. That's fine for this
	// demo: main just exits once it's done publishing.
	time.Sleep(50 * time.Millisecond)
}
