package main

import (
	"fmt"
	"hash/fnv"
	"time"
)

// ProcessingLatency is how long a single call to ProcessImage takes. It
// stands in for whatever expensive work a real thumbnail generator would
// do (downloading the image, decoding it, resizing it, ...).
const ProcessingLatency = 150 * time.Millisecond

// Thumbnail is the result of processing a single image URL.
type Thumbnail struct {
	URL  string
	Data string
}

// ProcessImage simulates generating a thumbnail for the image at url. It
// always takes ProcessingLatency to run, and always returns the same
// (fake) data for the same url, so tests can check correctness without
// needing a real image pipeline.
func ProcessImage(url string) Thumbnail {
	time.Sleep(ProcessingLatency)

	h := fnv.New64a()
	_, _ = h.Write([]byte(url))

	return Thumbnail{
		URL:  url,
		Data: fmt.Sprintf("thumb-%x", h.Sum64()),
	}
}
