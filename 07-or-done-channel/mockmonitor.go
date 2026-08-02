package main

import "time"

// MetricInterval is how often StartMetricStream emits a new reading.
const MetricInterval = 20 * time.Millisecond

// StartMetricStream simulates a monitoring agent emitting one metric
// reading every MetricInterval, forever, until the caller stops
// reading. It spawns a goroutine that loops indefinitely - sleep,
// then send an incrementing counter value on the returned channel -
// and it never closes that channel on its own. This is how a real
// long-lived feed (a metrics websocket, a tailed log, a sensor
// stream) behaves: it has no way to know the consumer stopped
// listening, so it's on the consumer to stop reading cleanly.
func StartMetricStream() <-chan int {
	out := make(chan int)

	go func() {
		counter := 0
		for {
			time.Sleep(MetricInterval)
			counter++
			out <- counter
		}
	}()

	return out
}
