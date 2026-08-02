package main

import "time"

// SensorInterval is how long StartSensor waits between emitting two
// consecutive readings.
const SensorInterval = 5 * time.Millisecond

// StartSensor simulates a hardware sensor: it emits `count` incrementing
// integer readings (0, 1, 2, ...), one every SensorInterval, on the
// channel it returns, then closes that channel. Think of a temperature
// probe or an accelerometer streaming samples that need to reach more
// than one consumer - a live display AND a logger, say.
func StartSensor(count int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for i := 0; i < count; i++ {
			time.Sleep(SensorInterval)
			out <- i
		}
	}()

	return out
}
