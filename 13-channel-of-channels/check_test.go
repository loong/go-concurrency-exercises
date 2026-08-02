//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"fmt"
	"testing"
	"time"
)

// TestBridgeFlattensAllShards makes sure Bridge doesn't stop at the
// first inner channel it receives from chanStream: it must keep
// reading chanStream and flatten every shard it ever produces into the
// single output stream, only closing that output once chanStream
// itself is closed and every shard has been fully drained.
//
// Against the naive Bridge in main.go, only the first shard's lines
// (4 of the 20 total) ever make it out, so this test fails there.
//
// This runs on the real clock rather than under testing/synctest: a
// naive/broken Bridge abandons chanStream after its first shard, which
// leaves the hub's producer goroutines permanently blocked trying to
// send shards/lines nobody will ever read again - exactly the bug this
// test is meant to expose. Inside a synctest bubble that permanent
// block is treated as a fatal deadlock the moment the test function
// returns, which would crash the whole test binary instead of
// reporting a normal, readable test failure. The assertions below are
// still fully deterministic (total count and exact set of lines seen);
// the 2s timeout is only a safety net against the test hanging forever
// if Bridge never closes its output at all.
func TestBridgeFlattensAllShards(t *testing.T) {
	const numShards = 5
	const linesPerShard = 4

	done := make(chan struct{})
	defer close(done)

	shardStream := StartLogHub(numShards, linesPerShard)
	logs := Bridge(shardStream, done)

	got := make(map[string]int)
	timeout := time.After(2 * time.Second)

collect:
	for {
		select {
		case line, ok := <-logs:
			if !ok {
				break collect
			}
			got[line]++
		case <-timeout:
			t.Fatal("timed out waiting for Bridge to flatten all shards")
		}
	}

	total := 0
	for _, n := range got {
		total += n
	}

	if total != numShards*linesPerShard {
		t.Fatalf("got %d total log lines, want %d (only shards seen: %d unique lines)", total, numShards*linesPerShard, len(got))
	}

	for shardIdx := 0; shardIdx < numShards; shardIdx++ {
		for i := 0; i < linesPerShard; i++ {
			want := fmt.Sprintf("shard-%d-line-%d", shardIdx, i)
			if got[want] != 1 {
				t.Errorf("expected exactly one occurrence of %q, got %d", want, got[want])
			}
		}
	}
}

// TestBridgeStopsOnDone proves that Bridge reacts to done being closed
// instead of hanging around (or leaking goroutines) until every shard
// naturally finishes. It starts a hub with far more shards/lines than
// it will ever consume, reads a few values, closes done, and then
// expects the output channel to close promptly.
//
// This uses the real clock (not synctest) since it's a "does it stop
// quickly" check rather than a precise-duration one, but the bound is
// generous enough to never be flaky while keeping the test fast.
func TestBridgeStopsOnDone(t *testing.T) {
	done := make(chan struct{})

	shardStream := StartLogHub(50, 50)
	logs := Bridge(shardStream, done)

	for i := 0; i < 3; i++ {
		select {
		case _, ok := <-logs:
			if !ok {
				t.Fatal("output channel closed before we could read the first few values")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out reading initial values from Bridge output")
		}
	}

	close(done)

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-logs:
			if !ok {
				return // output channel closed promptly after done - success.
			}
			// Drain any values already in flight and keep waiting for close.
		case <-deadline:
			t.Fatal("Bridge did not close its output channel promptly after done was closed")
		}
	}
}
