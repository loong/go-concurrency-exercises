package main

import (
	"flag"
	"fmt"
	"os"
	"vu/benchmark/queue/runner"
)

func main() {
	addr := flag.String("addr", ":8080", "queue server address")
	total := flag.Int("total", 1000, "total tasks to run")
	concurrency := flag.Int("concurrency", 8, "concurrent client workers")
	iterations := flag.Int("iterations", 100000, "hash iterations per tasks")
	flag.Parse()

	err := runner.RunClient(runner.ClientConfig{
		Addr:        *addr,
		Total:       *total,
		Concurrency: *concurrency,
		Iterations:  *iterations,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
