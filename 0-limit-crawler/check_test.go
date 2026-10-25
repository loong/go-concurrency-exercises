//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	fetchSig := fetchSignalInstance()

	// tooFast is signalled to the test goroutine when two crawls are
	// executed less than a second apart.
	tooFast := make(chan struct{}, 1)
	start := time.Unix(0, 0)
	go func(start time.Time) {
		for range fetchSig {
			// Check if signal arrived earlier than a second (with error margin)
			if time.Since(start) < 950*time.Millisecond {
				tooFast <- struct{}{}
				return
			}
			start = time.Now()
		}
	}(start)

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	select {
	case <-tooFast:
		t.Log("There exists a two crawls that were executed less than 1 second apart.")
		t.Log("Solution is incorrect.")
		t.Fail()
		// Let the crawler finish so its output is not cut off.
		<-done
	case <-done:
	}
}
