//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// wordCountChunks returns the ~10 text chunks used across the tests.
// Several words (e.g. "fox", "the", "dog", "quick") repeat both across
// chunks and, in a couple of places, within the same chunk, so a
// correct implementation has to actually merge partial counts rather
// than just union keys together.
func wordCountChunks() []string {
	return []string{
		"the quick brown fox jumps over the lazy dog",
		"the dog barks at the fox",
		"brown fox and brown dog play",
		"quick quick quick fox",
		"lazy dog sleeps all day",
		"the cat and the dog are friends",
		"cat chases mouse mouse runs",
		"fox fox fox jumps high",
		"the sun is bright today",
		"bright sun warms the lazy cat",
	}
}

// expectedWordCountsFor10Chunks is the hand-computed word count for
// wordCountChunks(), verified by tallying each word occurrence chunk
// by chunk.
func expectedWordCountsFor10Chunks() map[string]int {
	return map[string]int{
		"all":     1,
		"and":     2,
		"are":     1,
		"at":      1,
		"barks":   1,
		"bright":  2,
		"brown":   3,
		"cat":     3,
		"chases":  1,
		"day":     1,
		"dog":     5,
		"fox":     7,
		"friends": 1,
		"high":    1,
		"is":      1,
		"jumps":   2,
		"lazy":    3,
		"mouse":   2,
		"over":    1,
		"play":    1,
		"quick":   4,
		"runs":    1,
		"sleeps":  1,
		"sun":     2,
		"the":     8,
		"today":   1,
		"warms":   1,
	}
}

// tallyWordCounts is an independent (non-WordCount) computation of
// word counts over chunks, used to check WordCount's output against
// an input set that doesn't have a hand-written expected map (e.g.
// the larger input used by the race test).
func tallyWordCounts(chunks []string) map[string]int {
	want := make(map[string]int)
	for _, chunk := range chunks {
		for _, word := range strings.Fields(chunk) {
			want[word]++
		}
	}
	return want
}

// TestWordCountCorrectness checks that WordCount produces exactly the
// expected merged word counts. It makes no assumption about how the
// chunks are processed internally, so it passes against both the
// naive sequential implementation and a map-reduce one.
func TestWordCountCorrectness(t *testing.T) {
	chunks := wordCountChunks()

	got := WordCount(chunks)
	want := expectedWordCountsFor10Chunks()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("WordCount(chunks) = %v, want %v", got, want)
	}
}

// TestWordCountConcurrency asserts that WordCount actually maps
// chunks out across multiple goroutines instead of tokenizing them
// one at a time. Ten chunks at 30ms each take 300ms sequentially; a
// map-reduce implementation finishes in roughly one chunk's worth of
// (fake) time, regardless of how many chunks there are. synctest.Test
// runs the body on a fake clock that jumps forward as soon as every
// goroutine in the bubble is durably blocked, so this assertion is
// exact and doesn't flake on a busy machine.
func TestWordCountConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		chunks := wordCountChunks()

		start := time.Now()
		got := WordCount(chunks)
		elapsed := time.Since(start)

		want := expectedWordCountsFor10Chunks()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("WordCount(chunks) = %v, want %v", got, want)
		}

		const sequentialTime = 10 * ProcessDelay
		const budget = 100 * time.Millisecond

		if elapsed >= budget {
			t.Errorf("WordCount took %s (sequential would take %s); "+
				"want well under %s - looks like chunks are being processed one at a "+
				"time instead of concurrently", elapsed, sequentialTime, budget)
		}
	})
}

// TestWordCountRace stress-tests WordCount with many chunks across
// several runs, re-checking correctness every time, to catch a
// data race on a shared result map that a naive "fix" might introduce
// by spawning one goroutine per chunk but still writing directly into
// a single shared map without a proper merge step (run with `go test
// -race`).
func TestWordCountRace(t *testing.T) {
	base := wordCountChunks()

	chunks := make([]string, 0, 3*len(base))
	for i := 0; i < 3; i++ {
		chunks = append(chunks, base...)
	}

	want := tallyWordCounts(chunks)

	for i := 0; i < 5; i++ {
		got := WordCount(chunks)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: WordCount(chunks) = %v, want %v", i, got, want)
		}
	}
}
