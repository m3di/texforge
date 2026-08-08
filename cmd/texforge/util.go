package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// isTerminal reports whether f is an interactive terminal, so we only colorize
// when a human is watching. Uses the char-device bit — no external term deps.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func contextWithSignals() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx, cancel
}
