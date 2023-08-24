package main

import (
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	// minimum time it takes to read and process a tweet (320 reading + 320 processing)
	minProcessingTime := 640 * time.Millisecond
	sequentialProcessingTime := time.Duration(len(mockdata)) * minProcessingTime
	// Enabling concurrency should at least reduce processing time by a quarter.
	maximumAcceptedTotalProcessingTime := (sequentialProcessingTime / 4) * 3
	start := time.Now()
	main()
	elapsedTime := time.Since(start)

	if maximumAcceptedTotalProcessingTime < elapsedTime {
		t.Logf("The recorded duration indicates that the tweets were not processed concurrently.")
		t.Logf("Maximum accepted duraction (%s) recorded duration (%s)", maximumAcceptedTotalProcessingTime, elapsedTime)
		t.Log("Solution is incorrect.")
		t.FailNow()
	}
}
