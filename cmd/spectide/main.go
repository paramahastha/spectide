package main

import (
	"fmt"
	"github.com/spectide/spectide/internal/cli"
	"os"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(cli.ExitCode(err))
	}
}
