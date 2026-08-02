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

// rangeNums returns the slice of ints [1, n].
func rangeNums(n int) []int {
	nums := make([]int, n)
	for i := range nums {
		nums[i] = i + 1
	}
	return nums
}

// TestPipelineCorrectness checks that Pipeline computes the right
// answer - the square of every even-squared number from 1 to 10, in
// order. It makes no assumption about timing, so it passes against
// both the naive batch-per-stage implementation and a properly
// pipelined one.
func TestPipelineCorrectness(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	got := Pipeline(done, rangeNums(10)...)
	want := []int{4, 16, 36, 64, 100}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Pipeline(1..10) = %v, want %v", got, want)
	}
}

// TestPipelineStagesOverlap asserts that generator, square, and
// keepEven actually run concurrently, overlapping their work on
// different items, instead of each running to completion over the
// entire input before the next stage starts. With 20 numbers and
// every stage paying WorkLatency per item: a batch-per-stage
// implementation takes roughly 3 * 20 * WorkLatency (each stage waits
// for the previous one to fully finish before it can start); a truly
// pipelined implementation, where items flow through the stages as
// soon as they're ready, takes roughly one stage's total work plus a
// small constant, regardless of how many stages there are.
// synctest.Test runs the body on a fake clock that jumps forward as
// soon as every goroutine in the bubble is durably blocked, so this
// assertion is exact and doesn't flake on a busy machine.
func TestPipelineStagesOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nums := rangeNums(20)

		done := make(chan struct{})
		defer close(done)

		start := time.Now()
		got := Pipeline(done, nums...)
		elapsed := time.Since(start)

		wantLen := 0
		for _, n := range nums {
			if sq := n * n; sq%2 == 0 {
				wantLen++
			}
		}
		if len(got) != wantLen {
			t.Fatalf("Pipeline returned %d results, want %d", len(got), wantLen)
		}

		const budget = 250 * time.Millisecond
		if elapsed >= budget {
			t.Errorf("Pipeline took %s to process %d items; want well under %s - "+
				"looks like generator, square, and keepEven are running one at a "+
				"time over the whole batch instead of overlapping as a pipeline",
				elapsed, len(nums), budget)
		}
	})
}

// TestPipelineStopsEarly asserts that Pipeline stops promptly once
// its done channel is closed, instead of grinding through the entire
// (here, deliberately huge) input regardless of whether anyone still
// wants the result. A goroutine closes done shortly after Pipeline
// starts; a properly pipelined implementation that checks done on
// every send/receive abandons the in-flight work and returns quickly,
// while the naive implementation has no way to observe done at all
// and keeps processing every one of the many items to the very end.
// synctest.Test gives this a fake clock, so the huge input doesn't
// cost any real wall-clock time to run.
func TestPipelineStopsEarly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const hugeInput = 100
		nums := rangeNums(hugeInput)

		done := make(chan struct{})

		go func() {
			// Let the pipeline get going - and let at least one item
			// make it all the way through - before pulling the plug.
			time.Sleep(WorkLatency)
			close(done)
		}()

		start := time.Now()
		Pipeline(done, nums...)
		elapsed := time.Since(start)

		const budget = 200 * time.Millisecond
		if elapsed >= budget {
			t.Errorf("Pipeline took %s to return after done was closed while "+
				"processing %d items; want well under %s - looks like the "+
				"pipeline keeps working through the entire input instead of "+
				"stopping once done fires", elapsed, hugeInput, budget)
		}

		// Pipeline returns as soon as the final stage's channel
		// closes, which can happen slightly before upstream stages
		// have themselves noticed done and unwound. Give them a
		// generous amount of fake time to finish before the bubble is
		// torn down - synctest.Test requires every bubble goroutine
		// to have exited or become permanently, unresolvably blocked
		// by the time the test function returns, and a lingering
		// goroutine that's merely asleep (about to wake up and exit)
		// would otherwise be reported as a deadlock instead of a
		// clean pass. This costs no real wall-clock time.
		time.Sleep(10 * time.Second)
	})
}

// TestPipelineConcurrentUse stress-tests many independent, concurrent
// calls to Pipeline to catch data races on any shared state used
// internally by the stages (run with `go test -race`). Each call gets
// its own done channel and its own set of stage goroutines, so none
// of them should interfere with any other.
func TestPipelineConcurrentUse(t *testing.T) {
	const calls = 10
	want := []int{4, 16, 36, 64, 100}

	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			done := make(chan struct{})
			defer close(done)

			got := Pipeline(done, rangeNums(10)...)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Pipeline(1..10) = %v, want %v", got, want)
			}
		}()
	}
	wg.Wait()
}
