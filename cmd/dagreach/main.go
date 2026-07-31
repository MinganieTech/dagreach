// Command dagreach is a portable change-impact analyser for dependency graphs.
package main

import (
	"os"

	"github.com/MinganieTech/dagreach/internal/dagreach"
)

func main() {
	os.Exit(dagreach.Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}
