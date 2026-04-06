package main

import (
	"fmt"
	"os"

	"github.com/lauritsk/hatchctl/internal/bridge"
)

func main() {
	if err := bridge.HelperMain(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
