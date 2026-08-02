//////////////////////////////////////////////////////////////////////
//
// Given is a function GenerateThumbnails that, given a list of image
// URLs, generates a thumbnail for each of them by calling
// ProcessImage (see mockimageprocessor.go). ProcessImage simulates a
// slow operation - decoding an image and resizing it - by sleeping
// for a fixed amount of time before returning the result.
//
// The naive implementation below processes the URLs one at a time,
// in a simple sequential loop. This works, but it means the total
// time to generate all thumbnails grows linearly with the number of
// images: 8 images at 150ms each takes well over a second, even
// though each ProcessImage call is completely independent of the
// others and could just as well run at the same time as the rest.
//
// Your task is to change GenerateThumbnails so that it fans the work
// out across multiple worker goroutines that call ProcessImage
// concurrently, and then fans the results back in into a single
// slice that is returned to the caller - without dropping any
// result, without processing any URL twice, and without introducing
// any data races. The function signature must stay the same:
//
//     func GenerateThumbnails(urls []string) []Thumbnail
//
// so that it remains a drop-in replacement for the sequential
// version below.
//
// One goroutine per URL, fanned back in with a sync.WaitGroup, is a
// perfectly fine solution here - a fixed-size worker pool is not
// required. Bounding concurrency is its own topic, covered later in
// 10-semaphore and 11-worker-pool.
//

package main

import (
	"fmt"
	"time"
)

// GenerateThumbnails generates a thumbnail for every URL in urls and
// returns the results. It currently does so sequentially, one URL at
// a time.
func GenerateThumbnails(urls []string) []Thumbnail {
	thumbnails := make([]Thumbnail, 0, len(urls))

	for _, url := range urls {
		thumbnails = append(thumbnails, ProcessImage(url))
	}

	return thumbnails
}

func main() {
	urls := []string{
		"https://example.com/images/1.jpg",
		"https://example.com/images/2.jpg",
		"https://example.com/images/3.jpg",
		"https://example.com/images/4.jpg",
		"https://example.com/images/5.jpg",
		"https://example.com/images/6.jpg",
		"https://example.com/images/7.jpg",
		"https://example.com/images/8.jpg",
	}

	start := time.Now()
	thumbnails := GenerateThumbnails(urls)
	elapsed := time.Since(start)

	for _, t := range thumbnails {
		fmt.Printf("%s -> %s\n", t.URL, t.Data)
	}

	fmt.Printf("Generated %d thumbnails in %s\n", len(thumbnails), elapsed)
}
