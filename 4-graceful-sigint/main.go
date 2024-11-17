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
	"time"
)

func main() {

	sigsStop := make(chan os.Signal, 1)

	done := make(chan bool, 1)

	signal.Notify(sigsStop, syscall.SIGINT, syscall.SIGTERM)

	// Create a process
	proc := MockProcess{}

	go func() {
		sigStop := <-sigsStop
		fmt.Println("\nПолучен сигнал:", sigStop)
		go func() {
			proc.Stop()
		}()
		fmt.Println("Выполняем корректное завершение работы...")
		time.Sleep(2 * time.Second) // имитация завершения задач

		done <- true

	}()

	go func() {
		<-done
		fmt.Println("\nПрограмма завершена.")
	}()
	// Run the process (blocking)
	fmt.Println("Программа запущена. Нажмите Ctrl+C для завершения.")
	proc.Run()

}
