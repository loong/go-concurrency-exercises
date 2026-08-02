//////////////////////////////////////////////////////////////////////
//
// Given is a numeric processing pipeline with three conceptual
// stages:
//
//  1. generator - produces a sequence of numbers
//  2. square    - squares each number
//  3. keepEven  - keeps only the even results
//
// generator, square, and keepEven are supposed to be independent
// pipeline stages, each running concurrently and connected by
// channels, but the naive Pipeline below does no such thing: it runs
// every stage to completion, one at a time, over the *entire* input
// materialized as a slice, before starting the next stage. Nothing
// overlaps - square doesn't start on item 1 until generator has
// finished producing every item, and keepEven doesn't start until
// square has finished squaring every item - and there is no way to
// stop early: even if the caller only ever wanted the first result,
// every single item still gets pushed through every single stage.
//
// Your task is to turn generator, square, and keepEven into true
// pipeline stages that operate on channels instead of slices, each
// running in its own goroutine, so that stage 2 can start working on
// item 1 while stage 1 is still producing item 2, and so on down the
// line. Every stage must also respect the done channel: as soon as
// done is closed, every stage still running must stop promptly
// instead of blocking forever on a send that nobody will ever receive
// (imagine an early-terminating caller that only reads the first
// result and moves on), and every stage must close its output channel
// once its input is exhausted so that downstream stages (and the
// caller) know there is nothing more coming.
//
// The new stage signatures must be:
//
//     func generator(done <-chan struct{}, nums ...int) <-chan int
//     func square(done <-chan struct{}, in <-chan int) <-chan int
//     func keepEven(done <-chan struct{}, in <-chan int) <-chan int
//
// Each of these spawns its own goroutine that reads from its input
// (or, for generator, from the literal nums argument), calls
// SimulateWork once per item processed, writes each result to a
// freshly created output channel, and closes that output channel when
// there is nothing left to send. Pipeline composes the three stages
// (generator -> square -> keepEven) and drains the final stage's
// channel into a slice to return. Pipeline's own signature must stay
// the same, since it's the one thing callers/tests depend on:
//
//     func Pipeline(done <-chan struct{}, nums ...int) []int
//

package main

import "fmt"

// generator is supposed to be a pipeline stage that emits nums, one
// at a time, on a channel. For now it just returns the entire input
// as a slice.
func generator(done <-chan struct{}, nums ...int) []int {
	return nums
}

// square is supposed to be a pipeline stage that reads numbers as
// they arrive and squares them concurrently with its upstream and
// downstream stages. For now it waits for the entire input slice and
// squares it one item at a time.
func square(done <-chan struct{}, nums []int) []int {
	out := make([]int, len(nums))
	for i, n := range nums {
		SimulateWork()
		out[i] = n * n
	}
	return out
}

// keepEven is supposed to be a pipeline stage that reads numbers as
// they arrive and filters out the odd ones concurrently with its
// upstream stage. For now it waits for the entire input slice and
// filters it one item at a time.
func keepEven(done <-chan struct{}, nums []int) []int {
	var out []int
	for _, n := range nums {
		SimulateWork()
		if n%2 == 0 {
			out = append(out, n)
		}
	}
	return out
}

// Pipeline runs nums through the generator -> square -> keepEven
// pipeline and returns the final result.
func Pipeline(done <-chan struct{}, nums ...int) []int {
	return keepEven(done, square(done, generator(done, nums...)))
}

func main() {
	done := make(chan struct{})
	defer close(done)

	result := Pipeline(done, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)

	fmt.Println(result)
}
