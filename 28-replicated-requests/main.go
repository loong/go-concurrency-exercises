//////////////////////////////////////////////////////////////////////
//
// Given is a FetchFastest that is supposed to send the same request
// to several redundant replicas concurrently (see mockreplica.go) and
// return whichever one answers first, so that no single replica's
// unpredictable tail latency can slow the caller down. It already
// does that much correctly: it fans out to every replica and returns
// the first winner's value, so a test that only checks "does it
// return the right value" passes against it as-is.
//
// The bug is what happens to the replicas that lose the race. Right
// now every replica goroutine is started with a nil done channel, so
// none of them ever has any way to learn that FetchFastest already
// has its answer and has moved on. Every losing replica just keeps
// running - sleeping out its full artificial latency and doing
// whatever work it was doing - long after the caller stopped caring,
// wasting work and leaking a goroutine per call until it eventually
// finishes on its own.
//
// Your task is to fix FetchFastest so that as soon as a winner is
// picked (or the caller-supplied done is closed), every other
// in-flight replica is told to stop via its own done channel, and
// actually reacts to it instead of running to completion regardless.
//
// The signatures must stay the same:
//
//   type Replica func(done <-chan struct{}) (string, error)
//   func FetchFastest(done <-chan struct{}, replicas ...Replica) (string, error)
//

package main

import (
	"errors"
	"fmt"
	"time"
)

// Replica represents one redundant handler that can serve a request.
// A well-behaved Replica must return promptly once done is closed,
// instead of continuing to run to completion regardless.
type Replica func(done <-chan struct{}) (string, error)

// replicaResult carries one replica's outcome back to FetchFastest.
type replicaResult struct {
	value string
	err   error
}

// FetchFastest calls every replica concurrently (one goroutine each)
// and returns the value and error from whichever one sends on its own
// result channel FIRST - later stragglers are ignored. If done is
// closed before any replica has responded, FetchFastest returns early
// with an error and no winner.
func FetchFastest(done <-chan struct{}, replicas ...Replica) (string, error) {
	if len(replicas) == 0 {
		return "", errors.New("fetchfastest: no replicas provided")
	}

	results := make(chan replicaResult, len(replicas))

	for _, replica := range replicas {
		go func(r Replica) {
			// BUG: every replica is handed a nil done, so none of
			// them can ever learn that FetchFastest already picked a
			// winner (or that the caller cancelled). A losing replica
			// just keeps running for its full artificial latency
			// instead of stopping as soon as it lost the race.
			value, err := r(nil)
			results <- replicaResult{value: value, err: err}
		}(replica)
	}

	select {
	case res := <-results:
		return res.value, res.err
	case <-done:
		return "", errors.New("fetchfastest: cancelled before any replica responded")
	}
}

func main() {
	replicas := []Replica{
		NewMockReplica("replica-A", 150*time.Millisecond).Replica,
		NewMockReplica("replica-B", 10*time.Millisecond).Replica,
		NewMockReplica("replica-C", 300*time.Millisecond).Replica,
	}

	done := make(chan struct{})
	start := time.Now()

	value, err := FetchFastest(done, replicas...)

	fmt.Printf("winner after %s: value=%q err=%v\n", time.Since(start), value, err)
	fmt.Println("...but replica-A and replica-C are still out there, running to completion")
	fmt.Println("in the background, even though nobody is listening for their result anymore.")

	// Give the losing replicas time to (not) notice they lost, just so
	// this demo doesn't exit before you can see them still ticking
	// away in a goroutine dump (e.g. via a SIGQUIT or pprof).
	time.Sleep(400 * time.Millisecond)
}
