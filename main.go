// Command memlint checks mechanical invariants in file-based agent memory
// repositories. It is read-only: it never modifies, creates, or repairs files.
package main

import (
	"os"

	"github.com/frankbesch/memlint/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
