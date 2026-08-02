//////////////////////////////////////////////////////////////////////
//
// Given is a function StartMetricStream (see mockmonitor.go) that
// simulates a monitoring agent: it emits one metric reading every
// 20ms, forever, on the channel it returns. It's a long-lived feed -
// think of a metrics websocket or a tailed log - and it has no way of
// knowing when the consumer has stopped caring about new values, so
// it never closes its channel on its own.
//
// orDone below is supposed to let a consumer stop reading early (by
// closing the done channel it's given) without leaking the goroutine
// that forwards values, and without making every read site litter
// itself with a done-aware select. Right now it doesn't do any of
// that - it just hands back the input channel unchanged, so closing
// done has no effect on it at all: nothing ever stops the producer
// goroutine from trying to send a value that nobody will read again.
//
// Your task is to implement orDone properly. It must spawn its own
// goroutine that ranges over c, forwarding each value onto a new
// output channel it returns. Both operations that goroutine performs
// - receiving the next value from c, and sending the forwarded value
// on the output channel - can block forever if nobody's on the other
// end any more, so each of them must be a select that also watches
// done. As soon as done is closed, the goroutine must return promptly
// (closing the output channel behind it) instead of blocking forever
// on a send nobody will read or a receive that may never come. The
// function signature must stay the same:
//
//     func orDone(done <-chan struct{}, c <-chan int) <-chan int
//
// so that it remains a drop-in replacement for the passthrough
// version below.
//

package main

import (
	"fmt"
	"time"
)

// orDone wraps c so that ranging over the returned channel stops as
// soon as done is closed, instead of blocking forever waiting for a
// value from c that may never come (or waiting to send a value that
// nobody's reading any more). Right now it's a no-op: it just returns
// c, so done has no effect on it whatsoever.
func orDone(done <-chan struct{}, c <-chan int) <-chan int {
	return c
}

func main() {
	done := make(chan struct{})
	stream := StartMetricStream()
	out := orDone(done, stream)

	count := 0
	for v := range out {
		fmt.Println("metric:", v)
		count++
		if count == 5 {
			close(done)
			break
		}
	}
	fmt.Println("done reading, done closed - checking whether orDone actually stopped...")

	// If orDone worked, closing done should make out close promptly,
	// so this receive should come back immediately with ok == false.
	// Against the no-op passthrough, out IS stream: nothing closes it,
	// and the goroutine inside StartMetricStream is still out there
	// trying to send the next tick on a channel nobody drains anymore.
	select {
	case v, ok := <-out:
		if ok {
			fmt.Println("unexpected: got a value after done closed:", v)
		} else {
			fmt.Println("OK: out closed promptly after done was closed - no leak")
		}
	case <-time.After(200 * time.Millisecond):
		fmt.Println("BUG: out did not close within 200ms of done closing - orDone leaked the forwarding goroutine")
	}
}
