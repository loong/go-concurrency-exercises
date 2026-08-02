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

func TestMain(t *testing.T) {
	cache, db := run(t)

	cacheLen := len(cache.cache)
	pagesLen := cache.pages.Len()
	if cacheLen != CacheSize {
		t.Errorf("Incorrect cache size %v", cacheLen)
	}
	if pagesLen != CacheSize {
		t.Errorf("Incorrect pages size %v", pagesLen)
	}
	if db.Calls > callsPerCycle {
		t.Errorf("Too much db uses %v", db.Calls)
	}
}

func TestLRU(t *testing.T) {
	loader := Loader{
		DB: GetMockDB(),
	}
	cache := New(&loader)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			value := cache.Get("Test" + strconv.Itoa(i))
			if value != "Test" + strconv.Itoa(i) {
				t.Errorf("Incorrect db response %v", value)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	if len(cache.cache) != 100 {
		t.Errorf("cache not 100: %d", len(cache.cache))
	}
	cache.Get("Test0")
	cache.Get("Test101")
	if _, ok := cache.cache["Test0"]; !ok {
		t.Errorf("0 evicted incorrectly: %v", cache.cache)
	}

}
