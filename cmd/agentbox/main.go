package main

import (
	"os"

	"github.com/aleksey925/agentbox/internal/cli"
)

var version = "0.0.0"

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.NewBuildInfo(version)))
}
