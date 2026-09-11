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
	"strings"

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

const topUsage = `memlint - integrity checks for file-based AI agent state

Usage:
  memlint <command> [flags] [path]
  memlint --version

Commands:
  check         verify the invariants declared in <path>/.memlint.toml
  init          write a starter .memlint.toml from what the repo shows
  fingerprint   print the fingerprint of the tree check would judge

Run "memlint <command> --help" for that command's flags. Flags may come
before or after the path. Docs: https://github.com/frankbesch/memlint
`

const checkUsage = `memlint check [flags] [path]

Evaluates the invariants declared in <path>/.memlint.toml (path defaults
to "."). Read-only: it reports drift and never repairs it. Flags may come
before or after the path.

Flags:
  --strict             treat YELLOW findings as failures
  --base <ref>         compare [append_only] files against <ref> instead of
                       HEAD (e.g. the pull request's base sha in CI)
  --changed            report only findings that touch files changed since
                       HEAD (modified, staged, or untracked); needs git
  --expect-tree <fp>   RED tree/moved unless the tree's fingerprint starts
                       with <fp> (full or >=12 hex): the receipt is stale
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

const initUsage = `memlint init [flags] [path]

Inspects the repository at <path> (default ".") and writes a starter
.memlint.toml. Rules are enabled on evidence only: an index file such as
MEMORY.md turns on [pointers], observed .DS_Store or *.tmp files turn on
[junk]. Rules with plausible but unconfirmed evidence are written as
commented sections; everything else is left out and listed as not
inferred. init will never overwrite an existing .memlint.toml, and it is
the only memlint command that writes a file.

Flags:
  --dry-run            print the config that would be written to stdout
                       and the report to stderr; write nothing. Works
                       when a .memlint.toml already exists.
  -h, --help           show this help
`

const fingerprintUsage = `memlint fingerprint [path]

Prints the SHA-256 fingerprint of the tree check would judge at <path>
(default "."): every visible regular file's path, size, and content hash,
sorted by path. Content-based, so a clone or a touch does not move it and
one changed byte does. Visible means what a commit could contain when git
is present; without git, every file under the root except .git/.

Every check summary ends with the first 12 hex of this value. Hold a later
run to it with:

  memlint check --expect-tree <fp> [path]

which turns a tree that changed in between into one RED tree/moved.

Flags:
  -h, --help           show this help
`

// Main runs memlint and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "memlint: no command given")
		fmt.Fprint(stderr, topUsage)
		return ExitUsage
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "fingerprint":
		return runFingerprint(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "help":
		// "memlint help <command>" is an alias for "memlint <command> --help".
		if len(args) > 1 {
			if u, ok := commandUsage[args[1]]; ok {
				fmt.Fprint(stdout, u)
				return ExitClean
			}
			fmt.Fprintf(stderr, "memlint: unknown command %q\n", args[1])
			fmt.Fprint(stderr, topUsage)
			return ExitUsage
		}
		fmt.Fprint(stdout, topUsage)
		return ExitClean
	case "-h", "--help":
		fmt.Fprint(stdout, topUsage)
		return ExitClean
	case "--version":
		fmt.Fprintln(stdout, "memlint "+Version())
		return ExitClean
	default:
		fmt.Fprintf(stderr, "memlint: unknown command %q\n", args[0])
		fmt.Fprint(stderr, topUsage)
		return ExitUsage
	}
}

var commandUsage = map[string]string{
	"check":       checkUsage,
	"init":        initUsage,
	"fingerprint": fingerprintUsage,
}

// parseCommand is the argument pre-pass every command shares. It walks args
// once, keeps recognized flags (with their values) in order, and collects
// the rest as positionals, so a flag is honored whether it comes before or
// after the path. What it refuses, it refuses loudly: an unknown flag
// anywhere, a value-taking flag with no value, or more than one positional
// is a usage error on stderr with that command's help, never a silent
// ignore. "--" ends flag parsing. It returns the single positional (or "")
// and, when done is true, the exit code the command should return at once.
func parseCommand(fs *flag.FlagSet, args []string, usage string, stdout, stderr io.Writer) (root string, code int, done bool) {
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positionals = append(positionals, args[i+1:]...)
			i = len(args)
		case a == "-h" || a == "--help" || a == "-help":
			fmt.Fprint(stdout, usage)
			return "", ExitClean, true
		case len(a) > 1 && a[0] == '-' && a != "-":
			name := strings.TrimLeft(a, "-")
			value, hasValue := "", false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name, value, hasValue = name[:eq], name[eq+1:], true
			}
			f := fs.Lookup(name)
			if f == nil {
				fmt.Fprintf(stderr, "memlint: unknown flag %q\n", a)
				fmt.Fprint(stderr, usage)
				return "", ExitUsage, true
			}
			if isBoolFlag(f) {
				flags = append(flags, a)
				continue
			}
			if hasValue {
				flags = append(flags, "-"+name+"="+value)
				continue
			}
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "memlint: flag %s needs a value\n", a)
				fmt.Fprint(stderr, usage)
				return "", ExitUsage, true
			}
			flags = append(flags, "-"+name, args[i+1])
			i++
		default:
			positionals = append(positionals, a)
		}
	}
	if err := fs.Parse(flags); err != nil {
		fmt.Fprintf(stderr, "memlint: %v\n", err)
		fmt.Fprint(stderr, usage)
		return "", ExitUsage, true
	}
	if len(positionals) > 1 {
		fmt.Fprintf(stderr, "memlint: unexpected argument %q (one path at most)\n", positionals[1])
		fmt.Fprint(stderr, usage)
		return "", ExitUsage, true
	}
	if len(positionals) == 1 {
		return positionals[0], 0, false
	}
	return ".", 0, false
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	strict := fs.Bool("strict", false, "treat YELLOW findings as failures")
	format := fs.String("format", "text", `output format: "text", "json", or "github"`)
	noColor := fs.Bool("no-color", false, "disable ANSI color")
	base := fs.String("base", "", "compare [append_only] files against this git ref instead of HEAD")
	changed := fs.Bool("changed", false, "report only findings touching files changed since HEAD")
	expectTree := fs.String("expect-tree", "", "fail with tree/moved unless the tree fingerprint starts with this")

	root, code, done := parseCommand(fs, args, checkUsage, stdout, stderr)
	if done {
		return code
	}
	if *format != "text" && *format != "json" && *format != "github" {
		fmt.Fprintf(stderr, "memlint: unknown format %q (want \"text\", \"json\", or \"github\")\n", *format)
		return ExitUsage
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

	var changedSet map[string]bool
	if *changed {
		// Like --base: an explicit narrowing that cannot be honored must
		// refuse, not silently widen back to a full run.
		changedSet, err = lint.ChangedFiles(absRoot)
		if err != nil {
			fmt.Fprintf(stderr, "memlint: %v\n", err)
			return ExitUsage
		}
	}

	if *expectTree != "" && !isHexPrefix(*expectTree) {
		fmt.Fprintf(stderr, "memlint: --expect-tree wants 12 to 64 hex characters, got %q\n", *expectTree)
		return ExitUsage
	}

	res := lint.RunChanged(absRoot, cfg, changedSet)
	tree, err := lint.Fingerprint(absRoot)
	if err != nil {
		fmt.Fprintf(stderr, "memlint: %v\n", err)
		return ExitUsage
	}
	res.Tree = tree
	if *expectTree != "" {
		res.ExpectTree(*expectTree)
	}

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

func isHexPrefix(s string) bool {
	if len(s) < 12 || len(s) > 64 {
		return false
	}
	for _, c := range strings.ToLower(s) {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

// runFingerprint prints the tree fingerprint and nothing else, so a script
// can capture it whole. Read-only, needs no config.
func runFingerprint(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	root, code, done := parseCommand(fs, args, fingerprintUsage, stdout, stderr)
	if done {
		return code
	}
	info, err := os.Stat(root)
	if err != nil {
		fmt.Fprintf(stderr, "memlint: cannot read target %s: %v\n", root, err)
		return ExitUsage
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "memlint: target %s is not a directory\n", root)
		return ExitUsage
	}
	fp, err := lint.Fingerprint(root)
	if err != nil {
		fmt.Fprintf(stderr, "memlint: %v\n", err)
		return ExitUsage
	}
	fmt.Fprintln(stdout, fp)
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
