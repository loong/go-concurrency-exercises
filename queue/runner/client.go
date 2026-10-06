package runner

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"vu/benchmark/queue/tasks"
)

// ClientConfig collects options for pushing tasks into the queue server.
type ClientConfig struct {
	Addr        string
	Total       int
	Concurrency int
	Iterations  int
}

func RunClient(cfg ClientConfig) error {
	payload, err := json.Marshal(tasks.HashTaskInput{Iteration: cfg.Iterations})
	if err != nil {
		return err
	}

	var sent int64
	var completed int64

	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	var once sync.Once

	start := time.Now()

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			fmt.Printf("Starting goroutine %d\n", index+1)
			defer wg.Done()

			conn, err := net.Dial("tcp", cfg.Addr)
			if err != nil {
				recordError(errCh, &once, err)
				return
			}
			defer conn.Close()

			encoder := json.NewEncoder(conn)
			decoder := json.NewDecoder(conn)
			var lastTask int64
			for {
				if lastTask == 0 {
					lastTask = atomic.AddInt64(&sent, 1)
				}

				if lastTask > int64(cfg.Total) {
					return
				}

				fmt.Printf("Goroutine %d runs tasks %d\n", index+1, int(lastTask))

				task := tasks.Task{
					Id:    strconv.FormatInt(lastTask, 10),
					Type:  tasks.HashTaskType,
					Input: payload,
				}

				// ---- NEW: retry loop per tasks ----
				for retry := 0; retry < 5; retry++ {

					if err := encoder.Encode(task); err != nil {
						fmt.Printf("Goroutine %d: encode error %v — reconnecting\n", index+1, err)
						conn.Close()

						// reconnect
						var err2 error
						conn, err2 = net.Dial("tcp", cfg.Addr)
						if err2 != nil {
							fmt.Printf("Goroutine %d: reconnect failed %v\n", index+1, err2)
							time.Sleep(time.Second)
							continue
						}
						encoder = json.NewEncoder(conn)
						decoder = json.NewDecoder(conn)
						continue
					}

					var resp clientResponse
					if err := decoder.Decode(&resp); err != nil {
						fmt.Printf("Goroutine %d: decode error %v — reconnecting\n", index+1, err)
						conn.Close()

						// reconnect
						var err2 error
						conn, err2 = net.Dial("tcp", cfg.Addr)
						if err2 != nil {
							fmt.Printf("Goroutine %d: reconnect failed %v\n", index+1, err2)
							time.Sleep(time.Second)
							continue
						}
						encoder = json.NewEncoder(conn)
						decoder = json.NewDecoder(conn)
						continue
					}

					if resp.Error != "" {
						fmt.Printf("Goroutine %d: server error %s — retry\n", index+1, resp.Error)
						time.Sleep(200 * time.Millisecond)
						continue
					}

					// success
					atomic.AddInt64(&completed, 1)
					lastTask = 0
					break
				}
			}
		}(i)
	}
	wg.Wait()
	duration := time.Since(start)

	select {
	case err := <-errCh:
		return fmt.Errorf("benchmark aborted after %v: %w", duration, err)
	default:
		tput := float64(completed) / duration.Seconds()
		fmt.Printf("completed %d tasks in %v (throughput: %.2f tasks/sec)\n", completed, duration, tput)
		return nil
	}
}

// RunClient dials the server and pushes hash tasks, returning an error if the run aborts early.
//func RunClient(cfg ClientConfig) error {
//	payload, err := json.Marshal(tasks.HashTaskInput{Iteration: cfg.Iterations})
//	if err != nil {
//		return err
//	}
//
//	var sent int64
//	var completed int64
//
//	var wg sync.WaitGroup
//	errCh := make(chan error, 1)
//	var once sync.Once
//
//	start := time.Now()
//
//	for i := 0; i < cfg.Concurrency; i++ {
//		wg.Add(1)
//		go func(index int) {
//			fmt.Printf("Starting goroutine %d\n", index+1)
//			defer wg.Done()
//
//			conn, err := net.Dial("tcp", cfg.Addr)
//			if err != nil {
//				recordError(errCh, &once, err)
//				return
//			}
//			defer conn.Close()
//
//			encoder := json.NewEncoder(conn)
//			decoder := json.NewDecoder(conn)
//
//			var lastTask int64
//			for {
//				if lastTask == 0 {
//					lastTask = atomic.AddInt64(&sent, 1)
//				}
//
//				if lastTask > int64(cfg.Total) {
//					return
//				}
//
//				fmt.Printf("Goroutine %d runs tasks %d\n", index+1, int(lastTask))
//
//				tasks := tasks.Task{
//					Id:    strconv.FormatInt(lastTask, 10),
//					Type:  tasks.HashTaskType,
//					Input: payload,
//				}
//
//				if err := encoder.Encode(tasks); err != nil {
//					fmt.Printf("Goroutine %d, met encoder.Encode error %v\n", index+1, err)
//					recordError(errCh, &once, err)
//					return
//				}
//
//				var resp clientResponse
//				if err := decoder.Decode(&resp); err != nil {
//					fmt.Printf("Goroutine %d, met decoder.Decode error %v\n", index+1, err)
//					recordError(errCh, &once, err)
//					return
//				}
//				if resp.Error != "" {
//					fmt.Printf("Error received: %s. Retry after 2s\n", resp.Error)
//					recordError(errCh, &once, errors.New(resp.Error))
//					time.Sleep(2 * time.Second)
//					continue // retry
//				}
//
//				lastTask = 0
//				atomic.AddInt64(&completed, 1)
//			}
//		}(i)
//	}
//
//	wg.Wait()
//	duration := time.Since(start)
//
//	select {
//	case err := <-errCh:
//		return fmt.Errorf("benchmark aborted after %v: %w", duration, err)
//	default:
//		tput := float64(completed) / duration.Seconds()
//		fmt.Printf("completed %d tasks in %v (throughput: %.2f tasks/sec)\n", completed, duration, tput)
//		return nil
//	}
//}

type clientResponse struct {
	ID     string `json:"id"`
	Result []byte `json:"result"`
	Error  string `json:"error"`
}

func recordError(ch chan<- error, once *sync.Once, err error) {
	once.Do(func() {
		ch <- err
	})
}
