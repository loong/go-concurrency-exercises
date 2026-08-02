# Sharded Concurrent Cache: Reducing Lock Contention — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `23-sharded-cache/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

The starting point is a `Cache` that is already race-free — one mutex protects one map — but pays for that safety with terrible contention:

```go
type Cache struct {
	mu   sync.Mutex
	data map[string]int
}

func (c *Cache) Do(key string, work func(cur int) int) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	cur := c.data[key]
	next := work(cur)
	c.data[key] = next
	return next
}
```

The lock is held for the *entire* call to `work`, including however long the caller's work function takes — so a slow operation on key `"a"` completely blocks a totally unrelated, fast operation on key `"b"` for just as long, even though the two keys never logically conflict.

The task is to reimplement `Cache` internally as `N` independent shards (e.g. `const numShards = 16`), each with its own mutex and its own sub-map, so keys landing in different shards get true parallelism while operations on the *same* key stay correctly serialized. The exercise pins down the sharding function so its own tests can rely on it:

```go
shardIndex := int(key[0]) % numShards
```

The signature must stay identical: `func NewCache() *Cache` and `func (c *Cache) Do(key string, work func(cur int) int) int`.

## Why the naive version is wrong

There's no data race here — that's the trap. The single `sync.Mutex` makes every `Do` call fully safe under `-race`, which can make the design look "done" if you only check for races. The actual bug is a **performance** one: the lock's scope is far too coarse. It's held across the call to `work`, not just across the map read/write, so two calls on keys that share nothing in common are still forced to run one after another.

Verified: running the current `check_test.go` against this naive `main.go` in a throwaway scratch copy fails exactly the contention test, and only that one:

```
--- FAIL: TestCacheDifferentKeysDontBlockEachOther (0.20s)
    check_test.go:162: cache.Do(keyB, ...) took 180.410208ms, want < 50ms - keyA and keyB
    are in different shards and must not block each other, but this call appears to have
    waited behind keyA's 200ms lock hold
FAIL
```

`TestCacheCorrectness` and `TestCacheSameKeySerializesCorrectly` both pass against the naive version — a single global mutex is trivially correct, it's just needlessly serializing.

A note on the test file itself: `TestCacheDifferentKeysDontBlockEachOther` deliberately runs on the real wall clock instead of `testing/synctest`. That's not an oversight — a plain `sync.Mutex.Lock()` (exactly what the naive `Cache` does) is not a synctest-"durable" blocking operation in Go 1.25's `testing/synctest` package. Wrapping this test in `synctest.Test` would make the fake clock advance forever without ever considering that blocked `Lock()` call "durably blocked," so it would just hang against the naive implementation instead of failing cleanly. This is a useful `synctest` limitation to know in general: it durably tracks goroutines parked on channels, `context.Done()`, `time.Sleep`, and a handful of other stdlib primitives, but a goroutine spinning inside `sync.Mutex.Lock()` contention doesn't count as one of them — so tests that specifically want to prove "this call was blocked behind a mutex" have to fall back to the real clock with a generous margin, exactly as this test does (`slowWork` at 4x the pass/fail `budget` on either side).

## Approach 1: fixed sharding by first byte

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// numShardsRef is deliberately not named numShards: check_test.go
// (which lives in the same package) already declares its own
// package-level const numShards = 16, purely so it can compute
// reference shard indices for keyA/keyB - see its referenceShard
// helper. Reusing that identifier here would be a redeclared-in-this-
// block compile error, so the solution's own shard count gets a
// distinct name instead. The *value* still has to be 16 and the
// formula still has to be int(key[0]) % N for the test's own
// pre-computed keyA/keyB shard assignment to line up with this code's.
const numShardsRef = 16

type shard struct {
	mu   sync.Mutex
	data map[string]int
}

// Cache is a key-value store sharded into numShardsRef independent
// shards, each guarded by its own mutex, so operations on keys in
// different shards can proceed with true parallelism while operations
// on the same key remain correctly serialized through that key's
// shard lock.
type Cache struct {
	shards [numShardsRef]*shard
}

// NewCache creates an empty Cache.
func NewCache() *Cache {
	c := &Cache{}
	for i := range c.shards {
		c.shards[i] = &shard{data: make(map[string]int)}
	}
	return c
}

// shardFor picks the shard responsible for key, using the key's first
// byte mod numShardsRef. Empty keys are not expected by this exercise,
// but are mapped to shard 0 rather than panicking.
func (c *Cache) shardFor(key string) *shard {
	if len(key) == 0 {
		return c.shards[0]
	}
	idx := int(key[0]) % numShardsRef
	return c.shards[idx]
}

// Do reads the current value stored at key (0 if absent), passes it
// to work, stores whatever work returns back at key, and returns that
// new value - all while holding only key's shard's lock, so
// operations on keys in other shards are never blocked by this call.
func (c *Cache) Do(key string, work func(cur int) int) int {
	s := c.shardFor(key)

	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.data[key]
	next := work(cur)
	s.data[key] = next
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
	fmt.Println("(with sharded locks, this returns almost immediately regardless of \"a\"'s slow work)")

	wg.Wait()
}
```

Design notes:

- **The shard count constant is named `numShardsRef`, not `numShards`.** `check_test.go` (same package) already declares its own package-level `const numShards = 16` purely so it can compute reference shard indices for `keyA`/`keyB` via its `referenceShard` helper. Naming the solution's own constant `numShards` too would be a `numShards redeclared in this block` compile error the moment `main.go` and `check_test.go` are compiled together - so the value (16) and the formula (`int(key[0]) % N`) both have to match the test's assumptions, but the identifier itself has to be different.
- **`shardFor` reproduces the exact formula the exercise pins down**, `int(key[0]) % 16`, and `check_test.go` relies on this precisely: it picks `keyA = "apple-key"` (first byte `'a'` = 97, `97 % 16 = 1`) and `keyB = "banana-key"` (first byte `'b'` = 98, `98 % 16 = 2`) specifically because they land in *different* shards under this formula. The test even has its own `init()` that panics at test-setup time if `keyA` and `keyB` ever computed to the same shard, so this isn't an incidental detail — any solution has to shard by first byte mod 16 (or something that happens to agree with it on these two keys) for `TestCacheDifferentKeysDontBlockEachOther` to mean what it's supposed to mean.
- **Each shard owns both its own mutex and its own sub-map** (`type shard struct { mu sync.Mutex; data map[string]int }`), never a shared map behind per-key locks. That keeps the design simple: locking shard `i` is both necessary and sufficient to safely read/write any key that hashes to shard `i`, with no cross-shard bookkeeping required.
- **`Do`'s critical section is exactly the same shape as the naive version's** — lock, read, call `work`, write, unlock — just scoped to one shard's lock instead of one global lock. The fix here is purely about **where** the lock boundary is drawn (per-shard vs. global), not about shrinking what's inside it; `work` still runs while holding a lock, which is required for the same-key serialization guarantee.
- **Fixed shard count (16) means fixed, bounded memory** — `NewCache` allocates exactly 16 mutex+map pairs up front and that's it, regardless of how many distinct keys ever pass through the cache.

**Verified**: copied the exercise into a throwaway scratch directory, confirmed the naive `main.go` fails `TestCacheDifferentKeysDontBlockEachOther` as shown above (the other two tests already passed), then dropped in this solution. `go test -race -count=1 ./...` passes, repeated 5 times in a row with no flakes — including the timing-sensitive contention test on the real clock and the 50-goroutine same-key serialization test under `-race`.

## Approach 2: per-key locks via `sync.Map` (alternative)

A genuinely different structure: instead of pre-splitting into a fixed number of shards, hand out one `*sync.Mutex` **per distinct key**, created lazily and atomically via `sync.Map.LoadOrStore`.

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// Cache is a key-value store that hands out one *sync.Mutex per
// distinct key (created lazily, atomically, via sync.Map.LoadOrStore)
// instead of pre-splitting into a fixed number of shards. Two
// different keys always get two different mutexes, so they can never
// block each other; the same key always maps to the same mutex, so
// operations on it stay correctly serialized.
type Cache struct {
	locks sync.Map // key (string) -> *sync.Mutex
	data  sync.Map // key (string) -> int
}

// NewCache creates an empty Cache.
func NewCache() *Cache {
	return &Cache{}
}

// Do reads the current value stored at key (0 if absent), passes it
// to work, stores whatever work returns back at key, and returns that
// new value - all while holding only key's own mutex, so operations
// on any other key are never blocked by this call.
func (c *Cache) Do(key string, work func(cur int) int) int {
	// sync.Map itself does NOT serialize "read, compute, write" for a
	// given key - two concurrent Do calls on the same key could both
	// Load the old value before either Store'd the new one, silently
	// losing an update. LoadOrStore is only used here to atomically
	// get-or-create that key's own *sync.Mutex; the mutex is what
	// actually provides the same-key serialization the exercise
	// requires - sync.Map on its own would not be enough.
	lockIface, _ := c.locks.LoadOrStore(key, &sync.Mutex{})
	lock := lockIface.(*sync.Mutex)

	lock.Lock()
	defer lock.Unlock()

	cur := 0
	if v, ok := c.data.Load(key); ok {
		cur = v.(int)
	}
	next := work(cur)
	c.data.Store(key, next)
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
	fmt.Println("(with per-key locks, this returns almost immediately regardless of \"a\"'s slow work)")

	wg.Wait()
}
```

Design notes and honest tradeoffs versus Approach 1:

- **`sync.Map` alone does not solve the "hold a lock during slow work" problem — the per-key mutex does, same as before.** It's tempting to reach for `sync.Map` and assume its own internal atomicity is enough, but `sync.Map` only guarantees atomicity of *individual* `Load`/`Store`/`LoadOrStore` calls, not of a read-modify-write sequence spanning two of them. Two concurrent `Do("shared-key", ...)` calls could each `Load` the same current value before either has `Store`d the updated one, silently dropping an update — exactly the bug the exercise's `TestCacheSameKeySerializesCorrectly` is designed to catch. `sync.Map`'s only real job in this solution is providing an atomic "get-or-create" for the per-key `*sync.Mutex` (via `LoadOrStore`); the actual serialization guarantee still comes entirely from that mutex, not from `sync.Map` itself.
- **This is finer-grained than 16 fixed shards — arguably *more* than the exercise asks for.** Two different keys are guaranteed distinct mutexes here, always, regardless of what their first byte happens to be. That means `TestCacheDifferentKeysDontBlockEachOther` passes for a stronger reason than under Approach 1: it's not that `keyA` and `keyB` happen to land in different shards under a mod-16 formula, it's that *any* two distinct keys never share a lock at all.
- **The real cost: unbounded, permanent memory growth.** Every distinct key that has ever been passed to `Do` gets its own `*sync.Mutex` entry in `locks`, forever — `sync.Map` has no eviction and nothing in this design ever removes an entry once created. A cache that sees millions of distinct keys over its lifetime accumulates millions of mutex entries that never get reclaimed, even if the underlying key's value is long since irrelevant. Approach 1's fixed 16-shard footprint has no such leak: memory is bounded at cache-creation time no matter how many distinct keys pass through. For a long-lived cache with a large or unbounded keyspace, this is a genuine, practical reason to prefer fixed sharding over per-key locks, even though per-key locks give strictly better concurrency in principle.
- **`sync.Map` is optimized for a different access pattern than this one shows off.** Its stated sweet spot (per its own doc comment) is keys that are mostly written once and read many times by many goroutines, or many goroutines operating on disjoint key sets — not a workload that's constantly creating new keys or churning existing ones, which is closer to what a general-purpose cache actually does. It still works correctly here; it's just not obviously the tool `sync.Map` was designed to showcase.

**Verified**: same scratch-directory protocol as Approach 1 — dropped this version into the same throwaway copy in place of the sharded-mutex version, confirmed `go build ./...` is clean, and ran `go test -race -count=1 ./...` five times in a row with no flakes, covering all three tests including the real-clock contention test and the 50-goroutine same-key test.

## Key takeaways

- A mutex that's race-free can still be a serious performance bug if its scope is too coarse. `-race` will never flag "this lock is held longer than it needs to be" — that only shows up as unnecessary blocking, which needs a timing-based test (like `TestCacheDifferentKeysDontBlockEachOther` here) to catch, not the race detector.
- Sharding by a deterministic, documented function (`int(key[0]) % numShards`) turns "different keys shouldn't block each other" into something a test can assert on precisely — pick two literal keys whose shard indices are known in advance, rather than relying on randomness or hoping for the best.
- `sync.Map`'s per-operation atomicity is not the same as serializing a read-modify-write sequence across two of its calls — using it as a top-level map replacement, by itself, does not give you back the same-key serialization a per-key or per-shard mutex provides.
- Per-key locks (via `sync.Map.LoadOrStore`) are a real, finer-grained alternative to fixed sharding, and they do solve the contention problem the same way (never holding one key's lock while operating on another's) — but they trade fixed, bounded memory for unbounded growth in the number of live mutexes, since nothing here ever evicts a key's lock once created.
- `testing/synctest` fast-forwards a *fake* clock, but it only recognizes a fixed set of stdlib operations as "durably blocked" (channels, `context.Done()`, `time.Sleep`, etc.). A goroutine contending on a plain `sync.Mutex.Lock()` isn't one of them, so a test that wants to prove "this call was blocked behind a mutex" has to use the real wall clock with a generous margin instead of `synctest` — otherwise it risks hanging forever rather than failing cleanly.
