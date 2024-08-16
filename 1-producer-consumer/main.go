//////////////////////////////////////////////////////////////////////
//
// Given is a producer-consumer scenario, where a producer reads in
// tweets from a mockstream and a consumer is processing the
// data. Your task is to change the code so that the producer as well
// as the consumer can run concurrently
//

package main

import (
	"bytes"
	"fmt"
	"sync"
	"time"
)

func producer(stream Stream, ch chan *Tweet) {
	for {
		tweet, err := stream.Next()
		if err == ErrEOF {
			close(ch)
			return
		}

		ch <- tweet
	}
}

func consumer(ch chan *Tweet, output *bytes.Buffer, wg *sync.WaitGroup) {
	defer wg.Done()
	for t := range ch {
		if t.IsTalkingAboutGo() {
			fmt.Fprintln(output, t.Username, "\ttweets about golang")
		} else {
			fmt.Fprintln(output, t.Username, "\tdoes not tweet about golang")
		}
	}
}

func processStream(stream Stream) string {
	start := time.Now()
	var output bytes.Buffer
	var wg sync.WaitGroup

	ch := make(chan *Tweet)

	// Producer
	go producer(stream, ch)

	// Consumer
	wg.Add(1)
	go consumer(ch, &output, &wg)
	wg.Wait()

	fmt.Fprintf(&output, "Process took %s\n", time.Since(start))
	return output.String()
}

func main() {
	stream := GetMockStream()
	result := processStream(stream)
	fmt.Println(result)
}
