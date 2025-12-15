package runner

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"vu/benchmark/queue/internal"
	"vu/benchmark/queue/server"
)

// ServerConfig collects the tunables for running the queue server.
type ServerConfig struct {
	Addr     string
	Capacity int
	Workers  int
}

// RunServer starts the TCP server and blocks until shutdown.
func RunServer(cfg ServerConfig) error {
	queue := internal.NewQueue(cfg.Capacity, cfg.Workers, false)

	done := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		close(done)
		fmt.Println("signal received, shutting down")
	}()

	fmt.Printf("Queue server listening on %s\n", cfg.Addr)
	err := server.Serve(cfg.Addr, queue, done)
	if err != nil {
		fmt.Println("server error:", err)
	}

	queue.Shutdown()
	fmt.Println("queue drained, server exiting")
	return err
}
