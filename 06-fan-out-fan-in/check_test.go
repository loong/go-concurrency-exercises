//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"fmt"
	"hash/fnv"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// expectedThumbnailData mirrors the (deterministic) data-generation
// logic in ProcessImage, without paying for the simulated latency, so
// tests can check correctness cheaply.
func expectedThumbnailData(url string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(url))

	return fmt.Sprintf("thumb-%x", h.Sum64())
}

func testURLs(n int) []string {
	urls := make([]string, n)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/images/%d.jpg", i)
	}

	return urls
}

// TestGenerateThumbnailsCorrectness checks that every URL that goes in
// comes back out exactly once, with the right thumbnail data. It
// makes no assumption about ordering or timing, so it passes against
// both the naive sequential implementation and a fanned-out one.
func TestGenerateThumbnailsCorrectness(t *testing.T) {
	urls := testURLs(10)

	got := GenerateThumbnails(urls)

	if len(got) != len(urls) {
		t.Fatalf("expected %d thumbnails, got %d", len(urls), len(got))
	}

	byURL := make(map[string]Thumbnail, len(got))
	for _, th := range got {
		if _, dup := byURL[th.URL]; dup {
			t.Errorf("URL %q appeared more than once in the result", th.URL)
		}
		byURL[th.URL] = th
	}

	for _, url := range urls {
		th, ok := byURL[url]
		if !ok {
			t.Errorf("missing thumbnail for URL %q", url)
			continue
		}
		if want := expectedThumbnailData(url); th.Data != want {
			t.Errorf("thumbnail data for %q = %q, want %q", url, th.Data, want)
		}
	}
}

// TestGenerateThumbnailsConcurrency asserts that GenerateThumbnails
// actually fans work out across multiple goroutines instead of
// processing URLs one at a time. Ten URLs at 150ms each take 1.5s
// sequentially; a fanned-out implementation finishes in roughly one
// ProcessImage call's worth of (fake) time, regardless of how many
// URLs there are. synctest.Test runs the body on a fake clock that
// jumps forward as soon as every goroutine in the bubble is durably
// blocked, so this assertion is exact and doesn't flake on a busy
// machine. This test does not require a fixed-size worker pool - one
// goroutine per URL passes just as well as a bounded pool would.
func TestGenerateThumbnailsConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		urls := testURLs(10)

		start := time.Now()
		got := GenerateThumbnails(urls)
		elapsed := time.Since(start)

		if len(got) != len(urls) {
			t.Fatalf("expected %d thumbnails, got %d", len(urls), len(got))
		}

		const sequentialTime = 10 * ProcessingLatency
		const budget = 500 * time.Millisecond

		if elapsed >= budget {
			t.Errorf("GenerateThumbnails took %s (sequential would take %s); "+
				"want well under %s - looks like URLs are being processed one at a time "+
				"instead of concurrently", elapsed, sequentialTime, budget)
		}
	})
}

// TestGenerateThumbnailsConcurrentCorrectness stress-tests
// GenerateThumbnails with many URLs to catch data races on any shared
// slice/map used internally to collect results (run with `go test
// -race`).
func TestGenerateThumbnailsConcurrentCorrectness(t *testing.T) {
	urls := testURLs(50)

	got := GenerateThumbnails(urls)

	if len(got) != len(urls) {
		t.Fatalf("expected %d thumbnails, got %d", len(urls), len(got))
	}

	var mu sync.Mutex
	seen := make(map[string]bool, len(urls))

	var wg sync.WaitGroup
	for _, th := range got {
		th := th
		wg.Add(1)
		go func() {
			defer wg.Done()

			want := expectedThumbnailData(th.URL)

			mu.Lock()
			defer mu.Unlock()

			if seen[th.URL] {
				t.Errorf("URL %q appeared more than once in the result", th.URL)
			}
			seen[th.URL] = true

			if th.Data != want {
				t.Errorf("thumbnail data for %q = %q, want %q", th.URL, th.Data, want)
			}
		}()
	}
	wg.Wait()

	if len(seen) != len(urls) {
		t.Errorf("expected %d distinct URLs in result, got %d", len(urls), len(seen))
	}
}
