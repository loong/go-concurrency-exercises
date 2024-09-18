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
	"fmt"
	"os"
	"os/signal"
)

func main() {
	// Create a process
	proc := MockProcess{}

	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt)

	var count int
	go func() {
		for {
			select {
			case sig := <-sigs:
				count++
				if count == 2 {
					fmt.Println(sig)
					os.Exit(0)
				}
				go proc.Stop()
			}
		}
	}()

	// Run the process (blocking)
	proc.Run()
}
