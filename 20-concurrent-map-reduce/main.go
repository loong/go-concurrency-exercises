//////////////////////////////////////////////////////////////////////
//
// WordCount is supposed to count word occurrences across many text
// chunks, using the map-reduce pattern: a "map" phase that processes
// each chunk independently and concurrently into its own partial
// count, and a "reduce" phase that merges all the partials into one
// final result - all without multiple goroutines ever writing to the
// same shared map at the same time (which would race). Right now it
// processes chunks sequentially, one at a time, merging each chunk's
// words directly into a single shared result map as it goes - which
// is correct, but leaves all the chunk-level parallelism on the
// table, since chunk 2 can't even start being tokenized until chunk
// 1's tokenizing (simulated by time.Sleep(ProcessDelay) per chunk) has
// fully finished.
//
// Your task is to reimplement WordCount as true map-reduce: spawn one
// goroutine per chunk (the "map" phase) that tokenizes its OWN chunk
// into its OWN local map[string]int (after time.Sleep(ProcessDelay),
// simulating the per-chunk cost), with no shared mutable state
// touched during this phase. Then, once all map-phase goroutines have
// finished (a sync.WaitGroup, or fanning the partials in over a
// channel of maps, both work), a single "reduce" step merges every
// partial map into one final result sequentially, on one goroutine,
// so no concurrent writes to the final map are ever needed. The
// function signature must stay the same:
//
//     func WordCount(chunks []string) map[string]int
//
// so that it remains a drop-in replacement for the sequential version
// below.
//

package main

import (
	"fmt"
	"strings"
	"time"
)

// WordCount counts word occurrences across all chunks. It currently
// does so sequentially, one chunk at a time.
func WordCount(chunks []string) map[string]int {
	result := make(map[string]int)

	for _, chunk := range chunks {
		time.Sleep(ProcessDelay)
		for _, word := range strings.Fields(chunk) {
			result[word]++
		}
	}

	return result
}

func main() {
	chunks := []string{
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

	start := time.Now()
	counts := WordCount(chunks)
	elapsed := time.Since(start)

	for word, count := range counts {
		fmt.Printf("%s: %d\n", word, count)
	}

	fmt.Printf("Counted %d distinct words across %d chunks in %s\n", len(counts), len(chunks), elapsed)
}
