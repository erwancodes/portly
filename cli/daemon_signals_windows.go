//go:build windows

package main

import (
	"os"
	"os/signal"
)

func installSignalNotify(ch chan<- os.Signal) {
	// Windows exposes Ctrl+C as os.Interrupt. SIGTERM is not a catchable
	// console signal, so shutdown is also handled by the process controller.
	signal.Notify(ch, os.Interrupt)
}
