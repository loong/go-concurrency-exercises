# Limit Your Crawler — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `00-limit-crawler/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

`Crawl()` recursively fetches pages and spawns a new goroutine for every link it finds, so it hammers `fetcher.Fetch()` as fast as the goroutine scheduler allows. The task is to throttle it to at most one `Fetch()` call per second *system-wide*, while still launching each `Crawl()` call in its own goroutine (the `go Crawl(...)` line must stay).

The hidden test (`check_test.go`) hooks `fetchSignalInstance()` from `mockfetcher.go`, which fires every time `Fetch` is called, and fails as soon as two fetches land less than ~0.95s apart.

## Why the naive version is wrong

The starting code has no shared throttle at all — every spawned goroutine calls `fetcher.Fetch` the moment it runs, so with 4 URLs fanning out across depth levels, most fetches happen within milliseconds of each other. Confirmed empirically: running the unmodified stub finishes (and fails) in well under a second:

```
--- FAIL: TestMain (0.00s)
    check_test.go:24: There exists a two crawls that were executed less than 1 second apart.
    check_test.go:25: Solution is incorrect.
FAIL
```

Concurrency itself isn't the bug — the fix must *keep* `go Crawl(...)`. What's missing is a single, shared gate that every goroutine funnels through before it's allowed to fetch.

## Approach 1: `time.Tick` gate (the "3 lines" hint)

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

// throttle ticks once per second; every Fetch must wait for a tick
// before proceeding, so no two fetches can start less than 1s apart,
// no matter how many goroutines are racing to call Crawl concurrently.
var throttle = time.Tick(time.Second)

// Crawl uses `fetcher` from the `mockfetcher.go` file to imitate a
// real crawler. It crawls until the maximum depth has reached.
func Crawl(url string, depth int, wg *sync.WaitGroup) {
	defer wg.Done()

	if depth <= 0 {
		return
	}

	<-throttle
	body, urls, err := fetcher.Fetch(url)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("found: %s %q\n", url, body)

	wg.Add(len(urls))
	for _, u := range urls {
		// Do not remove the `go` keyword, as Crawl() must be
		// called concurrently
		go Crawl(u, depth-1, wg)
	}
}

func main() {
	var wg sync.WaitGroup

	wg.Add(1)
	Crawl("http://golang.org/", 4, &wg)
	wg.Wait()
}
```

**How it works:** `time.Tick(time.Second)` returns a channel that receives a value once per second, forever. It's a package-level variable, so every goroutine spawned by `go Crawl(...)` shares the *same* channel. A channel receive (`<-throttle`) only succeeds for one goroutine at a time — Go's runtime hands the value to exactly one waiting receiver — so however many goroutines are blocked on `<-throttle` simultaneously, only one proceeds per tick. That serializes calls to `fetcher.Fetch` to at most one per second, while every goroutine still runs (and blocks) concurrently, satisfying "`Crawl()` must be called concurrently."

Verified in a scratch copy of the exercise directory: `go test -race -count=5 ./...` passed all 5 runs (`ok  	limitcrawler	66.937s`, i.e. ~13.4s/run, matching the README's expected "13.009s" ballpark for a 4-URL/depth-4 crawl).

**Tradeoffs:** `time.Tick` is documented as leaking its underlying ticker forever (there's no way to stop it) — acceptable for a short-lived program like this exercise, but in a long-running service you'd want `time.NewTicker` plus an explicit `Stop()` (e.g. deferred in `main`, with the ticker itself still a package-level or otherwise shared variable so all goroutines see the same one). Either way, the essential trick is the same: one shared channel, one receive per fetch.

## Approach 2: Mutex-guarded "last fetch" timestamp + `time.Sleep`

```go
package main

import (
	"fmt"
	"sync"
	"time"
)

var (
	rateMu    sync.Mutex
	lastFetch time.Time
)

// rateLimit blocks the caller until at least one second has elapsed
// since the previous call returned, across all goroutines.
func rateLimit() {
	rateMu.Lock()
	defer rateMu.Unlock()
	if wait := time.Second - time.Since(lastFetch); wait > 0 {
		time.Sleep(wait)
	}
	lastFetch = time.Now()
}

// Crawl uses `fetcher` from the `mockfetcher.go` file to imitate a
// real crawler. It crawls until the maximum depth has reached.
func Crawl(url string, depth int, wg *sync.WaitGroup) {
	defer wg.Done()

	if depth <= 0 {
		return
	}

	rateLimit()
	body, urls, err := fetcher.Fetch(url)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("found: %s %q\n", url, body)

	wg.Add(len(urls))
	for _, u := range urls {
		// Do not remove the `go` keyword, as Crawl() must be
		// called concurrently
		go Crawl(u, depth-1, wg)
	}
}

func main() {
	var wg sync.WaitGroup

	wg.Add(1)
	Crawl("http://golang.org/", 4, &wg)
	wg.Wait()
}
```

**How it works:** instead of a channel that ticks on a fixed schedule, this tracks the wall-clock time of the last fetch in a shared variable and computes, per call, how much longer to wait. The `sync.Mutex` makes "check how long since last fetch, sleep the remainder, then update the timestamp" one atomic critical section — without it, two goroutines could both read the same stale `lastFetch`, both sleep the same (too-short) duration, and both fetch within the same window. Holding the lock across the `time.Sleep` is what forces goroutines to queue up and take their turn strictly one second apart, rather than racing each other.

**Tradeoffs:** more lines than Approach 1 and slightly more subtle to get right (the classic bug is releasing the lock before sleeping, which reintroduces the race). Its advantage is flexibility — the wait duration is computed dynamically, so it's easy to extend to variable/adaptive rate limits (e.g. back off longer after an error) in a way a fixed `time.Tick` channel doesn't naturally support. For a plain constant-rate limiter, Approach 1 is simpler and idiomatic Go, matching the "3 lines" hint.

Verified in the same scratch setup: `go test -race -count=3 ./...` passed all 3 runs (`ok  	limitcrawler	39.801s`).

## Key takeaways

- Throttling concurrent work means **serializing access to a shared gate**, not removing concurrency — `go Crawl(...)` stays; only the moment of `Fetch()` gets synchronized.
- A package-level `time.Tick` (or `time.NewTicker`) channel is a clean way to express "at most once per interval" across arbitrarily many goroutines: only one goroutine can receive each tick.
- The mutex + timestamp variant shows the same guarantee can be built from lower-level primitives (`sync.Mutex`, `time.Sleep`, `time.Since`) — useful to know when the rate needs to vary dynamically, but it's easy to introduce a race if the lock doesn't span both the "check" and the "update".
- Either approach is verified against the repo's actual hidden test (`fetchSignalInstance()` in `mockfetcher.go`, checked in `check_test.go`), which fails fast (in well under a second) on the naive version and passes in ~13s per run on both fixes above.
