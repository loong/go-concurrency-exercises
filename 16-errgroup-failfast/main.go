//////////////////////////////////////////////////////////////////////
//
// Group is meant to be your own tiny version of
// golang.org/x/sync/errgroup's Group: a way to launch any number of
// independent tasks via Go, let every one of them run to completion
// concurrently regardless of whether others fail, and then have Wait
// return the FIRST error encountered (if any) - safely, even though
// multiple tasks might fail around the same time.
//
// Right now Group does none of that. Go just runs the function
// immediately, right on the calling goroutine, blocking until it
// returns - so tasks run strictly one after another instead of
// concurrently. The part where Wait "remembers" the first error is
// fine as far as it goes, but the naive plain field is not protected
// against concurrent access at all, even though real usage always
// calls Go from multiple goroutines racing against each other and
// against Wait.
//
// Your task is to fix Group so that:
//
//   - Go(f func() error) launches f in its OWN goroutine immediately
//     and returns without blocking, tracking the goroutine via an
//     internal sync.WaitGroup.
//   - Wait() error blocks until every goroutine launched via Go has
//     finished, and returns the FIRST error encountered - captured
//     safely from concurrently running goroutines (e.g. via
//     sync.Once or a mutex) even if several tasks fail around the
//     same time, without introducing a data race.
//
// (No context/cancellation support needed - keep the scope to the
// plain Go/Wait pair, matching real errgroup's base Group without
// WithContext.)
//
// The signature must stay the same:
//
//     func (g *Group) Go(f func() error)
//     func (g *Group) Wait() error
//

package main

import (
	"errors"
	"fmt"
	"time"
)

// Group runs a collection of tasks, started via Go, and collects the
// first error (if any) returned by them once every task has finished.
type Group struct {
	firstErr error
}

// Go is supposed to run f concurrently with any other task previously
// or subsequently started via Go. Right now it just calls f directly
// on the calling goroutine, so tasks run one at a time instead of
// concurrently.
func (g *Group) Go(f func() error) {
	if err := f(); err != nil && g.firstErr == nil {
		g.firstErr = err
	}
}

// Wait is supposed to block until every task started via Go has
// finished, and return the first error encountered, if any. Since Go
// currently blocks until the task is done, Wait "works" today only by
// accident - there is nothing left to wait for.
func (g *Group) Wait() error {
	return g.firstErr
}

func main() {
	var g Group

	tasks := []error{
		nil,
		nil,
		errors.New("task 2 failed"),
		nil,
		errors.New("task 4 failed"),
	}

	start := time.Now()

	for i, taskErr := range tasks {
		i, taskErr := i, taskErr
		g.Go(func() error {
			time.Sleep(100 * time.Millisecond)
			if taskErr != nil {
				return fmt.Errorf("task %d: %w", i, taskErr)
			}
			return nil
		})
	}

	err := g.Wait()
	elapsed := time.Since(start)

	fmt.Printf("Wait() returned: %v\n", err)
	fmt.Printf("Elapsed: %s\n", elapsed)
}
