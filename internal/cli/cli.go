// Package cli implements memlint's command line: argument parsing, exit codes,
// and output selection.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/frankbesch/memlint/internal/config"
	"github.com/frankbesch/memlint/internal/lint"
	"github.com/frankbesch/memlint/internal/report"
	"golang.org/x/term"
)

// version is injected by release builds via
// -ldflags "-X github.com/frankbesch/memlint/internal/cli.version=vX.Y.Z".
// The symbol name is part of the release contract: .goreleaser.yaml points at
// it, and a test builds with -X to pin it.
var version = ""

// Version resolves what "memlint --version" reports: the injected release
// version, else the module version recorded by `go install`, else "dev".
func Version() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

// Exit codes. These are the tool's contract with CI.
const (
	// ExitClean means no RED findings (and, under --strict, no YELLOW either).
	ExitClean = 0
	// ExitFindings means an invariant was violated or could not be verified.
	ExitFindings = 1
	// ExitUsage means memlint could not start: bad arguments, or a missing or
	// invalid config. It never overlaps with a real result.
	ExitUsage = 2
)

const usageText = `memlint - invariant checker for file-based agent memory

Usage:
  memlint check [flags] [path]
  memlint init [path]
  memlint --version

check evaluates the invariants declared in <path>/.memlint.toml. Path defaults
to ".". check is read-only: it reports drift and never repairs it.

init inspects the repository and writes a commented starter .memlint.toml,
enabling only rules it found evidence for. It refuses to overwrite an existing
config, and it is the only memlint command that writes a file.

Flags (must precede [path]):
  --strict             treat YELLOW findings as failures
  --base <ref>         compare [append_only] files against <ref> instead of
                       HEAD (e.g. origin/main in pull-request CI)
  --format text|json|github
                       output format (default "text"); "github" emits
                       GitHub Actions annotations
  --no-color           disable ANSI color (also honors NO_COLOR)
  -h, --help           show this help

Exit codes:
  0  no RED findings
  1  RED findings, or YELLOW findings with --strict
  2  usage error, or missing/invalid .memlint.toml
`

// Main runs memlint and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "memlint: no command given")
		fmt.Fprint(stderr, usageText)
		return ExitUsage
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usageText)
		return ExitClean
	case "--version":
		fmt.Fprintln(stdout, "memlint "+Version())
		return ExitClean
	default:
		fmt.Fprintf(stderr, "memlint: unknown command %q\n", args[0])
		fmt.Fprint(stderr, usageText)
		return ExitUsage
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usageText) }

	strict := fs.Bool("strict", false, "treat YELLOW findings as failures")
	format := fs.String("format", "text", `output format: "text", "json", or "github"`)
	noColor := fs.Bool("no-color", false, "disable ANSI color")
	base := fs.String("base", "", "compare [append_only] files against this git ref instead of HEAD")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprint(stdout, usageText)
			return ExitClean
		}
		fmt.Fprint(stderr, usageText)
		return ExitUsage
	}

	// Flags must precede the path, so anything past the first positional is a
	// mistake -- most likely a flag written after it, which flag.Parse would
	// otherwise ignore in silence.
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "memlint: unexpected argument %q (flags must precede the path)\n", fs.Arg(1))
		fmt.Fprint(stderr, usageText)
		return ExitUsage
	}
	if *format != "text" && *format != "json" && *format != "github" {
		fmt.Fprintf(stderr, "memlint: unknown format %q (want \"text\", \"json\", or \"github\")\n", *format)
		return ExitUsage
	}

	root := "."
	if fs.NArg() == 1 {
		root = fs.Arg(0)
	}

	// Startup checks. Anything wrong here is exit 2: memlint has not evaluated a
	// single invariant yet, so reporting a finding would misrepresent the run.
	info, err := os.Stat(root)
	if err != nil {
		fmt.Fprintf(stderr, "memlint: cannot read target %s: %v\n", root, err)
		return ExitUsage
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "memlint: target %s is not a directory\n", root)
		return ExitUsage
	}
	cfg, err := config.Load(root)
	if err != nil {
		fmt.Fprintf(stderr, "memlint: %v\n", err)
		return ExitUsage
	}
	if *base != "" {
		// --base is an explicit demand for a baseline. A demand that cannot be
		// honored — or that nothing consumes — must refuse, not silently pass.
		if cfg.AppendOnly == nil {
			fmt.Fprintf(stderr, "memlint: --base has no effect: [append_only] is not enabled in %s\n",
				filepath.Join(root, config.FileName))
			return ExitUsage
		}
		if err := lint.ValidateBaseRef(root, *base); err != nil {
			fmt.Fprintf(stderr, "memlint: %v\n", err)
			return ExitUsage
		}
		cfg.AppendOnly.BaseRef = *base
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(stderr, "memlint: cannot resolve target %s: %v\n", root, err)
		return ExitUsage
	}

	res := lint.Run(absRoot, cfg)

	switch *format {
	case "json":
		err = report.JSON(stdout, res)
	case "github":
		err = report.GitHub(stdout, res)
	default:
		err = report.Text(stdout, res, useColor(*noColor, *format, stdout))
	}
	if err != nil {
		fmt.Fprintf(stderr, "memlint: writing output: %v\n", err)
		return ExitUsage
	}

	if res.Red() > 0 || (*strict && res.Yellow() > 0) {
		return ExitFindings
	}
	return ExitClean
}

// useColor decides whether to emit ANSI codes. JSON is never colored, and
// color is suppressed whenever stdout is not an interactive terminal so piped
// or redirected output stays clean.
func useColor(noColor bool, format string, stdout io.Writer) bool {
	if noColor || format == "json" {
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	f, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
