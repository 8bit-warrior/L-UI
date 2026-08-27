package main

import (
	"fmt"
	"os"

	"github.com/8bit-warrior/L-UI/internal/app"
)

var version = "dev"
var commit = "unknown"
var buildDate = "unknown"

func main() {
	app.Version = version
	app.Commit = commit
	app.BuildDate = buildDate
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
