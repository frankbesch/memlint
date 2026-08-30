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
   Optional limit=M (must be > budget): over M = RED tokens/over-limit.

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

# --- v0.4 addendum: critique fixes + adds (approved 2026-08-10) ---

Fix — [tokens] zero-match watch glob: a watch glob that matches no file
is YELLOW ("watch glob matched no files"), one finding per glob, path =
the glob string. Rationale: [tokens] watch declares files the config
author expects to exist, so a stale glob is a check that silently never
ran — the failure mode the config layer already refuses for typo'd keys.
[junk] is exempt (matching nothing is the desired state) and [pointers]/
[mirrors]/[blocks] already report missing declared files as RED.
fixture-broken gains one zero-match glob: acceptance becomes 7 red +
3 yellow.

Add — finding codes: every finding carries a stable machine code
"<rule>/<kind>" (e.g. pointers/dead-ref, blocks/unterminated,
mirrors/missing, tokens/no-match, <rule>/unverifiable). Codes are
additive in the JSON document (schema_version stays 1; additions are
non-breaking, removals or renames bump it). Text output is unchanged.
Construction is explicit per site — helpers require a code, so the
compiler enforces coverage.

Add — --format github: GitHub Actions workflow commands, one annotation
per finding: ::error (RED) / ::warning (YELLOW) with file=, line= (when
known), title=memlint <code>. Data is escaped per the workflow-command
rules (% -> %25, CR -> %0D, LF -> %0A; property values also , -> %2C,
: -> %3A). Never colored. Summary line still printed as plain text.
Exit codes unchanged.

Add — memlint init [path]: writes a commented .memlint.toml at path from
read-only inspection of the repo; REFUSES to overwrite an existing
config (exit 2). Exit 0 on write, 2 on any failure; exit 1 unused. Rules
are enabled only on evidence (an index file that exists -> [pointers]
with roots = top-level dirs containing .md files; observed .DS_Store or
*.tmp -> [junk]); everything else appears as commented-out examples.
The generated config must always pass config.Load — pinned by test.
This flips two v0.1 non-goals (config generation, additional
subcommands) — both already promised by the public README roadmap, and
approved explicitly with this addendum. init performs memlint's ONLY
file write, creating one new file; check remains strictly read-only.

Add — Homebrew tap: goreleaser brews block publishing to
frankbesch/homebrew-tap. Prerequisites (manual, before next tag): create
the tap repo, add a TAP_GITHUB_TOKEN secret with write access to it.

# --- v0.5 addendum: base-ref append_only (approved 2026-08-10) ---

`memlint check --base <ref> [path]` makes [append_only] compare each file
against <ref> instead of HEAD, turning the rule into a pull-request gate:
with --base origin/main, a rewrite committed inside the PR is caught even
though the CI working tree equals the PR's HEAD. The baseline source is a
flag, not a TOML key, because it is invocation-specific — local runs want
HEAD, PR CI wants the base branch — and a config would hard-code one
context's answer into every context.

Semantics: <ref> is anything `git rev-parse --verify <ref>^{commit}`
accepts (branch, tag, SHA, merge-base output). Same prefix rule, same
trailing-newline tolerance, same YELLOW when the file has no baseline at
<ref> (a file created after the base has nothing to have diverged from).
Only [append_only] consumes the flag; [human_brief] already scans full
history.

Failure posture, stricter than the default: --base is an explicit demand
for a baseline, so failure to honor it is exit 2 at startup, not a
finding — an unresolvable ref, git missing, or --base given while
[append_only] is not enabled in the config (a flag that silently does
nothing is the failure mode this tool exists to refuse). Default behavior
without --base is unchanged (HEAD, working-tree guard, committed rewrite
goes quiet — still pinned by test).

# --- v0.6 addendum: pointers grow up (approved 2026-08-11) ---

Anchor-aware [pointers]. A candidate containing exactly one "#" splits
into base path + anchor. The base must independently survive every
existing filter — contains "/", no URL (://), no other reject characters,
no YYYY, no whitespace, no root escape — and the roots filter applies to
the base. The base is checked for existence: dead base = RED
pointers/dead-ref (unchanged code), with the anchored form shown in the
message. Deduplication is by base path, so a bare ref and an anchored ref
to the same file report once. Candidates with more than one "#", bare
"#anchor" fragments (no slash), and URLs with fragments stay skipped. The
anchor itself is NOT yet validated — pointers/dead-anchor is reserved for
when it is. This un-skips references that have been invisible since v0.1:
a repo that was clean can turn red, which is the point of the change.

Glob support in [pointers] files. An entry containing glob metacharacters
(* ? [) is a glob, matched against the root-relative path only, never the
basename — a source list is a statement about specific files, and
basename matching would silently widen it (the [tokens] watch rationale
verbatim). ** stays rejected until recursive globs ship (roadmap). Each
matched file is scanned as a pointer source, deduplicated against literal
entries. A glob matching nothing = YELLOW pointers/no-match (the v0.4
tokens/no-match rationale: declared coverage that silently never runs).
A missing literal entry stays RED pointers/missing-source.

[tokens] needs nothing here: watch is already glob-based; its
coverage-narrowing fix is recursion, which is the separate ** roadmap
item. Ruled 2026-08-11.

Fixture ripple: fixture-broken's memory/gone-anchored.md#section flips
from must-not-report guard to planted defect; its [pointers] files gains
a zero-match glob. Acceptance becomes 8 red + 4 yellow. New guards pin
what still must NOT be reported: multi-"#" candidates and URL fragments.
fixture-clean gains a resolving anchored ref and a matching files glob.

# --- v0.7 addendum: self-describing findings (approved 2026-08-11) ---

Every finding links to its own explanation, keyed by the stable codes
v0.4 introduced. No explain subcommand: the subcommand restraint stands
(ruled 2026-08-11); links carry the weight.

docs/findings.md: one "## <code>" heading per static finding code with
severity, what the finding means, and what to do about it. The dynamic
<rule>/unverifiable family shares one "## unverifiable" section. A test
scans the lint package source for code literals and fails when one lacks
a heading, so a new code cannot ship undocumented.

JSON: each finding gains doc_url alongside code — the docs/findings.md
URL on the repository's main branch plus the heading anchor derived from
the code (GitHub's anchor rule strips the slash; the unverifiable family
maps to #unverifiable). Additive, schema_version stays 1. doc_url is
derived at render time, not stored on the finding: it is a property of
the report, not of the invariant.

Text output: each finding line carries its bracketed code after the
message ("... does not exist [pointers/dead-ref]"), and a run with
findings prints one dimmed "docs:" line immediately BEFORE the summary.
The summary stays the final line, so CI's tail -1 contract is unchanged.
This deliberately supersedes v0.4's "text output is unchanged": the
codes existed but were invisible exactly where humans read findings.
--format github is unchanged: annotations already carry the code in
their title, and workflow commands have no link field.

# --- v0.8 addendum: two-tier [tokens] (approved 2026-08-30) ---

Add — [tokens] limit=M: an optional hard tier above budget. Past budget
stays YELLOW tokens/over-budget; past limit is RED tokens/over-limit
("N estimated tokens exceeds hard limit of M (budget B)"). One finding
per file — the worse tier wins. Config: limit absent or 0 disables the
tier; a set limit must be > budget (load error, "greater than budget").
Motivation: a soft tier that is permanently over goes signal-dead — the
FBOS handoff ran over its 2000 budget for 15 straight wraps while
--strict wraps silently failed. The budget warns; the limit gates.
fixture-broken: [tokens] gains limit=400 and memory/medium.md
(250 tokens, between the tiers); big.md (420) upgrades to RED.
Acceptance becomes 9 red + 4 yellow.
