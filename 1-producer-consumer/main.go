//////////////////////////////////////////////////////////////////////
//
// Given is a producer-consumer scenario, where a producer reads in
// tweets from a mockstream and a consumer is processing the
// data. Your task is to change the code so that the producer as well
// as the consumer can run concurrently
//

package main

import (
	"fmt"
	"sync"
	"time"
)

func producer(stream Stream, conn chan *Tweet, wg *sync.WaitGroup) {
	wg.Done()
	for {
		tweet, err := stream.Next()
		if err == ErrEOF {
			close(conn)
			return
		}
		conn <- tweet
	}
}

func consumer(conn chan *Tweet, wg *sync.WaitGroup) {
	defer wg.Done()
	for t := range conn {
		if t.IsTalkingAboutGo() {
			fmt.Println(t.Username, "\ttweets about golang")
		} else {
			fmt.Println(t.Username, "\tdoes not tweet about golang")
		}
	}
}

func main() {
	start := time.Now()
	stream := GetMockStream()

	conn := make(chan *Tweet)
	var wg sync.WaitGroup
	wg.Add(2)
	// Producer
	go producer(stream, conn, &wg)

	// Consumer
	go consumer(conn, &wg)

	wg.Wait()

	fmt.Printf("Process took %s\n", time.Since(start))
}
