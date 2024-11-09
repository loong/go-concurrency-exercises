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

func producer(stream Stream, tweetsChan chan<- *Tweet) {
	for {
		tweet, err := stream.Next()
		if err == ErrEOF {
			close(tweetsChan)
			return
		}

		tweetsChan <- tweet
	}
}

func consumer(tweetsChan <-chan *Tweet, wg *sync.WaitGroup) {
	defer wg.Done()

	for tweet := range tweetsChan {
		if tweet.IsTalkingAboutGo() {
			fmt.Println(tweet.Username, "\ttweets about golang")
		} else {
			fmt.Println(tweet.Username, "\tdoes not tweet about golang")
		}
	}
}

func main() {
	start := time.Now()
	stream := GetMockStream()
	var wg sync.WaitGroup

	tweetsChan := make(chan *Tweet)

	// Producer
	go producer(stream, tweetsChan)

	wg.Add(1)
	// Consumer
	go consumer(tweetsChan, &wg)
	wg.Wait()

	fmt.Printf("Process took %s\n", time.Since(start))
}
