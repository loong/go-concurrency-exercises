package main

import (
	"flag"
	"fmt"
	"os"
	"vu/benchmark/queue/runner"
)

//go run main.go -mode=server -workers=4 -capacity=10 -addr=:8082

func main() {
	mode := flag.String("mode", "server", "choose server or client mode")
	addr := flag.String("addr", ":8080", "tcp listen address")

	// Server options.
	capacity := flag.Int("capacity", 100, "queue capacity")
	workers := flag.Int("workers", 8, "number of worker goroutines")

	// Client options.
	total := flag.Int("total", 1000, "total tasks to run")
	concurrency := flag.Int("concurrency", 8, "concurrent client workers")
	iterations := flag.Int("iterations", 100000, "hash iterations per tasks")

	flag.Parse()

	switch *mode {
	case "server":
		err := runner.RunServer(runner.ServerConfig{
			Addr:     *addr,
			Capacity: *capacity,
			Workers:  *workers,
		})
		if err != nil {
			os.Exit(1)
		}
	case "client":
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
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q (expected server or client)\n", *mode)
		os.Exit(1)
	}
}
