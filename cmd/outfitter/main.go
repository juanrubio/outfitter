package main

import (
	"fmt"
	"os"

	"github.com/juanrubio/outfitter/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "outfitter:", err)
		os.Exit(cli.ExitCode(err))
	}
}
