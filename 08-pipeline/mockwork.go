package main

import "time"

// WorkLatency is how long a single call to SimulateWork takes. It
// stands in for whatever non-trivial per-item cost a real pipeline
// stage would pay (parsing, hashing, a network round trip, ...).
const WorkLatency = 10 * time.Millisecond

// SimulateWork simulates the cost of processing a single item in a
// pipeline stage by sleeping for a fixed amount of time.
func SimulateWork() {
	time.Sleep(WorkLatency)
}
