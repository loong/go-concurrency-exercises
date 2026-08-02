# Suggested Solutions

> **Spoiler warning.** Every file in this folder contains a full worked solution (sometimes two) for one exercise in this repo, including the exact Go code, verified to pass that exercise's own hidden tests (`go test -race`).
>
> These are meant for when you're stuck, or when you've already solved an exercise and want to compare your approach against another valid one — not as something to read before you've tried the exercise yourself. That's also why these live in their own folder instead of sitting next to each `main.go`: nothing here shows up while you're working in an exercise's directory unless you come looking for it.
>
> Each file explains *why* the naive starting code is wrong (sometimes it's a correctness bug, sometimes — as with the fan-out/fan-in exercises — the naive code is already correct and just too slow), walks through one idiomatic fix, and, where a second approach is genuinely different rather than a cosmetic variation, shows that too with its own tradeoffs.

## Index

| # | Exercise | Solution |
|---|----------|----------|
| 0 | Limit your Crawler | [00-limit-crawler.md](00-limit-crawler.md) |
| 1 | Producer-Consumer | [01-producer-consumer.md](01-producer-consumer.md) |
| 2 | Race Condition in Caching Scenario | [02-race-in-cache.md](02-race-in-cache.md) |
| 3 | Limit Service Time for Free-Tier Users | [03-limit-service-time.md](03-limit-service-time.md) |
| 4 | Graceful SIGINT Killing | [04-graceful-sigint.md](04-graceful-sigint.md) |
| 5 | Clean Inactive Sessions to Prevent Memory Overflow | [05-session-cleaner.md](05-session-cleaner.md) |
| 6 | Fan-Out, Fan-In: Concurrent Thumbnail Generation | [06-fan-out-fan-in.md](06-fan-out-fan-in.md) |
| 7 | Or-Done Channel: Stopping a Monitoring Feed Cleanly | [07-or-done-channel.md](07-or-done-channel.md) |
| 8 | Pipeline: Multi-Stage Number Processing | [08-pipeline.md](08-pipeline.md) |
| 9 | Context Cancellation & Propagation | [09-context-cancellation.md](09-context-cancellation.md) |
| 10 | Semaphore: Bounding Parallelism Against a Rate-Limited API | [10-semaphore.md](10-semaphore.md) |
| 11 | Worker Pool: Batch Job Processor with Partial Failures | [11-worker-pool.md](11-worker-pool.md) |
| 12 | Pub-Sub: In-Memory Event Bus | [12-pub-sub.md](12-pub-sub.md) |
| 13 | Channel of Channels (Bridge Pattern): Merging Dynamic Log Shards | [13-channel-of-channels.md](13-channel-of-channels.md) |
| 14 | Tee Channel: Duplicating a Sensor Stream | [14-tee-channel.md](14-tee-channel.md) |
| 15 | Or-Channel Combinator: Combining Shutdown Triggers | [15-or-channel-combinator.md](15-or-channel-combinator.md) |
| 16 | Your Own errgroup: Concurrent Tasks with First-Error Capture | [16-errgroup-failfast.md](16-errgroup-failfast.md) |
| 17 | Future/Promise Pattern: Async, Memoized Computation | [17-future-promise.md](17-future-promise.md) |
| 18 | Bounded Pipeline with Backpressure | [18-bounded-pipeline-backpressure.md](18-bounded-pipeline-backpressure.md) |
| 19 | Actor Model: A Bank Account with No Locks | [19-actor-model.md](19-actor-model.md) |
| 20 | Concurrent Map-Reduce: Parallel Word Count | [20-concurrent-map-reduce.md](20-concurrent-map-reduce.md) |
| 21 | Dining Philosophers: Deadlock Avoidance | [21-dining-philosophers.md](21-dining-philosophers.md) |
| 22 | Circuit Breaker: Protecting a Flaky Payment Gateway | [22-circuit-breaker.md](22-circuit-breaker.md) |
| 23 | Sharded Concurrent Cache: Reducing Lock Contention | [23-sharded-cache.md](23-sharded-cache.md) |
| 24 | Priority Worker Pool: Weighted Scheduling | [24-priority-worker-pool.md](24-priority-worker-pool.md) |
| 25 | Graceful Multi-Stage Shutdown | [25-graceful-multistage-shutdown.md](25-graceful-multistage-shutdown.md) |
| 26 | Pipeline Error Handling: Result Values Instead of Aborting | [26-pipeline-error-handling.md](26-pipeline-error-handling.md) |
| 27 | Heartbeats: Detecting a Stalled Worker Before It's Too Late | [27-heartbeats.md](27-heartbeats.md) |
| 28 | Replicated Requests: Racing Redundant Calls for Lower Tail Latency | [28-replicated-requests.md](28-replicated-requests.md) |
| 29 | Healing Unhealthy Goroutines: A Steward That Restarts a Wedged Ward | [29-healing-goroutines.md](29-healing-goroutines.md) |
| 30 | Livelock & Starvation: Two Failure Modes Beyond Deadlock | [30-livelock-starvation.md](30-livelock-starvation.md) |

## A note on exercises 0-5

Exercises 0 through 5 predate this `solutions/` folder and may already have your own in-progress or finished code sitting in their `main.go`. The write-ups for those five were done independently, from each exercise's original naive starting point (not from whatever you'd already written), so you get a clean reference to compare against rather than a mirror of your own draft.
