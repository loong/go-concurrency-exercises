//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"strconv"
	"sync"
	"testing"
)

const (
	cycles        = 15
	callsPerCycle = 100
)

// RunMockServer simulates a running server, which accesses the
// key-value database through our cache
func RunMockServer(cache *KeyStoreCache, t *testing.T) {
	var wg sync.WaitGroup

	for range cycles {
		wg.Go(func() {
			for i := range callsPerCycle {
				wg.Go(func() {
					value := cache.Get("Test" + strconv.Itoa(i))
					if t != nil {
						if value != "Test"+strconv.Itoa(i) {
							t.Errorf("Incorrect db response %v", value)
						}
					}
				})
			}
		})
	}

	wg.Wait()
}
