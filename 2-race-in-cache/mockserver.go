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

	for c := 0; c < cycles; c++ {
		wg.Add(1)
		go func() {
			for i := 0; i < callsPerCycle; i++ {

				wg.Add(1)
				go func(i int) {
					value := cache.Get("Test" + strconv.Itoa(i))
					if t != nil {
						if value != "Test"+strconv.Itoa(i) {
							t.Errorf("Incorrect db response %v", value)
						}
					}
					wg.Done()
				}(i)

			}
			wg.Done()
		}()
	}

	wg.Wait()
}
