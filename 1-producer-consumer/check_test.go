package main

import (
	"strings"
	"testing"
)

func TestMain(t *testing.T) {
	result := processStream(GetMockStream())
	lines := strings.Split(strings.TrimSpace(result), "\n")
	expected := []string{
		"davecheney \ttweets about golang",
		"beertocode \tdoes not tweet about golang",
		"ironzeb \ttweets about golang",
		"beertocode \ttweets about golang",
		"vampirewalk666 \ttweets about golang",
	}
	if len(lines) != len(expected)+1 { // +1 for the "Process took" line
		t.Fatalf("Expected %d lines, got %d", len(expected)+1, len(lines))
	}

	for i, line := range expected {
		if !strings.EqualFold(lines[i], line) {
			t.Errorf("Line %d: expected %q, got %q", i+1, line, lines[i])
		}
	}

	if !strings.HasPrefix(lines[len(lines)-1], "Process took") {
		t.Errorf("Last line should start with 'Process took', got %q", lines[len(lines)-1])
	}
}
