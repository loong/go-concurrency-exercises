//////////////////////////////////////////////////////////////////////
//
// Given is a mock process which runs indefinitely and blocks the
// program. Right now the only way to stop the program is to send a
// SIGINT (Ctrl-C). Killing a process like that is not graceful, so we
// want to try to gracefully stop the process first.
//
// Change the program to do the following:
//   1. On SIGINT try to gracefully stop the process using
//          `proc.Stop()`
//   2. If SIGINT is called again, just kill the program (last resort)
//

package main

import (
	"log"
	"os"
	"os/signal"
	"time"
)

func main() {
	// Create a process
	proc := MockProcess{}

	// Run the process (blocking)
	go proc.Run()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)

	sig := <-c
	log.Printf("captured sigint %v, stopping application and exiting...", sig)
	go proc.Stop()

	// Wait for either graceful stop or second SIGINT
	select {
	case <-time.After(10 * time.Second):
		log.Println("Graceful shutdown completed.")
	case sig := <-c:
		log.Printf("Captured %v again, forcing shutdown...", sig)
	}

	os.Exit(0)

}
