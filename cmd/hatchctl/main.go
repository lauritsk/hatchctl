package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/lauritsk/hatchctl/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	app := cli.New(stdout, stderr)
	if err := app.Run(ctx, args); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
