package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
}
