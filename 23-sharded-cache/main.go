//////////////////////////////////////////////////////////////////////
//
// Given is a key-value cache that is already correct - a single mutex
// protects a single map, so there is no data race no matter how many
// goroutines call it concurrently. But it has a real performance
// problem: the cache's ONE mutex is held for the entire duration of
// every call to Do, including however long the caller's work function
// takes to run. That means an operation on key "a" that happens to be
// slow completely blocks an operation on a totally unrelated key "b"
// for just as long, even though "a" and "b" don't logically conflict
// at all - there is no reason for that serialization other than this
// cache's overly coarse locking.
//
// Your task is to reimplement Cache internally as N independent
// shards (e.g. const numShards = 16), each with its own mutex and its
// own sub-map, so operations on keys that land in different shards
// can proceed with true parallelism, while operations on the SAME key
// remain correctly serialized through that key's shard lock. Use a
// simple, deterministic sharding function to pick a key's shard - for
// concreteness (and so this exercise's tests can rely on it), shard by
// the key's first byte:
//
//     shardIndex := int(key[0]) % numShards
//
// Keep the signature identical, so Cache remains a drop-in
// replacement:
//
//     func NewCache() *Cache
//     func (c *Cache) Do(key string, work func(cur int) int) int
//

package main

import (
	"fmt"
	"sync"
	"time"
)

// Cache is a key-value store that is supposed to let operations on
// DIFFERENT keys proceed truly concurrently (since they don't
// logically conflict), while still safely serializing operations on
// the SAME key. Right now it uses a single global mutex held for the
// entire duration of Do's call to work, which means an operation on
// key "a" that happens to be slow completely blocks an operation on a
// totally unrelated key "b" for just as long.
type Cache struct {
	mu   sync.Mutex
	data map[string]int
}

// NewCache creates an empty Cache.
func NewCache() *Cache {
	return &Cache{data: make(map[string]int)}
}

// Do reads the current value stored at key (0 if absent), passes it
// to work, stores whatever work returns back at key, and returns that
// new value - all while holding the cache's lock for the full
// duration of the call to work, so work can safely assume no
// concurrent access to key happens while it runs.
func (c *Cache) Do(key string, work func(cur int) int) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	cur := c.data[key]
	next := work(cur)
	c.data[key] = next
	return next
}

func main() {
	cache := NewCache()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		cache.Do("a", func(cur int) int {
			time.Sleep(200 * time.Millisecond)
			return cur + 1
		})
	}()

	time.Sleep(10 * time.Millisecond)

	start := time.Now()
	cache.Do("b", func(cur int) int { return cur + 1 })
	elapsed := time.Since(start)

	fmt.Printf("Do(\"b\", ...) took %s\n", elapsed)
	fmt.Println("(with a single global mutex, this is forced to wait ~200ms for the unrelated, slow \"a\" operation)")

	wg.Wait()
}
