# Contributing

memvet is small on purpose. Two things keep it that way.

**`check` stays read-only.** No `--fix`, no autofix path, no write outside
`memvet init`. A pull request that adds one will be declined on that ground
alone.

**A new rule needs a silent, consequential failure that nothing else catches.**
Before proposing an invariant, answer four questions in the issue: what must
remain true, what silent failure occurs if it does not, why no existing rule
detects it, and why memvet is the right layer rather than a general-purpose
tool.

## How a change lands

1. Open an issue describing the invariant or bug.
2. Behavior changes start as an addendum in [SPEC.md](SPEC.md) with numbered
   gates: the tests, `gofmt` and `go vet`, the fixture counts, and a self-check.
   A gate is a command, its exit status, and the matching output line.
3. Finding codes (`rule/code`) are stable once released. Messages may be
   reworded; codes may not.
4. The fixture counts under [testdata/](testdata) are the acceptance test.
   Changing a planted defect changes [docs/development.md](docs/development.md)
   in the same commit.
5. Dependencies stay at `BurntSushi/toml` and `golang.org/x/term`.

## Running the gates

```bash
go test ./...
gofmt -l . && go vet ./...
go build -o ./memvet .
./memvet check --no-color testdata/fixture-broken   # 10 red, 4 yellow, exit 1
./memvet check --no-color testdata/fixture-clean    # clean, exit 0
./memvet check --strict .                           # self-check
```

CI runs the same steps plus every directory under [examples/](examples).
