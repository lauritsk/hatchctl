package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/lauritsk/hatchctl/internal/cli"
)

func main() {
	app := cli.New(os.Stdout, os.Stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
