# Producer-Consumer — Suggested Solutions

> **Spoiler warning.** This file contains full worked solutions for `01-producer-consumer/`. Try solving it yourself first — come back here if you're stuck or want to compare approaches.

## The problem

A producer reads tweets one at a time from a mock stream (each read takes ~320ms). A consumer then checks each tweet for Go-related content (~330ms per check). The starting code runs them back-to-back:

```go
func producer(stream Stream) (tweets []*Tweet) {
	for {
		tweet, err := stream.Next()
		if err == ErrEOF {
			return tweets
		}
		tweets = append(tweets, tweet)
	}
}

func consumer(tweets []*Tweet) {
	for _, t := range tweets {
		if t.IsTalkingAboutGo() {
			fmt.Println(t.Username, "\ttweets about golang")
		} else {
			fmt.Println(t.Username, "\tdoes not tweet about golang")
		}
	}
}

func main() {
	stream := GetMockStream()
	tweets := producer(stream)
	consumer(tweets)
}
```

The task is to let the producer and consumer overlap so the total runtime approaches the *slowest* stage instead of the *sum* of both stages.

## Why the naive version is wrong

It isn't wrong in the sense of a bug — it produces correct output. It's wrong in the sense the exercise cares about: it's **fully serialized**. `producer` must read all 5 tweets (plus the terminating `ErrEOF` check) before `consumer` looks at a single one. The cost is additive:

- Producer: 6 calls to `stream.Next()` (5 tweets + 1 EOF) × 320ms ≈ 1.92s
- Consumer: 5 calls to `IsTalkingAboutGo()` × 330ms ≈ 1.65s
- Total: ≈ 3.57s

Measured in a scratch build of this exact naive code: **3.582s** — matching that prediction (and the README's "Before" figure of 3.58s).

If producer and consumer instead run concurrently and hand off tweets one at a time, the runtime collapses to roughly `(one stream read) + (5 × max(read, process))`, i.e. governed by the *max* of the two per-item costs, not their sum:

- 1st tweet ready at 320ms, consumer finishes it at 650ms
- 2nd tweet ready at 640ms (queued), consumer finishes at 980ms
- ... and so on, landing the last item at ≈ 1.97s

Measured for the concurrent version below: **1.976s** — matching both the arithmetic and the README's "After" figure of 1.977756255s almost exactly.

There's also a structural constraint worth calling out before picking a design: `mockstream.go` states the `Stream` is "not threadsafe," and `Next()` mutates `s.pos` through a pointer receiver. **Exactly one goroutine may ever call `stream.Next()`.** Whatever else you fan out, the producer side must stay single-threaded — that's what keeps the solutions below race-free without a mutex.

## Approach 1: producer returns a channel, consumer ranges over it

This is the idiomatic fix and the one the exercise is steering you toward: turn the producer into something that streams results instead of returning a finished slice.

```go
func producer(stream Stream) <-chan *Tweet {
	out := make(chan *Tweet)
	go func() {
		defer close(out)
		for {
			tweet, err := stream.Next()
			if err == ErrEOF {
				return
			}
			out <- tweet
		}
	}()
	return out
}

func consumer(tweets <-chan *Tweet) {
	for t := range tweets {
		if t.IsTalkingAboutGo() {
			fmt.Println(t.Username, "\ttweets about golang")
		} else {
			fmt.Println(t.Username, "\tdoes not tweet about golang")
		}
	}
}

func main() {
	start := time.Now()
	stream := GetMockStream()

	tweets := producer(stream)
	consumer(tweets)

	fmt.Printf("Process took %s\n", time.Since(start))
}
```

Why this works:

- `producer` now launches a goroutine that owns the `Stream` value exclusively and pushes each tweet onto an unbuffered channel as soon as it's read, then `close`s the channel when the stream is exhausted. Returning the receive-only end (`<-chan *Tweet`) makes the API self-documenting: callers can only read, never close or send.
- `consumer` uses `for t := range tweets`, which blocks until a value is available and exits cleanly the moment the channel is closed — no separate "done" signal needed.
- The unbuffered channel naturally paces the two sides: the producer blocks on `out <- tweet` until the consumer is ready for the previous item, and the consumer blocks on the range until the next item exists. They run concurrently, but throughput is capped by whichever stage is slower per item (here, the consumer's 330ms vs. the producer's 320ms).

**Verified** in a scratch copy (never in the live `01-producer-consumer/` directory):
- `go run .` → prints all 5 lines in order, `Process took 1.976337959s` (vs. 3.582158667s for the naive baseline copied from git HEAD).
- `go run -race .` → clean, no data race reported.
- A throwaway instrumented test confirmed the first tweet arrives on the channel after ~320ms (one stream read), not after ~1.6s (all five reads) — i.e., the producer is genuinely streaming, not silently building the whole slice first before consumption starts.

Would a buffered channel help here? No — the consumer is the tail bottleneck (330ms > 320ms), so buffering only lets the producer *finish* submitting slightly earlier; the last item still isn't consumed until ≈1.97s. Buffering only pays off when the *producer* is the bottleneck and can outrun a bursty consumer.

## Approach 2: fan-out to multiple consumer workers (alternative)

A genuinely different design: keep the single-producer goroutine (required, since `Stream` isn't threadsafe), but spin up several consumer goroutines pulling from the same channel, synchronized with a `sync.WaitGroup` instead of relying solely on range-until-closed in one place.

```go
func producer(stream Stream) <-chan *Tweet {
	out := make(chan *Tweet)
	go func() {
		defer close(out)
		for {
			tweet, err := stream.Next()
			if err == ErrEOF {
				return
			}
			out <- tweet
		}
	}()
	return out
}

func consumer(tweets <-chan *Tweet, wg *sync.WaitGroup) {
	defer wg.Done()
	for t := range tweets {
		if t.IsTalkingAboutGo() {
			fmt.Println(t.Username, "\ttweets about golang")
		} else {
			fmt.Println(t.Username, "\tdoes not tweet about golang")
		}
	}
}

func main() {
	start := time.Now()
	stream := GetMockStream()

	tweets := producer(stream)

	const numWorkers = 3
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go consumer(tweets, &wg)
	}
	wg.Wait()

	fmt.Printf("Process took %s\n", time.Since(start))
}
```

Multiple goroutines can safely range over the same channel — each value is delivered to exactly one of them, so no two workers ever process the same tweet. `wg.Wait()` in `main` blocks until every worker has drained the (now-closed) channel and returned.

**Verified** in the same scratch copy: builds, `go vet` clean, `go run -race .` reports no race, output correct — `Process took ~1.935s` (3 runs: 1.9348s / 1.9346s / 1.9354s).

Two honest caveats, confirmed against the measurements above rather than assumed:

- **It barely helps on this workload.** ~1.935s vs. ~1.976s for the single-consumer version — only ~40ms faster. That's because the producer's fixed 320ms-per-item cadence is the bottleneck, not the consumer's 330ms; fanning out the consumer can't make the producer read faster. This pattern earns its keep when the *consumer* side is the bottleneck (e.g., a slow network call per item) and you want several in flight at once — not here.
- **It gives up strict output ordering.** With one channel and several readers, delivery order across workers isn't guaranteed. The runs above happened to print in the original order because each worker's ~330ms processing time is close to the producer's ~320ms cadence, which staggers the workers just enough to avoid interleaving — that's a coincidence of this workload's timing, not a guarantee. Don't be surprised if your own run comes out shuffled relative to the README's sample output; that doesn't mean it's wrong.

## Key takeaways

- The naive version isn't buggy, just serial: total time = sum of every stage's cost. Overlapping producer and consumer turns that sum into roughly the max of the two stages plus one item's pipeline-fill delay — verified here as 3.58s → 1.98s, matching the README's own before/after numbers almost exactly.
- The standard idiom is: producer returns a `<-chan T` and closes it from inside the goroutine that owns the writes; consumer uses `for range` on the channel. No explicit "done" flag, mutex, or manual synchronization needed for the single-consumer case.
- `mockstream.go`'s "not threadsafe" comment on `Stream` is load-bearing: only one goroutine may ever call `stream.Next()`. You can safely fan out consumers, but never the producer — that constraint is exactly what keeps both solutions above race-free.
- Fan-out (Approach 2) is a real, distinct pattern (worker pool + `sync.WaitGroup`), but only pays off when the consumer stage — not the producer — is the bottleneck. Here it isn't, so the gain is marginal and ordering is no longer guaranteed.
