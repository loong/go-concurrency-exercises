//////////////////////////////////////////////////////////////////////
//
// This file simulates a piece of log-collecting infrastructure and is
// given to you as-is - you don't need to (and shouldn't) change
// anything here.
//

package main

import (
	"fmt"
	"time"
)

// StartLogHub simulates log-collecting infrastructure where new "shard"
// sources connect over time. It returns a channel-of-channels: each
// value it emits is itself a channel carrying the log lines from one
// shard, which closes when that shard is done producing lines.
// StartLogHub closes the outer channel once numShards shards have been
// started.
func StartLogHub(numShards, linesPerShard int) <-chan (<-chan string) {
	shardStream := make(chan (<-chan string))

	go func() {
		defer close(shardStream)

		for shardIdx := 0; shardIdx < numShards; shardIdx++ {
			lines := make(chan string)

			go func(shardIdx int) {
				defer close(lines)

				for i := 0; i < linesPerShard; i++ {
					time.Sleep(10 * time.Millisecond)
					lines <- fmt.Sprintf("shard-%d-line-%d", shardIdx, i)
				}
			}(shardIdx)

			shardStream <- lines

			// New shards trickle in over time rather than all at once.
			time.Sleep(5 * time.Millisecond)
		}
	}()

	return shardStream
}
