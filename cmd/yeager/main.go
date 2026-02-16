package main

import (
	"os"

	"github.com/gridlhq/yeager/internal/cli"
)

// Set via ldflags at build time.
var version = "dev"

func main() {
	code := cli.Execute(version, os.Args[1:])
	os.Exit(code)
}
