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
	"syscall"
)

func main() {
	// Create a process
	proc := MockProcess{}
	done := make(chan bool)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)

	// Run the process (blocking)
	go proc.Run()
	<-sig
	go func() {
		proc.Stop()
		done <- true
	}()
	// stop never actually returns from the mockprocess, so done never gets sent a signal - this handles the case of a process actually stopping
	select {
	case <-sig:
		fmt.Println("aborting clean shutdown...")
	case <-done:
		fmt.Println("exiting cleanly...")
	}
}
