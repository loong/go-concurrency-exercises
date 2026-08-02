package main

import (
	"hash/fnv"
	"sync"
	"time"
)

// ComputeLatency is how long a single call to ComputeExpensive takes. It
// stands in for whatever expensive work a real computation would do
// (a cache-cold database aggregation, a heavy calculation, ...).
const ComputeLatency = 150 * time.Millisecond

// callCount, protected by callCountMu, lets tests verify how many times
// the underlying expensive computation actually ran.
var (
	callCountMu sync.Mutex
	callCount   int
)

// ComputeExpensive simulates an expensive, slow computation (e.g. a
// cache-cold database aggregation) for the given key: it sleeps for
// ComputeLatency, then returns a deterministic result derived from
// key, incrementing the shared call counter exactly once per
// invocation.
func ComputeExpensive(key string) int {
	time.Sleep(ComputeLatency)

	callCountMu.Lock()
	callCount++
	callCountMu.Unlock()

	h := fnv.New64a()
	_, _ = h.Write([]byte(key))

	return int(h.Sum64() % 1_000_000)
}

// CallCount returns how many times ComputeExpensive has run since the
// last ResetCallCount.
func CallCount() int {
	callCountMu.Lock()
	defer callCountMu.Unlock()

	return callCount
}

// ResetCallCount resets the shared call counter. It is a test-only
// helper for isolating call counts between test cases.
func ResetCallCount() {
	callCountMu.Lock()
	callCount = 0
	callCountMu.Unlock()
}
