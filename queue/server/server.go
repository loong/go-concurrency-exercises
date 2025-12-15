package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"vu/benchmark/queue/internal"
	"vu/benchmark/queue/tasks"
)

type response struct {
	ID     string `json:"id"`
	Result []byte `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

var waitingGoroutines int64

// Serve listens for TCP connections and forwards incoming tasks to the queue.
func Serve(addr string, queue internal.IQueue, done <-chan struct{}) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	var wg sync.WaitGroup

	// Stop accepting new connections when shutdown is signaled.
	go func() {
		<-done
		listener.Close()
	}()

	go func() {
		for {
			select {
			case <-done:
				{
					break
				}
			default:
				time.Sleep(5 * time.Second)
				fmt.Println("Goroutines count: ", atomic.LoadInt64(&waitingGoroutines))
			}
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-done:
				wg.Wait()
				return nil
			default:
			}

			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				fmt.Println("temporary accept error:", err)
				continue
			}

			wg.Wait()
			return err
		}

		wg.Add(1)
		go func(c net.Conn) {
			idx := atomic.AddInt64(&waitingGoroutines, 1)
			defer func() {
				wg.Done()
				fmt.Printf("Goroutine %d exits\n", idx+1)
				atomic.AddInt64(&waitingGoroutines, -1)
			}()
			fmt.Printf("Goroutine %d accpet connection\n", idx+1)
			handleConnection(c, queue, done)
		}(conn)
	}
}

func handleConnection(conn net.Conn, queue internal.IQueue, done <-chan struct{}) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	// Results coming back from workers
	results := make(chan response, 16)

	// Writer goroutine
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for resp := range results {
			encoder.Encode(resp)
		}
	}()

	for {
		var task tasks.Task

		// Detect shutdown
		select {
		case <-done:
			// Tell writer goroutine to exit
			close(results)
			<-writeDone
			return
		default:
		}

		// Read next tasks
		if err := decoder.Decode(&task); err != nil {
			if errors.Is(err, io.EOF) {
				// client closed connection normally
				close(results)
				<-writeDone
				return
			}
			fmt.Printf("decode error from %s: %v\n", conn.RemoteAddr(), err)

			// send error to client before closing
			results <- response{Error: err.Error()}
			close(results)
			<-writeDone
			return
		}

		ch, err := queue.Put(&task)
		if err != nil {
			results <- response{ID: task.Id, Error: err.Error()}
			continue
		}

		// Spawn worker response waiters
		go func(id string, workerCh <-chan internal.Output) {
			output := <-workerCh
			resp := response{ID: id, Result: output.Res}
			if output.Err != nil {
				resp.Error = output.Err.Error()
				resp.Result = nil
			}

			// Avoid panic if `results` is already closed during shutdown
			select {
			case results <- resp:
			case <-done:
			}
		}(task.Id, ch)
	}
}
