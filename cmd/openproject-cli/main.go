package main

import (
	"os"

	"github.com/daniele-raiteri/openproject-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run())
}
