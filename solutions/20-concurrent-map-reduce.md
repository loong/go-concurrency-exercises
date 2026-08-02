# Concurrent Map-Reduce: Parallel Word Count — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `20-concurrent-map-reduce/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`WordCount` is supposed to count word occurrences across many text
chunks using the map-reduce pattern: a "map" phase that tokenizes each
chunk independently and concurrently, and a "reduce" phase that merges
all the partial counts into one final result — without ever having two
goroutines write to the same shared map at the same time.

The given implementation processes chunks strictly sequentially,
merging each chunk's words directly into a single shared `result` map
as it goes:

```go
func WordCount(chunks []string) map[string]int {
	result := make(map[string]int)

	for _, chunk := range chunks {
		time.Sleep(ProcessDelay)
		for _, word := range strings.Fields(chunk) {
			result[word]++
		}
	}

	return result
}
```

This is correct — there's only ever one goroutine touching `result` —
but chunk 2 can't even start tokenizing until chunk 1's
`time.Sleep(ProcessDelay)` has fully elapsed, so ten chunks cost ten
chunks' worth of wall-clock time, even though tokenizing one chunk has
nothing to do with tokenizing any other.

## Why the naive version is wrong

Again, "wrong" here is a throughput bug, not a correctness bug. The
sequential loop passes `TestWordCountCorrectness` every time — it never
drops a word, never double-counts, and (being single-threaded) can't
race on `result`.

What it fails is `TestWordCountConcurrency`, which uses `synctest.Test`
to run the call on a fake clock and asserts the elapsed time stays well
under 100ms. Ten chunks at 30ms each sequentially costs exactly 300ms —
and that's exactly what the test observes:

```
--- FAIL: TestWordCountConcurrency (0.00s)
    check_test.go:126: WordCount took 300ms (sequential would take 300ms); want
    well under 100ms - looks like chunks are being processed one at a time
    instead of concurrently
```

This is the right test tool for this exercise: the naive version never
gets stuck, it's just slow, so a fake clock that fast-forwards once
every goroutine is durably blocked can measure it exactly, with no
flakiness on a busy machine. (Contrast this with exercise 21, where the
naive bug is a genuine deadlock rather than mere slowness — `synctest`
would be the *wrong* tool there, for reasons explained in that
exercise's solution doc.)

## Approach 1: Goroutine-per-chunk into local maps, merge once (map-reduce, as specified)

This is the design the exercise text asks for, verbatim: each chunk
gets its own goroutine that tokenizes into its *own* local
`map[string]int`, touching no shared mutable state during the map
phase. Only after every goroutine has finished does a single,
sequential reduce step fold the partials together.

```go
func WordCount(chunks []string) map[string]int {
	partials := make([]map[string]int, len(chunks))

	var wg sync.WaitGroup
	wg.Add(len(chunks))

	for i, chunk := range chunks {
		i, chunk := i, chunk
		go func() {
			defer wg.Done()

			time.Sleep(ProcessDelay)

			local := make(map[string]int)
			for _, word := range strings.Fields(chunk) {
				local[word]++
			}
			partials[i] = local
		}()
	}

	wg.Wait()

	result := make(map[string]int)
	for _, partial := range partials {
		for word, count := range partial {
			result[word] += count
		}
	}

	return result
}
```

Walking through why this is race-free:

- **Map phase**: every goroutine owns exactly one slot,
  `partials[i]`, that no other goroutine ever touches, and it builds
  its own brand-new `local` map that no other goroutine even has a
  reference to. Assigning `partials[i] = local` is the only shared
  write, and each `i` is unique per goroutine, so there's no
  overlapping access — writing to distinct slice indices concurrently
  is safe in Go.
- **Reduce phase**: `wg.Wait()` establishes a happens-before edge
  between every map-phase goroutine's completion and the reduce loop,
  so by the time the reduce loop runs, all `partials` entries are
  fully populated and visible. The reduce loop itself is single-
  threaded, so merging into `result` needs no locking at all.
- Fanning the partials in over a `chan map[string]int` instead of a
  pre-sized `[]map[string]int` + `sync.WaitGroup` works identically
  well and is mentioned as an equally valid option in the exercise
  text — the essential invariant is the same either way: no map is
  ever written by more than one goroutine.

This passed `go test -race -count=3` in a scratch copy with no
flakiness: `TestWordCountCorrectness`, `TestWordCountConcurrency`, and
`TestWordCountRace` (which reruns `WordCount` five times over 30
chunks) all passed in ~0.03s–0.16s per run, well under the 100ms
concurrency budget and with the race detector silent throughout.

## Common mistake to avoid: goroutine-per-chunk writing into ONE shared map

A very natural-looking "fix" is to spawn one goroutine per chunk (so
`TestWordCountConcurrency` passes) but skip the local-map step
entirely, having every goroutine increment the *same* shared `result`
map directly:

```go
func WordCount(chunks []string) map[string]int {
	result := make(map[string]int)

	var wg sync.WaitGroup
	wg.Add(len(chunks))

	for _, chunk := range chunks {
		chunk := chunk
		go func() {
			defer wg.Done()
			time.Sleep(ProcessDelay)
			for _, word := range strings.Fields(chunk) {
				result[word]++ // shared map, no lock - racy
			}
		}()
	}

	wg.Wait()
	return result
}
```

This looks like it should work — and on a small input, over a handful
of runs, it might even *appear* to work, because `result[word]++` is
"only" a read-modify-write and Go's map implementation doesn't always
corrupt itself just because two goroutines hit it around the same
time. That's exactly the trap: this code has a real, undefined-
behavior data race regardless of whether any given run happens to show
a symptom.

Verifying this in a scratch copy confirms both halves of that claim.
Running `go test -run TestWordCountRace -count=1` **without** `-race`
eight times in a row against this exact code passed every single time
— no crash, no wrong count, nothing to see. Running it **with**
`-race` fails immediately and reliably, first with data-race warnings
and then a fatal runtime error that kills the whole test binary:

```
==================
WARNING: DATA RACE
Read at 0x00c0001806c0 by goroutine 36:
  runtime.mapdelete_fast64()
      .../internal/runtime/maps/runtime_fast64_swiss.go:502 +0x8c
  verify20.WordCount.func1()
      main.go:33 +0xe0

Previous write at 0x00c0001806c0 by goroutine 14:
  runtime.mapaccess2_faststr()
      .../internal/runtime/maps/runtime_faststr_swiss.go:162 +0x29c
  verify20.WordCount.func1()
      main.go:33 +0x10c
==================
fatal error: concurrent map writes
```

The `fatal error: concurrent map writes` is Go's *runtime* map-
corruption guard, not the race detector — it fires when the map's
internal bookkeeping is caught in an inconsistent state by two
concurrent writers, independent of `-race`. That guard is probabilistic
by nature: it depends on unlucky-enough interleaving to actually
observe corrupted internal state, which is why plain `go test` (no
`-race`) can run this exact racy code all day and never trip it, while
`-race`'s instrumentation makes the underlying race itself deterministic
to detect (it tracks every memory access with happens-before
information, so it doesn't need the corruption to *manifest* — it
just needs two unsynchronized accesses to the same memory, one of
which is a write, and it did no such thing sporadically here: every
`-race` run failed). This is precisely why `TestWordCountRace` exists
and why the exercise's own instructions say to run `go test --race`:
a bug like this can hide from you indefinitely under plain `go test`
and still be live in production.

**The takeaway isn't "don't use a shared map."** It's "don't use a
shared map *without synchronizing access to it*." Approach 1 avoids
the problem by never sharing a mutable map across goroutines in the
first place — Approach 2 below shows the other legitimate way to
solve it: share the map, but synchronize each piece of it.

## Approach 2 (alternative, real-world contrast): sharded map with per-shard locks

**Worth flagging up front: this approach does not satisfy what the
exercise actually asks for.** The exercise's own comments and
`README.md` are specific that the map phase should touch "no shared
mutable state" and build results into each goroutine's "own local
`map[string]int`." The design below has every map-phase goroutine
write directly into one shared structure — which is exactly the shape
the "common mistake" above also has. It's included here anyway because
it's a real, distinct technique worth knowing, and because contrasting
it with the mistake above makes the actual rule clearer: **the defect
above was *unsynchronized* concurrent map access, not "a shared map"
per se.** A shared map is safe exactly when every access to a given
key is properly serialized against every other access to that same
key — which is what sharding buys you.

```go
const numShards = 16

type shard struct {
	mu     sync.Mutex
	counts map[string]int
}

type shardedMap struct {
	shards [numShards]*shard
}

func newShardedMap() *shardedMap {
	sm := &shardedMap{}
	for i := range sm.shards {
		sm.shards[i] = &shard{counts: make(map[string]int)}
	}
	return sm
}

func (sm *shardedMap) shardFor(word string) *shard {
	h := fnv.New32a()
	h.Write([]byte(word))
	return sm.shards[h.Sum32()%numShards]
}

func (sm *shardedMap) add(word string, n int) {
	s := sm.shardFor(word)
	s.mu.Lock()
	s.counts[word] += n
	s.mu.Unlock()
}

func (sm *shardedMap) merge() map[string]int {
	result := make(map[string]int)
	for _, s := range sm.shards {
		s.mu.Lock()
		for word, count := range s.counts {
			result[word] += count
		}
		s.mu.Unlock()
	}
	return result
}

func WordCount(chunks []string) map[string]int {
	sm := newShardedMap()

	var wg sync.WaitGroup
	wg.Add(len(chunks))

	for _, chunk := range chunks {
		chunk := chunk
		go func() {
			defer wg.Done()
			time.Sleep(ProcessDelay)
			for _, word := range strings.Fields(chunk) {
				sm.add(word, 1)
			}
		}()
	}

	wg.Wait()
	return sm.merge()
}
```

The map (word → shard index, by hash) is split across `numShards`
independent mutexes. Two goroutines updating words that hash to
*different* shards never contend at all; two updates to the same word
(same shard) are still safely serialized, by that shard's own lock,
never a global one. This also passed `go test -race -count=3` cleanly
in a scratch copy, with the same timings as Approach 1.

When would you actually reach for this over Approach 1? When the
partial results are too large to comfortably hold `len(chunks)`
separate full copies in memory at once (e.g. very high-cardinality
keys across very many chunks), or when you want writers to make
progress incrementally into one structure rather than waiting for a
distinct reduce pass — the classic real-world case is a long-lived
concurrent counter or cache, not a one-shot batch job like this
exercise. For this exercise's actual constraints (ten to thirty small
chunks, one batch call), Approach 1 is simpler, cheaper, and — again —
what the exercise is actually testing your understanding of.

## Key takeaways

- The naive version's defect is throughput, not correctness: it never
  drops or double-counts a word, so `TestWordCountCorrectness` passes
  against it; it fails only `TestWordCountConcurrency`, which measures
  wall-clock time on a fake clock via `synctest.Test`.
- "Spawn a goroutine per chunk" alone doesn't make code correct — it
  only fixes the throughput test. If those goroutines still write into
  one shared map with no synchronization, you've traded a throughput
  bug for a data race, and that race is fully capable of hiding from
  plain `go test` for many runs while still being live, undefined
  behavior. That's exactly why the exercise's own instructions call
  for `go test --race`, and why `TestWordCountRace` exists as a
  dedicated stress test.
- The rule is "don't touch shared mutable state without synchronizing
  it," not "never share a map." Approach 1 sidesteps the problem
  (local maps, merged once, sequentially); Approach 2 solves the same
  problem the other legitimate way (a shared map, but with per-key
  access properly serialized via sharding) — but Approach 2 is a
  real-world variant, not what this specific exercise is asking you to
  build.
- `sync.WaitGroup.Wait()` is what creates the happens-before edge
  between "all map-phase goroutines finished writing their partials"
  and "it's safe for the single reduce goroutine to read them" — that
  edge is what makes the reduce phase's lock-free reads safe, not
  anything about maps specifically.
