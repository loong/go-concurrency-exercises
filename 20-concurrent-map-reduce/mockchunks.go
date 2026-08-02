package main

import "time"

// ProcessDelay simulates the per-chunk tokenizing/counting cost - the time
// it would take to scan a chunk of text and count the words in it.
const ProcessDelay = 30 * time.Millisecond
