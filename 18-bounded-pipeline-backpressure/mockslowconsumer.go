package main

import "time"

// ConsumeLatency is how long a single call to SlowConsume takes. It
// stands in for whatever expensive per-item cost a real downstream
// sink would pay (writing to a remote store, a slow disk, a
// rate-limited API, ...).
const ConsumeLatency = 50 * time.Millisecond

// SlowConsume simulates a slow downstream sink (e.g. writing to a
// remote store) - it takes 50ms to consume a single item.
func SlowConsume(item int) {
	time.Sleep(ConsumeLatency)
}
