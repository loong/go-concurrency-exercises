# Fan-Out, Fan-In: Concurrent Thumbnail Generation

Given is a function `GenerateThumbnails` that generates a thumbnail
for every URL in a list by calling `ProcessImage`, which simulates a
slow operation (decoding and resizing an image) by sleeping for a
fixed amount of time before returning a result. The current
implementation processes the URLs one at a time in a simple
sequential loop, so the total time grows linearly with the number of
images - even though each call to `ProcessImage` is completely
independent of the others.

Your task is to change `GenerateThumbnails` so that it fans the work
out across multiple worker goroutines processing images
concurrently, and then fans the results back in into a single slice
that is returned to the caller. The result must contain every input
URL exactly once - no dropped results, no duplicated work, and no
data races - and the function signature must stay the same:

One goroutine per URL (with a `sync.WaitGroup` to fan the results
back in) is a perfectly acceptable solution here - you don't need a
fixed-size worker pool for this exercise. Bounding concurrency with a
pool or semaphore is its own topic, covered later in
[10-semaphore](../10-semaphore) and [11-worker-pool](../11-worker-pool).

```go
func GenerateThumbnails(urls []string) []Thumbnail
```

## Test your solution

To complete this exercise, you must pass the tests:

```
go test
go test --race
```
