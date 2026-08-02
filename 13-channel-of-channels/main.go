//////////////////////////////////////////////////////////////////////
//
// A LogHub (see mockloghub.go) simulates log-collecting infrastructure
// where new "shard" sources connect over time: StartLogHub returns a
// channel-of-channels - a "bridge channel" - where each value received
// is itself a channel carrying the log lines of one shard, closing
// when that shard finishes. New shards keep trickling in, and the
// total number of shards isn't known upfront by whoever is consuming
// the bridge; StartLogHub only signals "no more shards" by closing the
// outer channel once the last one has been started.
//
// The naive Bridge below only looks at the very first inner channel it
// receives from chanStream: it forwards every line from that one shard
// to its output channel, and once that shard's channel closes, Bridge
// returns and closes its own output. It never goes back to chanStream
// to pick up the second, third, ... shard, so as soon as the first
// shard finishes, every log line from every other shard is silently
// dropped on the floor - even though the hub keeps producing them in
// the background.
//
// Your task is to fix Bridge so that it flattens EVERY inner channel
// it ever receives from chanStream into the single output stream - a
// classic fan-in, except the set of input channels is not known ahead
// of time and arrives dynamically over the bridge channel itself. The
// output channel must only close once chanStream has been closed AND
// every inner channel Bridge has ever received from it has been fully
// drained. In addition, Bridge must stop promptly - closing its output
// and abandoning any further reads from chanStream and from the inner
// channels - as soon as done is closed. Keep the function signature
// identical:
//
//     func Bridge(chanStream <-chan (<-chan string), done <-chan struct{}) <-chan string
//

package main

import "fmt"

// Bridge flattens a channel-of-channels into a single output channel of
// values.
//
// NAIVE / BROKEN: it only ever reads the first inner channel it
// receives from chanStream, forwards its lines, and then returns -
// dropping every subsequent shard chanStream ever produces.
func Bridge(chanStream <-chan (<-chan string), done <-chan struct{}) <-chan string {
	valStream := make(chan string)

	go func() {
		defer close(valStream)

		stream, ok := <-chanStream
		if !ok {
			return
		}

		for {
			select {
			case <-done:
				return
			case v, ok := <-stream:
				if !ok {
					return
				}
				select {
				case valStream <- v:
				case <-done:
					return
				}
			}
		}
	}()

	return valStream
}

func main() {
	done := make(chan struct{})
	// done is never closed during this normal run.

	shardStream := StartLogHub(5, 4)
	logs := Bridge(shardStream, done)

	count := 0
	for line := range logs {
		fmt.Println(line)
		count++
	}

	fmt.Println("Total log lines:", count)
}
