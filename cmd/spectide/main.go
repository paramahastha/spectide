package main

import (
	"fmt"
	"github.com/spectide/spectide/internal/cli"
	"github.com/spectide/spectide/internal/security"
	"os"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", security.Redact(err.Error()))
		os.Exit(cli.ExitCode(err))
	}
}
