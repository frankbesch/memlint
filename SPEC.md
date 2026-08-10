# memlint — Claude Code build prompt (v0.1)
# Usage: run from a fresh empty repo folder. Start in plan mode.

Build a Go CLI called `memlint` — an invariant checker (think fsck, not
ESLint) for file-based agent memory systems: repos of markdown that AI
runtimes (Claude Code, Codex) read as persistent memory/contracts.

Single command: `memlint check [path]` (default "."). Reads .memlint.toml
at the target root. Missing config section = that rule is disabled.

Rules (all read-only; never autofix):
1. [mirrors] pairs=[[a,b],...] — files byte-identical; dirs: same *.md set,
   each pair byte-identical. Violations RED.
2. [append_only] files=[...] — `git show HEAD:<file>` must be a prefix of
   the working copy (trailing-newline tolerant). Violation RED, report first
   divergent line as was/now. No git baseline = YELLOW note.
3. [pointers] files=[...], roots=[...] — extract repo-path-like refs from
   the listed files (backticked or bare, containing "/"). Check existence
   ONLY if first segment is in roots. Skip: URLs (://), placeholders (<...>,
   YYYY), anchors. Dead ref = RED.
4. [junk] globs=[...] — matches anywhere under root (skip .git) = YELLOW.
5. [tokens] watch=[globs], budget=N — len(chars)/4 per file, over = YELLOW.

Output: findings to stdout, ANSI color (red / yellow / green summary),
auto-disable color when stdout is not a TTY; --no-color, --strict (yellow
also fails), --format json. Exit 0 clean-or-yellow, 1 red (or yellow w/
--strict), 2 config/usage error. Quiet on clean: one green line.

Deps: BurntSushi/toml only. stdlib flag, no cobra. golang.org/x/term OK
for TTY detect. Module: github.com/frankbesch/memlint. MIT license.

Tests: table-driven for the pointer extractor (cases MUST include:
"reading/daily" w/ roots not containing "reading" -> skip; "memory/x.md" ->
check; "reviews/YYYY-MM-DD-<topic>.html" -> skip; "https://a/b" -> skip) and
for append-only (append=pass, rewrite=fail, truncate=fail). testdata/
fixture-broken with 5 planted defects; fixture-clean.

Acceptance: go test ./... green; check on fixture-broken exits 1 with 5
reds + 2 yellows; on fixture-clean exits 0. README with real usage output,
roadmap section, install via go install.

Non-goals v0.1 (do not build): autofix, HTML output, subcommands, watch
mode, semantic/content linting, config generation.

Start in plan mode: plan + file layout before code.

# --- v0.2 addendum: agent-cohabitation rules (approved 2026-08-03) ---
# Source: promptkits/models/agent-cohabitation-contract.md, distilled from
# langchain-ai/openwiki. Adopted: ownership-block structure, human-brief
# authorship. Rejected: no-op commit detection (history hygiene, low
# value), repair-marker/degraded-output validation (semantic linting,
# stays a non-goal).

New rules (same contract: read-only, section presence enables):
6. [blocks] files=[...], start="...", end="..." — each listed file must
   contain exactly one well-formed ownership block: one start marker,
   one end marker, start before end. Markers are literal single-line
   strings matched anywhere in a line. Violations RED, one finding per
   file (first structural problem wins): no markers, end without start,
   unterminated, duplicate start, duplicate end, end before start.
   Listed file missing = RED. Config rejects empty/equal/multiline
   markers and markers containing each other (ambiguous matching).
7. [human_brief] files=[...], agent_authors=[...] — the full git history
   of each listed file must contain no commit authored OR co-authored by a
   configured agent identity. Both the commit author and every
   Co-Authored-By trailer are checked (the trailer is the common assisted-
   commit shape, so author-only matching would pass the very commits this
   rule exists to catch); match is by name or email, case-insensitive and
   exact. Violation RED, once per file, naming the most recent offending
   commit (author or co-author) plus a count of earlier ones. No git / no
   repo / no commits touching the file = YELLOW, mirroring [append_only].
   This is deliberately the first history-scanning rule: authorship is not
   readable from the working tree, so scanning history IS the invariant,
   not scope creep. Renames are not followed.

Tests: table-driven block scanner (well-formed, same-line block,
indented markers, each malformation, missing file); human_brief against
real temp repos (clean, match by author name, by author email, by
Co-Authored-By trailer name/email, case-insensitive, human co-author is
clean, multiple agent commits, untracked = yellow, no repo = yellow, root
inside larger repo, agent edit stays RED after later human commits).
fixture-broken gains 2 blocks REDs (unterminated AGENTS.md, duplicate
start docs/generated.md): acceptance becomes 7 red + 2 yellow.
[human_brief] stays out of the static fixtures for the same reason as
[append_only]: fixture files are committed by the repo's human author,
so a hermetic violation cannot be expressed there.

# --- v0.3 addendum: version + releases (approved 2026-08-10) ---

`memlint --version` prints "memlint <version>" to stdout and exits 0.
Top-level flag only, not a subcommand and not a `check` flag — the
subcommand non-goal stands. Version resolution, in order: ldflags-injected
value (release builds), module version from runtime/debug.ReadBuildInfo
(`go install` builds and VCS-stamped local builds), then "dev" (builds
where no version info survives). The injected form keeps the tag's
leading "v" so all paths print the same shape.

Releases: goreleaser, tag-triggered (v*). linux+darwin, amd64+arm64,
CGO disabled, checksums published. --version ships in the same release
as the first binaries: an unidentifiable binary is a support burden.
The ldflags symbol is pinned by a test that builds with -X and asserts
the output, so renaming the variable breaks tests before it breaks
releases.
