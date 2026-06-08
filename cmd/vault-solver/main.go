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

// run executes the root command and returns the process exit code. It exists so signal cleanup
// (defer stop()) runs before the process exits — main calls os.Exit(run()) only after run returns.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
}
