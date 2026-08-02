//////////////////////////////////////////////////////////////////////
//
// Five philosophers sit around a round table. Between each adjacent
// pair of philosophers sits exactly one fork, so there are as many
// forks as philosophers. To eat, a philosopher needs to pick up BOTH
// the fork to their left and the fork to their right - only then can
// they eat, after which they put both forks back down so their
// neighbors can use them.
//
// Dine is supposed to let every philosopher finish all of their meals
// without the table ever grinding to a halt. But the naive
// implementation below always acquires forks in the same order for
// every philosopher - left fork first, then right fork - and that is
// exactly the textbook setup for deadlock: if every philosopher picks
// up their left fork at roughly the same time, every single one of
// them ends up holding one fork while waiting forever for their right
// fork, which is being held by their neighbor who is, in turn,
// waiting for THEM to release the fork they're holding. It's a
// perfect circle of waiting, so nobody ever eats again.
//
// Your task is to fix Dine so it can never deadlock, using a
// standard, well-known deadlock-avoidance strategy. The simplest and
// most idiomatic fix here is resource ordering: change fork
// acquisition so every philosopher always locks the LOWER-INDEXED
// fork of their two forks first, regardless of which one happens to
// be their "left" fork and which their "right" - e.g.
//
//     first, second := p.leftFork, p.rightFork
//     if second.index < first.index {
//         first, second = second, first
//     }
//
// Because every philosopher now agrees on a single global order for
// acquiring any two forks, the circular-wait condition that causes
// the deadlock can never arise: there's always at least one
// philosopher (the one sitting between the two highest-indexed forks)
// who reaches for the same fork first as their neighbor, so someone is
// always able to make progress.
//
// (An equally valid alternative fix is an arbitrator/semaphore that
// only allows numPhilosophers-1 philosophers to attempt to pick up
// forks at once, which also breaks the circular wait - either
// approach is acceptable, but resource ordering is simplest to
// implement here.)
//
// Keep the function signature identical so it remains a drop-in
// replacement for the naive version below:
//
//     func Dine(numPhilosophers, mealsToEat int) (totalMealsEaten int32)
//

package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Fork is a single shared utensil that only one philosopher can hold
// at a time.
type Fork struct {
	mu    sync.Mutex
	index int
}

// Philosopher sits at the table with a fork on each side and eats
// mealsToEat times, each time picking up its left fork, then its
// right fork, eating (a brief simulated delay), then putting both
// forks back down.
type Philosopher struct {
	id                  int
	leftFork, rightFork *Fork
	mealsToEat          int
}

// Dine picks up both of the philosopher's forks (always left first,
// then right), eats mealsToEat times, and records each meal in
// mealsEaten.
func (p *Philosopher) Dine(wg *sync.WaitGroup, mealsEaten *int32) {
	defer wg.Done()

	for i := 0; i < p.mealsToEat; i++ {
		p.leftFork.mu.Lock()
		// A brief pause while "reaching" for the right fork. In
		// practice this is exactly how the classic deadlock actually
		// manifests: it gives every other philosopher's goroutine
		// time to also grab their own left fork before any of them
		// moves on to try for their right one, so the circular wait
		// forms reliably instead of only occasionally.
		time.Sleep(time.Millisecond)
		p.rightFork.mu.Lock()

		// eat
		time.Sleep(time.Millisecond)
		atomic.AddInt32(mealsEaten, 1)

		p.rightFork.mu.Unlock()
		p.leftFork.mu.Unlock()
	}
}

// Dine seats numPhilosophers at a round table, each needing
// mealsToEat meals, and waits for them all to finish. It returns the
// total number of meals eaten across every philosopher, which should
// always equal numPhilosophers * mealsToEat.
func Dine(numPhilosophers, mealsToEat int) (totalMealsEaten int32) {
	forks := make([]*Fork, numPhilosophers)
	for i := range forks {
		forks[i] = &Fork{index: i}
	}

	var wg sync.WaitGroup
	var mealsEaten int32
	for i := 0; i < numPhilosophers; i++ {
		left := forks[i]
		right := forks[(i+1)%numPhilosophers]
		p := &Philosopher{id: i, leftFork: left, rightFork: right, mealsToEat: mealsToEat}
		wg.Add(1)
		go p.Dine(&wg, &mealsEaten)
	}
	wg.Wait()

	return mealsEaten
}

func main() {
	// NOTE: because of the fork-ordering bug described above, this
	// call will, in practice, hang forever essentially every time you
	// run it directly with `go run .` - that's the bug this exercise
	// is about. The graded artifact is the test suite (check_test.go),
	// which guards the naive run with a timeout instead of waiting on
	// it forever.
	total := Dine(5, 3)
	fmt.Printf("philosophers ate a total of %d meals\n", total)
}
