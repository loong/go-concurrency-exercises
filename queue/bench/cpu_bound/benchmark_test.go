package cpu_bound

import (
	"encoding/json"
	"github.com/pkg/profile"
	"strconv"
	"testing"
	"time"

	"vu/benchmark/queue/internal"
	"vu/benchmark/queue/tasks"
)

func TestBench(t *testing.T) {
	defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	const iterations = 3_000_000_000

	poolSize := 8
	capacity := 1_000

	payload, err := json.Marshal(tasks.BurnCPUTaskInput{Iteration: iterations})
	if err != nil {
		t.Fatalf("marshal hash input: %v", err)
	}

	queue := internal.NewQueue(capacity, poolSize, true)
	pending := make(chan (<-chan internal.Output), capacity)
	for i := 0; i < 5; i++ {
		go func() {
			for ch := range pending {
				out := <-ch

				if out.Err != nil {
					t.Logf("task failed: %v", out.Err)
				} else {
					//t.Logf("task ok: %s", out.Res)
				}
			}
		}()
	}

	i := 0
	for i < 100 {
		task := tasks.Task{
			Id:    strconv.Itoa(i),
			Type:  tasks.BurnCPUTaskType,
			Input: payload,
		}

		ch, err := queue.Put(&task)
		if err != nil {
			//t.Logf("Error when put task %d: %v", i, err)
			time.Sleep(2 * time.Second)
			continue
		}

		//t.Logf("put task %d", i)
		pending <- ch
		i++
	}

	queue.Shutdown()
	close(pending)
	time.Sleep(1 * time.Second)
}

// Benchmark queue throughput for the hash task using a fixed iteration count.
func BenchmarkQueueHashFixedIterations(b *testing.B) {
	defer profile.Start(profile.TraceProfile, profile.ProfilePath(".")).Stop()
	const iterations = 3_000_000_000

	poolSize := 8
	capacity := 1_000

	payload, err := json.Marshal(tasks.BurnCPUTaskInput{Iteration: iterations})
	if err != nil {
		b.Fatalf("marshal hash input: %v", err)
	}

	queue := internal.NewQueue(capacity, poolSize, true)
	pending := make(chan (<-chan internal.Output), capacity)
	for i := 0; i < 5; i++ {
		go func() {
			for ch := range pending {
				out := <-ch

				if out.Err != nil {
					b.Logf("task failed: %v", out.Err)
				} else {
					//b.Logf("task ok: %s", out.Res)
				}
			}
		}()
	}

	i := 0
	b.ResetTimer()
	for i < b.N {
		task := tasks.Task{
			Id:    strconv.Itoa(i),
			Type:  tasks.BurnCPUTaskType,
			Input: payload,
		}

		ch, err := queue.Put(&task)
		if err != nil {
			//b.Logf("Error when put task %d: %v", i, err)
			time.Sleep(2 * time.Second)
			continue
		}

		//b.Logf("put task %d", i)
		pending <- ch
		i++
	}

	queue.Shutdown()
	b.StopTimer()
	close(pending)
	time.Sleep(1 * time.Second)
}
