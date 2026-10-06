package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"vu/benchmark/queue/tasks"
)

type IQueue interface {
	Put(task *tasks.Task) (<-chan Output, error)
	Shutdown() error
}

type _queue struct {
	capacity    int
	channel     chan _taskWrapper
	poolSize    int
	closed      bool
	mutex       sync.Mutex
	wg          sync.WaitGroup
	size        int
	logDisabled bool
}

type _taskWrapper struct {
	task    *tasks.Task
	channel chan Output
}

type Output struct {
	Err error
	Res []byte
}

func (q *_queue) Put(task *tasks.Task) (<-chan Output, error) {
	q.mutex.Lock()
	if q.size+1 > q.capacity {
		q.mutex.Unlock()
		return nil, errors.New("queue is full")
	}

	if q.closed {
		q.mutex.Unlock()
		return nil, errors.New("queue is closed")
	}

	defer q.mutex.Unlock()

	q.wg.Add(1)
	q.size += 1

	channel := make(chan Output, 1)

	q.channel <- _taskWrapper{
		task:    task,
		channel: channel,
	}
	return channel, nil
}

func (q *_queue) Shutdown() error {
	q.mutex.Lock()
	q.closed = true
	q.mutex.Unlock()

	q.wg.Wait()
	return nil
}

func (q *_queue) init() {
	// Avoid spamming stdout when running benchmarks so measurements stay clean.
	logWorkers := q.shouldLogWorker()

	for i := 0; i < q.poolSize; i++ {
		workerID := i + 1
		go func(id int) {
			for task := range q.channel {
				if logWorkers {
					fmt.Printf("Worker %d, pick up tasks %s\n", id, task.task.Id)
				}
				res, err := Execute(task)
				task.channel <- Output{Res: res, Err: err}
				close(task.channel)

				q.mutex.Lock()
				q.size--
				q.mutex.Unlock()
				q.wg.Done()
			}
		}(workerID)
	}
}

func NewQueue(capacity int, poolSize int, logDisabled bool) IQueue {
	channel := make(chan _taskWrapper, capacity)

	queue := &_queue{
		capacity:    capacity,
		channel:     channel,
		poolSize:    poolSize,
		logDisabled: logDisabled,
	}

	queue.init()

	return queue
}

func Execute(task _taskWrapper) ([]byte, error) {
	switch task.task.Type {
	case tasks.SumTaskType:
		return tasks.SumTask(task.task.Input)
	case tasks.HashTaskType:
		input := tasks.HashTaskInput{}
		if err := json.Unmarshal(task.task.Input, &input); err != nil {
			return nil, err
		}
		return tasks.HashTask(input.Iteration), nil
	case tasks.BurnCPUTaskType:
		return tasks.BurnCPUTask(task.task.Input)
	default:
		{
			return nil, errors.New("invalid tasks type")
		}
	}
}

// shouldLogWorker reports whether worker-level logging is enabled.
// Benchmarks pass -test.bench which adds the test.bench flag; if it is
// non-empty we suppress logs to keep benchmark output clean.
func (q *_queue) shouldLogWorker() bool {
	//benchFlag := flag.Lookup("test.bench")
	//if benchFlag != nil || q.logDisabled {
	//	return false
	//}

	if q.logDisabled {
		return false
	}

	return true
}
