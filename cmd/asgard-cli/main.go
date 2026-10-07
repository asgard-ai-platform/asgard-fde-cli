package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.NewRootCmd().ExecuteContext(ctx); err != nil {
		// Ctrl-C is not an error; exit quietly.
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		// A command that has already printed its whole answer as JSON fails
		// without a second, unparseable account of it on stderr.
		if errors.Is(err, cli.ErrSilent) {
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
