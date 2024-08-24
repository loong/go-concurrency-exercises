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
	"time"
)

func producer(stream Stream, tweets chan<- Tweet) {
	for {
		tweet, err := stream.Next()
		if err == ErrEOF {
			close(tweets)
			return
		}
		tweets <- *tweet
	}
}

func consumer(tweets <-chan Tweet, done chan<- bool) {
	for t := range tweets {
		if t.IsTalkingAboutGo() {
			fmt.Println(t.Username, "\ttweets about golang")
		} else {
			fmt.Println(t.Username, "\tdoes not tweet about golang")
		}
	}
	done <- true
}

func main() {
	start := time.Now()
	stream := GetMockStream()
	tweets := make(chan Tweet, 10)
	done := make(chan bool)

	// Producer
	go producer(stream, tweets)

	// Consumer
	go consumer(tweets, done)

	<-done
	fmt.Printf("Process took %s\n", time.Since(start))

}
