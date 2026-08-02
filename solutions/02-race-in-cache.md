# Race Condition in Caching Scenario — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `02-race-in-cache/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`KeyStoreCache` is an LRU cache backed by a `map[string]*list.Element` (for O(1) lookup) plus a `container/list.List` (to track recency order). `Get` does:

1. Look up `key` in `cache`. On a hit, move the element to the front of `pages` (most-recently-used) and return its value.
2. On a miss, load the value from the database, evict the least-recently-used entry if the cache is full (`CacheSize == 100`), then push the new entry onto both `pages` and `cache`.

The test harness (`mockserver.go`) hammers this with `15 * 100 = 1500` concurrent goroutines, requesting only 100 distinct keys (`"Test0"` … `"Test99"`) spread across the 15 cycles. Since `CacheSize == 100` equals the number of distinct keys, a correct solution should never evict anything in `TestMain`, and — per `check_test.go` — should load each key from the database **at most once** (`db.Calls` must not exceed `callsPerCycle == 100`).

## Why the naive version is wrong

There is no synchronization at all around `cache` or `pages`, and both are mutated concurrently:

- **The map.** One goroutine's `cache[key] = ...` write races with another goroutine's `cache[key]` read (a plain map read concurrent with a write is undefined behavior in Go, not just "unsafe on write vs write"). `go test -race` reports this directly:

  ```
  WARNING: DATA RACE
  Write at 0x00c0000ba600 by goroutine 88:
    runtime.mapaccess2_faststr()
    verify2.(*KeyStoreCache).Get()
        main.go:62   (the `if e, ok := k.cache[key]; ok` on a miss path being written concurrently)

  Previous read at 0x00c0000ba600 by goroutine 1455:
    runtime.mapaccess1_faststr()
    verify2.(*KeyStoreCache).Get()
        main.go:47
  ```

- **The list.** This is the easy-to-miss half of the bug: `container/list.List` is a plain doubly linked list with no internal locking, and `MoveToFront` **mutates** the list even though it looks like it belongs on the "read/hit" path. Two goroutines both hitting the cache concurrently and calling `MoveToFront` race on the same list nodes; `PushFront` on a miss races with any concurrent `MoveToFront`/`PushFront`/`Remove`. `-race` catches this too:

  ```
  WARNING: DATA RACE
  Read at 0x00c0000ae988 by goroutine 216:
    container/list.(*List).lazyInit()
    container/list.(*List).PushFront()
    verify2.(*KeyStoreCache).Get()
        main.go:61
  ```

  This second race is why an `RWMutex` that only takes an `RLock` on the "hit" path is **not** a valid fix: the hit path still writes to the list via `MoveToFront`. It's also why swapping `cache` for a `sync.Map` doesn't rescue you — that only makes the map safe; the LRU ordering still lives in a `container/list.List` that needs its own mutual exclusion regardless of which map type backs it.

There's a second, more subtle correctness requirement hiding in `check_test.go`: `db.Calls` must not exceed 100. Naively narrowing the critical section to just the map/list touches — i.e. releasing the lock while `k.load(key)` runs — keeps `-race` quiet (map and list are always touched under the lock) but breaks correctness in a different way: many goroutines can simultaneously miss on the same key before any of them finishes loading it, so each issues its own DB call. Verified empirically against this exact test suite: that variant is race-free but consistently fails with `db.Calls == 1500` (every single call reloads from the DB) and `pages.Len()` in the 1400s against a `cache` map size of 99 — the duplicate concurrent misses push multiple list elements for the same key while the map keeps only the last one, so the two data structures drift out of sync in count as well. The takeaway: whatever synchronization you use, the "check cache, and if missing, load + insert" sequence for a given key must be atomic end-to-end, not just the map/list bookkeeping around it.

## Approach 1: single mutex around the whole critical section

The simplest correct fix: one `sync.Mutex` guarding the entire body of `Get`, held for the full duration — including the call to `k.load(key)`. Holding the lock through the load is what prevents two goroutines from both missing on the same key and double-loading it (the `db.Calls` requirement above), at the cost of fully serializing all cache access.

```go
package main

import (
	"container/list"
	"sync"
)

// CacheSize determines how big the cache can grow
const CacheSize = 100

type KeyStoreCacheLoader interface {
	Load(string) string
}

type page struct {
	Key   string
	Value string
}

// KeyStoreCache is a LRU cache for string key-value pairs
type KeyStoreCache struct {
	mu    sync.Mutex
	cache map[string]*list.Element
	pages list.List
	load  func(string) string
}

// New creates a new KeyStoreCache
func New(load KeyStoreCacheLoader) *KeyStoreCache {
	return &KeyStoreCache{
		load:  load.Load,
		cache: make(map[string]*list.Element),
	}
}

// Get gets the key from cache, loads it from the source if needed
func (k *KeyStoreCache) Get(key string) string {
	k.mu.Lock()
	defer k.mu.Unlock()

	if e, ok := k.cache[key]; ok {
		k.pages.MoveToFront(e)
		return e.Value.(page).Value
	}
	// Miss - load from database and save it in cache
	p := page{key, k.load(key)}
	// if cache is full remove the least used item
	if len(k.cache) >= CacheSize {
		end := k.pages.Back()
		delete(k.cache, end.Value.(page).Key)
		k.pages.Remove(end)
	}
	k.pages.PushFront(p)
	k.cache[key] = k.pages.Front()
	return p.Value
}
```

**Verified**: `go vet ./...` clean; `go test -race -count=5 ./...` passes 5/5 with no races and correct assertions (`TestMain`, `TestLRU`).

Cost model: because the lock is held across `k.load`, every cache *miss* is fully serialized against every other miss (and every hit). Total time is roughly `(number of distinct keys) × (DB latency)`, independent of how much concurrency the caller throws at it. In this exercise that's `100 keys × 20ms ≈ 2s` per `TestMain` run — measured at ~2.10–2.13s under `-race` on go1.25.6/darwin-arm64, so ~4.2s total across `TestMain` + `TestLRU`. That comfortably clears the README's "under 30 seconds" goal and just barely clears "under 5 seconds," but the margin is thin and the approach doesn't scale: with more distinct keys or a slower backing store, this serialization becomes the bottleneck. That's exactly the gap Approach 2 closes.

## Approach 2: per-key load deduplication ("singleflight"), lock released during the DB call

A meaningfully different design: keep the mutex narrow (only around map/list bookkeeping) and release it while the actual DB load runs, so misses on *different* keys can be loaded concurrently. To still satisfy the "at most one DB call per key" requirement, track in-flight loads per key: if a goroutine misses on a key that's already being loaded by someone else, it waits on that load instead of starting a redundant one (the same idea as `golang.org/x/sync/singleflight`, written out by hand here).

```go
package main

import (
	"container/list"
	"sync"
)

// CacheSize determines how big the cache can grow
const CacheSize = 100

type KeyStoreCacheLoader interface {
	Load(string) string
}

type page struct {
	Key   string
	Value string
}

// call represents an in-flight (or just-completed) load for a single key,
// so concurrent misses on the same key share one DB call instead of each
// triggering its own ("thundering herd" / dogpile problem).
type call struct {
	done  chan struct{}
	value string
}

// KeyStoreCache is a LRU cache for string key-value pairs
type KeyStoreCache struct {
	mu      sync.Mutex
	cache   map[string]*list.Element
	pages   list.List
	load    func(string) string
	loading map[string]*call
}

// New creates a new KeyStoreCache
func New(load KeyStoreCacheLoader) *KeyStoreCache {
	return &KeyStoreCache{
		load:    load.Load,
		cache:   make(map[string]*list.Element),
		loading: make(map[string]*call),
	}
}

// Get gets the key from cache, loads it from the source if needed
func (k *KeyStoreCache) Get(key string) string {
	k.mu.Lock()
	if e, ok := k.cache[key]; ok {
		k.pages.MoveToFront(e)
		v := e.Value.(page).Value
		k.mu.Unlock()
		return v
	}
	if c, ok := k.loading[key]; ok {
		// Someone else is already loading this key - wait for them
		// instead of issuing a redundant DB call.
		k.mu.Unlock()
		<-c.done
		return c.value
	}
	c := &call{done: make(chan struct{})}
	k.loading[key] = c
	k.mu.Unlock()

	// Load from the database without holding the lock, so misses on
	// other keys (and cache hits) can proceed concurrently.
	value := k.load(key)

	k.mu.Lock()
	delete(k.loading, key)
	if len(k.cache) >= CacheSize {
		end := k.pages.Back()
		delete(k.cache, end.Value.(page).Key)
		k.pages.Remove(end)
	}
	k.pages.PushFront(page{key, value})
	k.cache[key] = k.pages.Front()
	k.mu.Unlock()

	c.value = value
	close(c.done)
	return value
}
```

**Verified**: `go vet ./...` clean; `go test -race -count=5 ./...` passes 5/5 with no races and correct assertions. Measured `TestMain` time: ~0.02–0.03s (vs ~2.1s for Approach 1) under the same conditions, because the 100 distinct-key loads now happen concurrently instead of serially — this is what actually satisfies the README's stronger hint ("fetching from the database takes the longest... get your solution down to less than 5 seconds").

This is more code for a genuine payoff: throughput no longer degrades linearly with the number of distinct keys. For a cache this small and a test suite this short either approach passes; Approach 2 is the one that scales.

## Key takeaways

- `go test -race` will flag the map access, but don't stop there — `container/list.List` is just as unsynchronized, and `MoveToFront` on the "hit" path is a write, not a read. An `RWMutex` with `RLock()` on hits is a common but incorrect "fix" for exactly this reason.
- `sync.Map` doesn't fit this problem: it would make the map safe, but the LRU recency ordering lives in `container/list.List`, which still needs a mutex (or equivalent) regardless of the map implementation.
- Correctness here isn't just "no race" — `check_test.go` also asserts `db.Calls <= callsPerCycle`. A version that only locks the map/list touches and reloads outside the lock is race-free but was verified to blow this assertion badly (`db.Calls == 1500` instead of ≤100, and `cache`/`pages` sizes diverging), because concurrent misses on the same key each triggers its own DB load.
- Approach 1 (hold the lock through the load) is the simplest correct fix and is plenty fast for this exercise's scale, but it serializes all misses — cost scales with `distinct keys × DB latency` regardless of available concurrency.
- Approach 2 (per-key in-flight dedup, a hand-rolled `singleflight`) keeps correctness while letting independent keys load in parallel, trading a bit of extra bookkeeping for throughput that scales with concurrency instead of against it.
