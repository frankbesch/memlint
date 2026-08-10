// Command memlint checks mechanical invariants in file-based agent memory
// repositories. check is read-only: it never modifies, creates, or repairs
// files. The one exception in the whole tool is init, which creates a starter
// .memlint.toml and refuses to overwrite an existing one.
package main

import (
	"os"

	"github.com/frankbesch/memlint/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
