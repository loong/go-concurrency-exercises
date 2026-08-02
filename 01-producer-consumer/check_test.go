//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMainProducesCorrectOutput runs the program's actual main() and
// checks that every tweet is classified correctly and printed in the
// same order the mock stream produces them - regardless of how
// producer/consumer are wired up internally, since main() is the only
// part of the contract this exercise doesn't let you change.
func TestMainProducesCorrectOutput(t *testing.T) {
	output := captureStdout(t, main)

	want := [][2]string{
		{"davecheney", "tweets about golang"},
		{"beertocode", "does not tweet about golang"},
		{"ironzeb", "tweets about golang"},
		{"beertocode", "tweets about golang"},
		{"vampirewalk666", "tweets about golang"},
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < len(want) {
		t.Fatalf("expected at least %d lines of output, got %d:\n%s", len(want), len(lines), output)
	}
	for i, w := range want {
		if !strings.Contains(lines[i], w[0]) || !strings.Contains(lines[i], w[1]) {
			t.Errorf("line %d = %q, want it to mention %q and %q", i, lines[i], w[0], w[1])
		}
	}
}

// TestMainRunsProducerAndConsumerConcurrently checks that the total
// time is close to the slower of the two stages run back-to-back per
// tweet, rather than close to their sum. Each Stream.Next() sleeps
// 320ms and each IsTalkingAboutGo() sleeps 330ms; running them fully
// sequentially (read all 5, then classify all 5) takes roughly
// 5*(320+330)ms = 3.25s. Pipelining them - the producer reading ahead
// while the consumer is still classifying an earlier tweet - takes
// closer to one read plus five classifications, ~1.97s, since
// classifying tweet N overlaps with reading tweet N+1. 2.5s is well
// above the pipelined time and well below the sequential time, so it
// reliably tells the two apart without being flaky about exactly how
// much the pipeline saves.
func TestMainRunsProducerAndConsumerConcurrently(t *testing.T) {
	start := time.Now()
	captureStdout(t, main)
	elapsed := time.Since(start)

	if elapsed > 2500*time.Millisecond {
		t.Errorf("main() took %s, want well under the ~3.25s a fully sequential producer+consumer would take - producer and consumer must run concurrently", elapsed)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and
// returns everything written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	fn()

	w.Close()
	os.Stdout = orig
	<-done

	return buf.String()
}
