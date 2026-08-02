//////////////////////////////////////////////////////////////////////
//
// A service often needs to shut down as soon as ANY of several
// independent triggers fires - a failed health check, an
// admin-requested shutdown, a deadline expiring, ... . Each of these
// is naturally represented as its own <-chan struct{} that gets
// closed the moment that particular condition occurs. or is supposed
// to combine an arbitrary, variadic number of such signal channels
// into a single channel that callers can select on: it must close as
// soon as ANY ONE of the input channels closes, no matter which one.
//
// Right now or ignores every channel except the first one it was
// given: it only ever waits on channels[0], so closing any channel
// OTHER than channels[0] has no effect on the returned channel at
// all - the combined channel just sits there, blocked forever, even
// though one of the shutdown triggers already fired.
//
// Your task is to fix or so the returned channel closes as soon as
// ANY of the input channels closes:
//
//   - With zero input channels there is nothing to wait on, so the
//     simplest correct behavior - and the one the naive version below
//     already gives you - is to return a channel that is never
//     closed. Don't call or() with no channels expecting it to ever
//     fire; there's nothing for it to react to.
//   - With exactly one input channel, there's nothing to combine it
//     with: just hand that single channel straight back. Resist the
//     urge to wrap it in a relay goroutine that watches it and closes
//     a new output channel in response - that relay would have no way
//     to know when to give up, so it leaks forever any time the
//     channel it's watching is never closed (which is completely
//     normal for one of several independent triggers - only one of
//     them fires, so the others simply stay open).
//   - With more than one input channel, use the classic recursive
//     divide-and-conquer idiom: watch channels[0] and channels[1]
//     directly in a select, and recurse on the rest as a third branch
//     of that same select. Whichever branch fires first - one of the
//     two directly-watched channels, or the recursive "or" of the
//     rest - closes the output.
//
//     There's a trap here: if you recurse on plain or(channels[2:]...)
//     and this level's select happens to fire because of channels[0]
//     or channels[1] (NOT the recursive branch), the goroutine that
//     recursive call already spawned is left behind forever, still
//     blocked waiting on channels further down the list that may
//     never close - a goroutine leak every time. Avoid it by folding
//     this level's own output channel into what you recurse on, e.g.
//     or(append(channels[2:], orDone)...): that way, closing orDone
//     (via the defer above) also unblocks the recursive call's
//     goroutine instead of orphaning it, and the same reasoning
//     applies at every level all the way down.
//
// The function signature must stay the same:
//
//     func or(channels ...<-chan struct{}) <-chan struct{}
//
// so that it remains a drop-in replacement for the naive version
// below.
//

package main

import (
	"fmt"
	"time"
)

// or is supposed to combine any number of done/signal channels into a
// single channel that closes as soon as ANY of the input channels
// closes - useful for combining independent shutdown triggers (e.g.
// health-check-failed, admin-requested-shutdown, deadline-exceeded)
// into one signal callers can select on. Right now it ignores every
// channel except the first one it was given, so closing any channel
// OTHER than channels[0] has no effect on the returned channel at
// all.
func or(channels ...<-chan struct{}) <-chan struct{} {
	orDone := make(chan struct{})
	if len(channels) == 0 {
		return orDone
	}
	go func() {
		defer close(orDone)
		<-channels[0]
	}()
	return orDone
}

func main() {
	healthCheckFailed := make(chan struct{})
	adminShutdown := make(chan struct{})
	deadlineExceeded := make(chan struct{})

	// Only the first trigger fires in this demo, so the program
	// completes even against the naive implementation above (which
	// only ever watches channels[0]). The naive implementation's bug
	// - ignoring every channel other than the first - only shows up
	// when a channel OTHER than channels[0] is the one that closes,
	// which is exactly what check_test.go exercises.
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(healthCheckFailed)
	}()

	start := time.Now()
	combined := or(healthCheckFailed, adminShutdown, deadlineExceeded)
	<-combined
	elapsed := time.Since(start)

	fmt.Printf("shutdown signal received after %s (triggered by healthCheckFailed)\n", elapsed)
}
