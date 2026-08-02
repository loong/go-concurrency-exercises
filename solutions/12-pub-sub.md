# Pub-Sub: In-Memory Event Bus — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `12-pub-sub/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`EventBus` is supposed to let any number of independent subscribers listen for
events published to it, with `Subscribe`, `Unsubscribe`, and `Publish` all safe
to call concurrently from any number of goroutines. Two things need fixing:

1. There is no way to unsubscribe at all — a caller that's done listening has
   no way to tell the bus "stop sending to me, forget my channel," so it leaks
   inside the bus forever.
2. `Publish` sends to every subscriber with a blocking, unbuffered send. One
   slow or stalled subscriber freezes delivery to everyone else, and every
   future `Publish` call, for as long as it never reads.

The starting `main.go` doesn't even compile against `check_test.go` — there's
no `Unsubscribe` method yet. That's by design: writing it is exactly the
missing half of the task, not a "make the tests pass" afterthought.

## Why the naive version is wrong

The two bugs named in the file comment are real, but there's a third,
sharper one underneath them: the blocking send happens **while holding
`b.mu`**.

```go
func (b *EventBus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subscribers {
		ch <- e // blocking send, inside the critical section
	}
}
```

So a stalled subscriber doesn't just freeze `Publish` — it freezes the entire
bus. Any other goroutine calling `Subscribe` (or, once it exists,
`Unsubscribe`) blocks trying to acquire `b.mu`, which `Publish` is holding
forever waiting on a channel nobody reads. One stuck reader deadlocks every
public method on the type.

The general lesson: **never perform a potentially-blocking operation inside a
critical section.** The fix below keeps the lock (state still needs
protecting) but makes every send inside it non-blocking, so the critical
section's duration no longer depends on subscriber behavior.

One more wrinkle worth naming up front: `Unsubscribe` takes a `<-chan Event`
(receive-only), but `Subscribe` necessarily creates and stores a bidirectional
`chan Event` (it needs to send on it). Go won't let you convert a
receive-only channel back to bidirectional, so you can't index a
`map[chan Event]...` directly with the `<-chan Event` you're given — both
approaches below deal with this by linear-scanning the subscriber set and
comparing with `==`, which Go permits here because a `chan Event` value is
assignable to `<-chan Event` (comparing values of assignable-but-different
channel types is legal). If this were a hot path at scale, keying the map by
the receive-only view instead — `map[<-chan Event]chan Event`, storing
`subs[(<-chan Event)(ch)] = ch` — would make removal O(1); it's not needed for
this exercise's scale, so the verified code below keeps the simpler scan.

Also worth calling out: `Unsubscribe` deliberately does **not** close `ch`.
The contract only requires the bus to forget about the channel, not close it
— closing is a natural first instinct, but it's wrong here, since nothing
guarantees no other code path might still try to send to it.

## Approach 1: buffered channels + non-blocking `Publish`, mutex-protected registry

The subscriber set is a `map[chan Event]struct{}` guarded by a plain
`sync.Mutex`. Each subscriber gets a buffered channel (buffer size 8);
`Publish` does a non-blocking `select`/`default` send to each one, so a full
buffer just drops that one event for that one subscriber instead of blocking
anything else. `Unsubscribe` takes the same lock and deletes the entry
directly.

```go
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

const subscriberBuffer = 8

// EventBus lets any number of subscribers listen for published
// events, supports cleanly unsubscribing, and makes sure a slow or
// stalled subscriber can never cause Publish to block.
type EventBus struct {
	mu          sync.Mutex
	subscribers map[chan Event]struct{}
}

// NewEventBus creates an empty, ready-to-use EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a new subscriber and returns the channel that
// events published from now on will be delivered to.
func (b *EventBus) Subscribe() <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, subscriberBuffer)
	b.subscribers[ch] = struct{}{}

	return ch
}

// Unsubscribe removes ch from the bus so it no longer receives
// published events. After Unsubscribe returns, the bus holds no
// reference to ch.
func (b *EventBus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subscribers {
		if sub == ch {
			delete(b.subscribers, sub)
			return
		}
	}
}

// Publish delivers e to every current subscriber. If a subscriber's
// buffer is full, the event is dropped for that subscriber only -
// Publish never blocks waiting on a slow or stalled subscriber.
func (b *EventBus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
}
```

Why this satisfies the requirements:

- The buffer (8) absorbs short bursts so a briefly-slow subscriber doesn't
  lose events, but a permanently stalled one (like `chStalled` in
  `TestPublishNeverBlocksOnStalledSubscriber`) just fills up and every
  subsequent event for it gets dropped via the `default` case — `Publish`
  itself never blocks on it.
- `Subscribe`/`Unsubscribe`/`Publish` all take the same mutex for the
  minimal duration needed to mutate the map or iterate it with non-blocking
  sends — never for a blocking operation — so none of them can freeze the
  others.
- Once `Unsubscribe` returns, `sub` has been deleted from the map under the
  same lock `Publish` uses, so no future `Publish` call can observe it.

## Approach 2: actor/monitor — single goroutine owns the state, no mutex

A genuinely different way to get the same safety: instead of a mutex guarding
shared state that any goroutine can touch, a single dedicated goroutine
(`run`) owns the subscriber map outright, and `Subscribe`/`Unsubscribe`/
`Publish` interact with it only by sending commands over channels — "share
memory by communicating" instead of "share memory, protect it with a lock."
There is no `sync.Mutex` anywhere in this version; serialization is
structural (only one goroutine ever touches the map), not enforced by lock
discipline.

```go
package main

import (
	"fmt"
	"time"
)

// Event is a single message flowing through the bus.
type Event struct {
	Topic string
	Data  string
}

const subscriberBuffer = 8

// subscribeReq/unsubscribeReq are commands sent to the bus's owner
// goroutine. Nothing outside run() ever touches the subscribers map -
// state is owned by a single goroutine and shared by communicating
// with it, not by locking.
type subscribeReq struct {
	resp chan chan Event
}

type unsubscribeReq struct {
	ch   <-chan Event
	done chan struct{} // closed once removal has actually happened
}

// EventBus lets any number of subscribers listen for published
// events, supports cleanly unsubscribing, and makes sure a slow or
// stalled subscriber can never cause Publish to block. Unlike the
// mutex-protected version, there is no lock: a single goroutine (run)
// owns the subscriber set, and Subscribe/Unsubscribe/Publish talk to
// it purely via channels.
type EventBus struct {
	subscribeCh   chan subscribeReq
	unsubscribeCh chan unsubscribeReq
	publishCh     chan Event
}

// NewEventBus creates an empty, ready-to-use EventBus and starts its
// owner goroutine.
func NewEventBus() *EventBus {
	b := &EventBus{
		subscribeCh:   make(chan subscribeReq),
		unsubscribeCh: make(chan unsubscribeReq),
		publishCh:     make(chan Event),
	}
	go b.run()
	return b
}

// run is the sole owner of the subscriber set. It never blocks inside
// a case body: subscribe/unsubscribe are map operations, and publish
// only ever does non-blocking sends, so this loop is always available
// to service the next request promptly.
func (b *EventBus) run() {
	subscribers := make(map[chan Event]struct{})

	for {
		select {
		case req := <-b.subscribeCh:
			ch := make(chan Event, subscriberBuffer)
			subscribers[ch] = struct{}{}
			req.resp <- ch

		case req := <-b.unsubscribeCh:
			for sub := range subscribers {
				if sub == req.ch {
					delete(subscribers, sub)
					break
				}
			}
			close(req.done)

		case e := <-b.publishCh:
			for ch := range subscribers {
				select {
				case ch <- e:
				default:
				}
			}
		}
	}
}

// Subscribe registers a new subscriber and returns the channel that
// events published from now on will be delivered to.
func (b *EventBus) Subscribe() <-chan Event {
	resp := make(chan chan Event)
	b.subscribeCh <- subscribeReq{resp: resp}
	return <-resp
}

// Unsubscribe asks the owner goroutine to remove ch and waits for
// confirmation before returning, so the bus never sends to ch again
// once Unsubscribe has returned.
func (b *EventBus) Unsubscribe(ch <-chan Event) {
	done := make(chan struct{})
	b.unsubscribeCh <- unsubscribeReq{ch: ch, done: done}
	<-done
}

// Publish delivers e to every current subscriber. If a subscriber's
// buffer is full, the event is dropped for that subscriber only -
// Publish never blocks waiting on a slow or stalled subscriber.
func (b *EventBus) Publish(e Event) {
	b.publishCh <- e
}
```

A couple of details that make this work correctly rather than just look
clever:

- `Unsubscribe` doesn't just fire a request and return — it waits on `done`,
  which `run` only closes *after* it has actually deleted the entry. Without
  that handshake, `Unsubscribe` could return before the owner goroutine got
  around to processing the deletion, which would violate "the bus must no
  longer send to that channel" the instant the call returns.
- `Publish` and `Subscribe` both look like they "block" on a channel send,
  but `run`'s `select` loop is always ready to receive one of its three
  request channels — it never blocks inside a case body — so those sends
  complete promptly regardless of subscriber behavior.
- The same `sub == req.ch` comparison trick from Approach 1 is needed here
  too, for the same reason (receive-only vs. bidirectional channel types).

Trade-off versus Approach 1: this buys nothing observable — same behavior,
same guarantees — at the cost of an extra goroutine and three extra channels
per `EventBus`. It's worth knowing as a pattern (no lock to misuse, no risk of
holding it across a blocking call by accident, since the design makes that
mistake structurally harder), but for this exercise's scale, Approach 1's
mutex is simpler and does the same job.

## Key takeaways

- Never hold a lock across a potentially-blocking operation — the naive bug
  here isn't just "`Publish` can stall," it's "`Publish` can stall *while
  holding the mutex*," which freezes every other method on the type too.
- Bounded per-subscriber buffers plus a non-blocking `select`/`default` send
  is the standard way to make "broadcast to N readers" resilient to any one
  reader being slow, without penalizing the others.
- A receive-only channel (`<-chan Event`) can't be converted back to
  bidirectional, so identifying "this is the same channel I handed out
  earlier" has to go through `==` comparison (or by keying maps on the
  receive-only view) rather than a direct type conversion.
- Mutex-protected shared state and single-goroutine-owns-the-state (actor)
  designs can implement the exact same contract; picking between them is
  about which failure modes you want to make structurally impossible, not
  about one being more "correct."
