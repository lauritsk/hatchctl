package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/lauritsk/hatchctl/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version information")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	fmt.Fprintln(os.Stdout, "hatchctl")
}
