package main

import (
	"os"

	"github.com/aleksey925/agentbox/internal/cli"
)

var version = "dev"

func main() {
	code := cli.Run(os.Args[1:], version)
	os.Exit(code)
}
