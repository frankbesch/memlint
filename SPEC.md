<!-- This is memlint's build log: the original build prompt followed by one
versioned addendum per shipped change, each with the gates that proved it.
It exists for design provenance. The user manual is README.md; reference
docs are under docs/. D-### references are the maintainer's decision log and
are not part of this repository. -->

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

# --- v0.7.0 release addendum, part 1: rotation-aware [append_only] (approved 2026-09-05) ---
# Source: promptkits D-126 (decisions-log rotation) and D-127. Tags stop at
# v0.6.0; the v0.7 and v0.8 addenda above shipped untagged and land in the
# same v0.7.0 release as this one.

Motivation: FBOS rotated memory/decisions.md — D-001–D-100 moved verbatim
to memory/archive/20260905-decisions-vol1-D001-D100.md, the live file kept
D-101+ under a header pointing at the volume. append_only diffs the working
file against HEAD, so the rotation commit was RED append_only/rewritten,
committing it reset the baseline, and memlint verified nothing about the
move.

Add — header_lines = N (optional, default 0) in [append_only]. The first N
lines of every listed file, in the baseline and the working copy alike, are
the ONLY mutable span. Everything after line N is immutable, or must move
verbatim (below). Divergence line numbers stay full-file. Negative is a
load error. N = 0 is byte-for-byte the pre-v0.7 check.

Add — the moved-to allowance. When a listed file no longer begins with its
(header-stripped) baseline, before reporting RED: take the cut span — the
baseline lines after the longest shared leading run and before the shortest
line-aligned baseline tail the working copy still continues with (same
trailing-newline tolerance) — and search every OTHER listed file that has
no baseline at the ref (untracked, or new after --base) for that span
appearing verbatim, whole-line, after that file's own header. Candidates in
config order, first hit wins. Found: no finding on the rewritten file, one
INFO append_only/rotated "<src> → <dst> (N lines moved verbatim)" at the
cut's first line, and the destination's append_only/no-baseline YELLOW is
withdrawn (the moved span is its baseline; leaving the YELLOW would trip
every --strict rotation, which defeats the allowance). Not found — altered
or missing moved line, destination already committed, not listed, or absent
— the existing RED, unchanged. A cut of only whitespace never qualifies.

Add — INFO severity. Green; never affects the exit code, not under --strict
either; a run with only INFO findings prints its findings, the docs line,
and the CLEAN summary line (tail -1 contract unchanged). JSON: severity
"INFO", summary.info present only when non-zero (schema_version stays 1;
red/yellow-only runs render byte-identically to v0.6). --format github:
::notice. Sort rank after YELLOW.

Tests: header_lines exempts header only, full-file line numbers; rotation
= one INFO, both files counted; rotation without header_lines; allowance
withdraws (altered, missing, span inside destination header, destination
not listed, no destination); destination must be new at baseline; the
non-target untracked file keeps its YELLOW; rotation under --base;
whole-line matching. Fixture: testdata/fixture-rotated (baseline/ +
rotated tree; TestFixtureRotated builds the git state). fixture-broken
unchanged: 9 red + 4 yellow.

# --- v0.7.0 release addendum, part 2: [ids] (approved 2026-09-05) ---
# Source: promptkits D-127 — two sessions wrote D-102 on 2026-09-01;
# next-id.sh prevents new collisions upstream but cannot see the file, and
# nothing in memlint checked id uniqueness.

8. [ids] files=[...] (literals and globs, the [pointers] files resolver:
   root-relative globs, literals in config order then glob matches in walk
   order, deduplicated; zero-match glob = YELLOW ids/no-match, missing
   literal = RED ids/missing-source), pattern="^(D-\\d{3}) \\|" (default:
   an entry line, delimiter included — see the acceptance note). The
   pattern is matched per line; only a match starting at column 1 is an id,
   so a mid-line citation never counts; the id is the first capture group,
   or the whole match without one. Every id must be unique across all
   listed files. Duplicate = RED ids/duplicate at the LATER occurrence,
   "duplicate id <id>: first at <path>:<line>", related_path = the first
   occurrence's file; three occurrences are two findings, each citing the
   first. Gaps are not findings (D-070 is legitimately absent). One section
   carries one pattern; a second numbering (lesson files) gets its own
   section when that ships. Config rejects an empty files list, duplicate
   entries, and an uncompilable pattern; an absent or empty pattern is the
   default.

Tests: unique = clean; duplicate in one file (path, line, related path,
message); triple = two findings; across files with literal-before-glob
order; column-1 rule with and without ^ in the pattern; custom pattern with
and without a capture group; CRLF; missing literal RED, zero-match glob
YELLOW. Fixtures: testdata/fixture-dupids (2 RED: D-102 twice in the live
log, D-050 across volume and log; a gap and a mid-line mention planted as
non-findings); fixture-clean gains [ids] and a decisions.md with a gap and
a mid-line mention (6 rules, 9 files). fixture-broken unchanged: 9 red +
4 yellow. TestEveryCodeIsDocumented scans "ids" too.

Acceptance (FBOS, run 2026-09-05 on a scratch clone): memlint check
--strict ~/Documents/promptkits is clean with today's config. Adding [ids]
with the DEFAULT pattern yields 12 ids/duplicate, not 1: FBOS entries wrap
by hand, and eleven continuation lines begin with a cited id at column 1
("D-102). (2) `scripts/next-id.sh` ships…"). With pattern =
"^(D-\\d{3}) \\|" — entry lines only — the result is exactly one:
D-102, first memory/decisions.md:43, again memory/decisions.md:58, the
receipt D-127 already records. RULED (Frank, 2026-09-05, same day): the
delimiter form "^(D-\\d{3}) \\|" IS the default; the bare form is opt-in.

# --- v0.7.0 release addendum, part 3: [ids] known (Frank ruled "A", 2026-09-05) ---

Add — [ids] known = [...]: ids whose collision is recorded and reconciled
(D-127: neither D-102 entry may be edited, so the RED was permanent and
blocked promptkits' push-ok.sh). A known id's duplicates report as INFO
ids/known-duplicate ("known duplicate id <id>: first at <path>:<line>"),
same detection, same anchoring. A known id that never collides is YELLOW
ids/known-unused — a stale entry would silently excuse a future collision
of that id (the tokens/no-match posture). Config rejects empty and
duplicate known entries.

# --- v0.8 addendum, part 1: recursive ** globs (approved 2026-09-05) ---

Every glob-taking key ([junk] globs, [tokens] watch, [pointers] files,
[ids] files) accepts "**" as a WHOLE path segment matching zero or more
directories: "memory/**/*.md" covers memory/a.md and memory/deep/er/a.md;
"memory/**" covers everything under memory/; "**/*.tmp" covers the tree.
"*" and "?" still never cross "/". "**" inside a segment ("a**b") is a
load error. One translator (lint.GlobToRegexp) serves matching and
validation. [junk] keeps its basename match as well. Removes the v0.1
non-goal and the v0.6 "** stays rejected" clause.

# --- v0.8 addendum, parts 2-4 (approved 2026-09-05) ---

2. [blocks] mirror = true: content between the markers identical across
   the listed files; first listed file is the reference; RED
   blocks/content-differ at the first differing line of the drifted file,
   related_path = reference, detail shows both lines. Structurally broken
   files keep their single structural finding and are not compared. Off
   by default: v0.2 behavior unchanged.
3. [append_only] headers = { "path" = N }: per-file override of
   header_lines, for both the file's own prefix check and its role as a
   rotation destination. Keys must be listed files; negative rejected.
4. [human_brief] follow_renames = true: git log --follow, so an agent
   commit under an earlier name stays a violation. Off by default.

# --- v0.8 addendum, parts 5-6: [ids] cited_in + ordered (approved 2026-09-05) ---

5. [ids] cited_in = [...] (same resolver), cite_pattern = "\\b(D-\\d{3})\\b"
   (default): every cited id in those files must be an entry collected
   from files. RED ids/dead-cite "cited id <id> has no entry" at
   file:line, one per distinct id per line. files' own prose is not
   checked unless listed (forward references are legitimate there).
   Missing literal = RED ids/missing-source; zero-match glob = YELLOW
   ids/no-match.
6. [ids] ordered = true: within each file, entries non-decreasing by the
   number in the id; RED ids/out-of-order "id <id> follows <prev> (line
   N)". Equal ids are the duplicate rule's business; gaps are fine. Ruled
   into [ids] rather than [append_only]: the invariant is about ids, and
   the prefix rule cannot see a committed mid-file paste anyway.

# --- v0.8 addendum, parts 7-8: [stamps] and [secrets] (approved 2026-09-05) ---

7. [stamps] files=[...] (mixed resolver), max_age_days=N (>0),
   pattern (default "(?i)last[ -]verified:?\\s*(\\d{4}-\\d{2}-\\d{2})",
   must capture the date). The stamp date must be within N days of the
   file's last change: the last commit's author date, or now when the
   working copy differs from HEAD. Stale = YELLOW stamps/stale on the
   stamp line (age and last-change date in the finding); no stamp = RED
   stamps/missing; unparsable date = RED stamps/unparsable; no history =
   YELLOW stamps/no-baseline. Source: FBOS stale-artifacts register,
   kept by hand until now.
8. [secrets] globs=[...] (root-relative), patterns=[...] (optional extra
   regexes). Built-ins: AWS access key, GitHub token, Anthropic key,
   OpenAI key, Slack token, private-key block, Luhn-valid 13-19-digit
   card number. Each hit is RED secrets/match "possible <kind> (value not
   shown)" — the value is never echoed. Zero-match glob = YELLOW
   secrets/no-match. Source: D-057 / FF-010 tripwire; --changed (part 9)
   makes it a pre-commit check.

# --- v0.8 addendum, part 9: --changed (approved 2026-09-05) ---

memlint check --changed [path]: changed = `git diff --name-only --relative
HEAD -- .` ∪ `git ls-files --others --exclude-standard -- .`, relative to
the memlint root. Every rule runs (cross-file logic intact), then findings
are kept only when path or related_path is in the set, or the path is not
an existing file (config-level: no-match globs, known-unused). FilesChecked
counts only changed files. [append_only], [human_brief], [stamps] skip
unchanged files before their git call. No git / not a repository = exit 2
(the --base posture: an explicit demand that cannot be honored must not
silently widen). Output formats and exit codes otherwise unchanged.

# --- v0.9 futures (listed 2026-09-05) ---

1. [pointers] dead-anchor — APPROVED for build 2026-09-08 (FBOS D-136);
   see v0.9 addendum part 2 below. Built 2026-09-08 (v0.9.1).
2. [secrets] entropy detector: long high-entropy strings the shape-based
   detectors miss, with an allowlist for fixtures and examples. NOT
   approved (09-08): wait for an allowlist tuned on the real corpus, else
   it floods every wrap with yellows.

# --- v0.9 addendum, part 1: tree fingerprint (Frank ruled 2026-09-08) ---
# Source: Paper Forge PF-0106 (Graft, NanoNets): every query fingerprints
# the working tree before answering, so an answer describes the tree as it
# is now. FBOS lesson 021 / D-053: a checked fact has a shelf life. memlint
# runs in ~0.5 s on FBOS, so a skip-if-unchanged cache buys nothing; the
# adopted idea is the RECEIPT — the verdict names the tree it judged, and a
# later step can demand that same tree. Graft itself is not adopted for
# FBOS or AIPOS (it indexes code only, writes into ~/.claude and ~/.codex,
# and phones home by default).

memlint fingerprint [path]: prints one line, the 64-hex SHA-256 of the
tree memlint would check: for every visible regular file, sorted by
root-relative slash path, "path\0size\0sha256(content)\n". Content-based,
so mtime, clone, and checkout do not move it; two identical trees on two
machines share one fingerprint. Visible = what a commit could contain when
git is present and root is inside a repository (`git ls-files --cached
--others --exclude-standard`, minus paths that no longer exist); else every
regular file under root except .git/. Symlinks are never followed. The
config file is inside the set, so a rule change moves the fingerprint.
Read-only; memlint still writes nothing but `init`.

memlint check gains the fingerprint in every summary: text
"memlint: clean (8 rules, 608 files checked, tree 0951f4c6ddbd)" and
"memlint: 9 red, 4 yellow (tree 0951f4c6ddbd)" — first 12 hex; the tail -1
receipt scripts already keep now carries the tree. JSON summary gains
"tree": "<64-hex>" (additive; schema_version stays 1). github format
unchanged. `--expect-tree <fp>` (full or >=12-hex prefix) adds one RED
finding `tree/moved` on "." when the computed fingerprint does not start
with it: the receipt is stale and the run is not the run you recorded.
Exit codes unchanged: a moved tree is exit 1 like any RED.

Gates (D-101), declared before code:
G1 CHECK `memlint fingerprint testdata/fixture-clean` twice, then on a
   copy of the fixture in a temp dir (fresh mtimes) EXPECT three identical
   64-hex lines.
G2 CHECK append one byte to one file in the temp copy EXPECT a different
   fingerprint; restore the byte EXPECT the original fingerprint.
G3 CHECK `check --expect-tree <fp of fixture-clean>` EXPECT exit 0, clean
   line ending "tree <12hex>)"; `check --expect-tree 000000000000` EXPECT
   exit 1 with exactly one RED `tree/moved`.
G4 CHECK gofmt -l is empty, go vet clean, go test ./... passes, and
   --format json summary.tree equals the fingerprint command's output.
G5 CHECK `check --strict .` on promptkits EXPECT under 1.0 s wall
   (baseline 0.56 s) and a fingerprint identical across two runs.

# --- v0.9 addendum, part 2: [pointers] dead-anchor (approved 2026-09-08, D-136; built 2026-09-08) ---

A reference of the form "file.md#anchor" resolves the file today and
ignores the anchor (reserved since v0.6). Dead-anchor makes the anchor
load-bearing: the target markdown must contain a heading whose GitHub-style
slug equals the anchor (lowercase, spaces to hyphens, punctuation dropped,
duplicate headings suffixed -1, -2), or an explicit HTML anchor
(<a id="..."> / <a name="...">). Missing = RED pointers/dead-anchor with
the target path as related_path and the nearest heading slugs in detail.
Only .md targets are checked; other extensions keep today's file-only
behavior. Fixture-broken gains one planted dead anchor and one live one.
Gates declared at build time, D-101.

Gates (D-101, declared 2026-09-08 before code):
G1 CHECK `check --no-color testdata/fixture-broken` EXPECT 10 red, 4 yellow,
   exit 1, with one RED pointers/dead-anchor on memory/index.md for
   `memory/existing.md#nowhere` and NO finding for `memory/existing.md#overview`.
G2 CHECK `check testdata/fixture-clean` EXPECT clean (its
   `memory/existing.md#notes` resolves to a "## Notes" heading).
G3 CHECK unit tests for the slug rule EXPECT: case folded, punctuation
   dropped, spaces to hyphens, duplicate headings suffixed -1/-2, headings
   inside fenced code ignored, `<a id="x">` / `<a name="x">` honored,
   non-.md targets never checked, anchored ref to a missing base stays
   pointers/dead-ref only.
G4 CHECK gofmt -l empty, go vet clean, go test ./... passes; README, CI
   acceptance step, and cli test all read 10 red, 4 yellow.
G5 CHECK `check --strict ~/Documents/promptkits` EXPECT clean with no new
   findings (FBOS pointer sources carry no anchored refs today).

# --- v0.10 addendum, docs note (shipped 2026-09-11, docs-only, no ruling required) ---

README became the landing page (7 KB); reference sections moved verbatim to
docs/cli.md, configuration.md, rules.md, tree-receipts.md, development.md;
docs/recipes.md is backed by examples/{minimal,decision-log,
shared-instructions,broken}, each held to its stated result by
internal/cli/examples_test.go, which also keeps the README output block
byte-identical to the real run. No rule, flag, or output changed. Commits
5e2478b, 632c0f2, 0547303. Source: Astral review 2026-09-11, items 1-5,
9, 13-17.

# --- v0.10 addendum, part 1: flags anywhere around the path (approved 2026-09-11, D-143 §1) ---

Today `memlint check . --strict` is refused with exit 2 because the stdlib
parser stops at the first positional and a silently ignored flag is worse
than an error. The refusal is correct; the affordance is not. The parser
gains a pre-pass that separates recognized flags (with their values) from
positionals, then parses the flags with the same FlagSet. Result: flags are
honored before or after the path; more than one positional is still exit 2
("unexpected argument"); an unknown flag anywhere is still exit 2; `--`
ends flag parsing as before. Applies to check, init, fingerprint. No
output or exit-code change on any currently accepted invocation. Deletes
the "flags must come before the path" paragraph from docs/cli.md.

Gates (D-101, declared before code):
G1 CHECK `check testdata/fixture-clean --strict` and
   `check --strict testdata/fixture-clean` EXPECT identical stdout and exit.
G2 CHECK `check testdata/fixture-broken --format json` EXPECT valid JSON,
   exit 1, same bytes as the flag-first form.
G3 CHECK `check --strict a b` EXPECT exit 2, stderr names "b" as unexpected;
   `check . --bogus` EXPECT exit 2, stderr names --bogus.
G4 CHECK TestFlagAfterPathIsRefusedNotIgnored becomes
   TestFlagAfterPathIsHonored; gofmt/vet/test green; fixtures 10/4, clean,
   2/0 unchanged.
G5 CHECK `check --strict ~/Documents/promptkits` EXPECT clean.
G6 CHECK post-push CI run green on ubuntu and macos (`gh run watch
   --exit-status`), cited by run id.

# --- v0.10 addendum, part 2: per-command help (approved 2026-09-11, D-143 §2) ---

One usageText serves every command today. Split it: `memlint --help` fits
one screen (name line, three commands with one-line purposes, "run memlint
<command> --help"); `check --help` owns flags, exit codes, formats;
`init --help` owns what it inspects and that it never overwrites;
`fingerprint --help` owns the receipt semantics and --expect-tree. `memlint
help <command>` is an alias. Help goes to stdout with exit 0; usage errors
still print the relevant command's help to stderr with exit 2. Docs
unchanged; docs/cli.md stays the long form.

Gates:
G1 CHECK `memlint --help | wc -l` EXPECT <= 24; contains "check", "init",
   "fingerprint", not "--expect-tree".
G2 CHECK `check --help` contains every flag in docs/cli.md's flag table (test
   parses the table and asserts each flag string appears).
G3 CHECK `init --help` contains "never overwrite"; `fingerprint --help`
   contains "--expect-tree".
G4 CHECK `check --bogus` stderr contains the check help, not the init help.
G5 gofmt/vet/test green; fixtures unchanged. G6 CI green, run id cited.

# --- v0.10 addendum, part 3: init reports what it inferred (approved 2026-09-11, D-143 §3) ---

init keeps its contract: evidence enables a rule; guesses never do; O_EXCL
refuses to overwrite. Two changes. (a) The generated file is trimmed to a
four-line header, the enabled sections, and commented sections ONLY for
rules init found plausible evidence for; the full commented encyclopedia
goes. (b) stdout becomes a report in three tiers:
  Enabled      pointers (MEMORY.md, CLAUDE.md -> roots memory, docs)
               junk (.DS_Store seen)
  Suggested    append_only (memory/decisions.md exists)  [commented in file]
               mirrors (CLAUDE.md and AGENTS.md both present)
  Not inferred human_brief (authorship is a policy choice)
               tokens (a budget is a choice)  blocks, ids, stamps, secrets
  Next         review .memlint.toml, then: memlint check
Suggestion evidence, exhaustively: append_only when a file named
decisions.md or decision-log.md exists under an md root; mirrors when two
or more of CLAUDE.md, AGENTS.md, GEMINI.md exist at the root. Nothing else
is suggested. The "N rules enabled" summary line is replaced by the report;
TestInitGeneratesWorkingConfig is updated to assert the Enabled tier.

Gates:
G1 CHECK init on a temp tree with MEMORY.md + memory/decisions.md + CLAUDE.md
   + AGENTS.md EXPECT Enabled pointers; Suggested append_only, mirrors;
   generated file contains "# [append_only]" and "# [mirrors]" commented, no
   other commented sections; file loads; `check` runs.
G2 CHECK init on an empty dir EXPECT no Enabled tier, all rules under Not
   inferred, file loads and `check` says "clean (no rules enabled)".
G3 CHECK generated file line count on G1's tree EXPECT <= 30.
G4 CHECK TestInitRefusesOverwrite unchanged and green.
G5 gofmt/vet/test green; fixtures unchanged. G6 CI green, run id cited.

# --- v0.10 addendum, part 4: init --dry-run (approved 2026-09-11, D-143 §4) ---

`memlint init --dry-run [path]` performs the same inspection, prints the
report to stderr and the config that WOULD be written to stdout, and
writes nothing. It works when .memlint.toml already exists, so discovery
can be rerun against a configured repo. No --force, now or later: the
overwrite refusal stays. `check` remains the only other command and stays
read-only.

Gates:
G1 CHECK `init --dry-run <dir>` EXPECT exit 0, stdout parses as the config
   G1 of part 3 would write, and no .memlint.toml exists afterwards.
G2 CHECK `init --dry-run` on a dir that already has .memlint.toml EXPECT
   exit 0 and the existing file byte-identical before and after.
G3 CHECK `init --force` EXPECT exit 2, unknown flag.
G4 gofmt/vet/test green; fixtures unchanged. G5 CI green, run id cited.

Release: parts 1-4 ship together as v0.10.0. Docs touched: docs/cli.md
(delete the flag-order paragraph, add --dry-run), README quick start
(one line on the init report), docs/development.md roadmap.

# --- v0.10 parts 1-4: build receipt (built 2026-09-11, D-143) ---

Gates run against the working tree that became this commit:
G1-G3 (all parts) CHECK `go test ./...` EXPECT ok for internal/cli,
   config, lint, report; grep FAIL empty. The declared gates are tests:
   TestFlagsAnywhereAroundThePath, TestBadArgumentsAreLoud,
   TestTopLevelHelpIsOneScreen (13 lines), TestCheckHelpCoversDocumentedFlags,
   TestPerCommandHelp, TestInitReportsTiers (18-line config, commented
   sections exactly append_only,mirrors), TestInitEmptyRepoListsEverythingAsNotInferred,
   TestInitDryRun, TestFlagAfterPathIsHonored (renamed from
   TestFlagAfterPathIsRefusedNotIgnored; the exit-code table row flips 2 -> 1
   and gains a "two paths" row at 2).
G4/G5 CHECK gofmt -l empty; go vet clean; fixture-broken 10 red 4 yellow;
   fixture-clean clean (6 rules, 9 files); fixture-dupids 2 red 0 yellow;
   examples minimal/shared-instructions clean, broken 1 red 1 yellow.
G5 CHECK `check --strict ~/Documents/promptkits` EXPECT clean, and the same
   with the flags after the path: clean (8 rules, 618 files, tree
   ac7db8cd94b5), exit 0.
G6 CI run id recorded in the commit that follows this one if it is not
   green on the first push; otherwise cited in the handoff.

# --- v0.11 addendum: gap closure vs agents-lint / ctxlint / claude-healthcheck (approved 2026-09-11, D-144 §1-6) ---
# Source: 2026-09-11 competitor read of giacomo/agents-lint, YawLabs/ctxlint,
# mister-no-one/claude-healthcheck (READMEs only, code not read; a Codex
# cross-check the same day corrected two claims, verified against the
# READMEs before this header was fixed). Among those three, no equivalent
# was found for append_only against a git baseline, human_brief authorship
# from git history, ids uniqueness/order/citation, blocks ownership, exact
# declared mirrors, or tree receipts. ctxlint is the closest neighbour and
# already covers memory hygiene (session-stale-memory: memory entries whose
# paths no longer exist; session-memory-index-overflow: MEMORY.md past the
# Claude Code load cap) plus MCP, skills, sessions, SARIF, a GitHub Action,
# pre-commit and an MCP server; its distinction from memlint is built-in
# health model vs owner-declared invariants, not codebase vs memory.
# agents-lint checks paths, npm scripts, dependencies, framework staleness,
# structure and Claude memory-file links; it has no secrets, junk, or
# token-budget rule (an earlier read conflated it with another project).
# claude-healthcheck is read-only too; memlint's distinction is that
# `check` has no fix mode at the contract level, while ctxlint and
# agents-lint expose --fix. memlint is behind on first-run friction: both
# linters run on a bare `npx` with no config, both check the Claude Code
# auto-memory folder, and ctxlint ships a GitHub Action and a pre-commit
# hook. The parts below close the cheap gaps only. Explicitly NOT pursued:
# --fix (hard rule 1), an MCP server (scope, no dependency budget), checks
# of context files against the codebase (npm scripts, framework staleness:
# a different product), SARIF (no consumer yet), and a score (advisory,
# not a gate).

# --- v0.11 part 1: check runs without a config, and says so (approved 2026-09-11) ---

Today `memlint check` on a tree with no .memlint.toml is exit 2. After
this part it runs the config `init --dry-run` would print (Enabled
sections only; Suggested stays commented and therefore off) and reports
one YELLOW finding first:
  config  YELLOW  .memlint.toml  not found; ran the inferred config
                                 (pointers, junk); memlint init to keep it
                                 [config/inferred]
Exit codes are unchanged in meaning: 0 with no RED, 1 with RED or with
--strict (the YELLOW then fails the run, so a CI job that forgot its
config is loud, not clean). Nothing is written. An invalid existing
config stays exit 2; only absence triggers inference. When inference
enables nothing, the run prints the YELLOW and then "clean (no rules
enabled)". `config` becomes a reserved rule name in output; no config
section of that name exists. Ruled: default-on with the YELLOW; no
`--infer` flag. The YELLOW is what keeps inferred coverage from ever
reading as a silent clean.

Gates:
G1 CHECK `check` on a temp tree with MEMORY.md + memory/a.md + a dead link
   EXPECT exit 1, first line is config/inferred YELLOW, then
   pointers/dead-ref RED; no .memlint.toml afterwards.
G2 CHECK `check --strict` on a temp tree with MEMORY.md + memory/a.md and
   no defects EXPECT exit 1 on the YELLOW alone; without --strict exit 0.
G3 CHECK `check` on an empty temp dir EXPECT YELLOW then "clean (no rules
   enabled)", exit 0.
G4 CHECK `check` on a tree with a malformed .memlint.toml EXPECT exit 2,
   unchanged.
G5 CHECK `--format json` and `--format github` carry the config finding
   like any other.
G6 gofmt/vet/test green; fixtures unchanged (all three carry configs).
G7 self-check on FBOS unchanged (it has a config). G8 CI green, run id.
Estimate: ~60 lines in internal/cli (reuse inspect + renderConfig, parse
the string through the same loader), one report constant, 4 tests.
Half a session.

# --- v0.11 part 2: wider index discovery (approved 2026-09-11) ---

init and part 1 inference enable [pointers] on any of MEMORY.md,
CLAUDE.md, AGENTS.md that exist. Add, on the same evidence-only basis:
GEMINI.md, COPILOT.md, .github/copilot-instructions.md, .cursorrules,
CONVENTIONS.md. These are the files agents-lint and ctxlint both read;
each is a plain text file with repo paths in it, so a dead reference in
one is the same defect. The [mirrors] suggestion list does NOT widen:
D-143 §3 ruled it closed, and widening it is a separate ruling if ever.

Gates:
G1 CHECK init on a temp tree with .github/copilot-instructions.md +
   docs/a.md EXPECT Enabled pointers naming that file, roots docs.
G2 CHECK TestInitReportsTiers unchanged in its counts (the v0.10 tree has
   none of the new files).
G3 gofmt/vet/test green; fixtures unchanged. G4 CI green, run id.
Estimate: a five-entry list change plus one test. Under an hour; ships
with part 1.

# --- v0.11 part 3: [pointers] on a flat memory folder (approved 2026-09-11) ---

The Claude Code auto-memory folder (~/.claude/projects/<slug>/memory/) is
MEMORY.md plus sibling notes, indexed as `- [Title](note.md) — hook`.
A candidate today must contain `/` and its first segment must be in
`roots`, so a sibling link is never checked and a renamed note is
invisible. agents-lint checks exactly this. Change: `roots` accepts `"."`,
meaning slash-less destinations from the markdown-link and image pass
ONLY (never bare tokens, which would flag ordinary words like `a.md` in
prose) resolve against the source file's own directory. Same finding
code, pointers/dead-ref; same anchor handling. Bare-token and code-span
passes are unchanged. init enables `roots = ["."]` when MEMORY.md sits
beside two or more .md files and no md root exists; part 1 then makes
`memlint check ~/.claude/projects/<slug>/memory` work with no config,
which is the one-line recipe this part exists for.

Gates:
G1 CHECK a temp dir with MEMORY.md linking `gone.md` and `here.md`, only
   here.md present, roots ["."] EXPECT 1 RED pointers/dead-ref at the
   MEMORY.md line of gone.md.
G2 CHECK the same tree with a prose line `see a.md for context` EXPECT no
   finding for a.md (bare tokens stay slash-gated).
G3 CHECK fixture-broken and fixture-clean unchanged (neither declares ".").
G4 CHECK init on the G1 tree EXPECT Enabled pointers with roots ["."].
G5 CHECK docs/rules.md pointers section documents "." and the link-only
   scope; docs/recipes.md gains "Claude Code auto-memory" with the
   one-line command.
G6 gofmt/vet/test green. G7 self-check on FBOS unchanged. G8 CI green.
Estimate: ~80 lines across lint/pointers.go, config validation, init;
5 tests; two doc sections. One session. This is the only part that
touches a rule's semantics, so it is the one to defer if anything is.

# --- v0.11 part 4: GitHub Action and pre-commit hook (approved 2026-09-11) ---

Add `action.yml` at the repo root: a composite action that downloads the
release binary for the runner's OS/arch from the tagged release, verifies
the checksum file goreleaser already publishes, and runs
`memlint check --format github [path] [--strict]`. Inputs: path (default
"."), strict (default false), version (default "latest"). Add
`.pre-commit-hooks.yaml` with one hook, id `memlint`, `language: golang`,
entry `memlint check --changed`, pass_filenames false. Neither file
changes the binary. Marketplace listing is a manual step for Frank and
is not a gate. Docs: README "In CI" gains the `uses:` snippet;
docs/recipes.md gains the pre-commit stanza.

Gates:
G1 CHECK a workflow in this repo runs the action against examples/broken
   with continue-on-error EXPECT the annotations for the 1 red 1 yellow
   appear on the run, and the step exit is 1.
G2 CHECK `pre-commit try-repo . memlint` on examples/broken EXPECT exit 1
   with the same two findings.
G3 gofmt/vet/test green; fixtures unchanged. G4 CI green, run id.
Estimate: ~50 lines of YAML, two doc snippets, one CI job. Half a
session, but G1 needs a tag to resolve `version: latest`, so it is
verified against the v0.11.0 release, not before.

# --- v0.11 part 5: run once without installing (approved 2026-09-11, docs only) ---

README install section adds `go run github.com/frankbesch/memlint@latest
check .` as the try-it line, matching the competitors' `npx` one-liner.
Nothing to gate beyond the README output test staying green.

Release: parts 1, 2, 4, 5 ship as v0.11.0 in about two sessions. Part 3
adds a third session and can follow as v0.11.1 without a visible gap.

# --- v0.11 parts 1-5: build receipt (built 2026-09-11, D-144; released v0.11.0, CI 34639081021, release run 34639549431) ---

Gates run against the working tree that became this commit:
G1-G5 (part 1) CHECK `go test ./internal/cli` EXPECT ok. The declared
   gates are tests: TestNoConfigRunsInferredConfigAndSaysSo (G1),
   TestInferredConfigFailsOnlyUnderStrict (G2),
   TestNoConfigOnEmptyTreeSaysNothingRan (G3),
   TestMalformedConfigIsStillAStartupError (G4),
   TestInferredFindingInEveryFormat (G5). TestMissingConfigIsAStartupError
   is deleted: its contract (exit 2 on absence) is the one this part
   replaces. Deviation from the addendum's G3 wording: with the YELLOW
   present the summary is "0 red, 1 yellow", not "clean (no rules
   enabled)"; the finding message carries "nothing to infer, so no rules
   ran" instead, because a run with a YELLOW must not summarize as clean.
G1-G2 (part 2) CHECK TestInitDiscoversOtherRuntimeInstructionFiles ok;
   TestInitReportsTiers unchanged and ok.
G1-G5 (part 3) CHECK TestFlatMemoryFolder (G1, G2, G4) and
   TestPointersSiblingRoot (five subtests: dead sibling red, source-dir
   resolution, sibling anchors, no dot root means no check, skips) ok;
   fixtures unchanged (G3); docs/rules.md and docs/recipes.md updated (G5).
   Real run: `check ~/.claude/projects/-Users-frankbesch-Documents-memlint/memory`
   with no config -> config/inferred YELLOW (pointers), 0 red, exit 0.
G1-G2 (part 4) CHECK the action's download step, extracted from action.yml
   and run locally with VERSION=latest against the v0.10.0 release EXPECT
   checksum "memlint_0.10.0_darwin_arm64.tar.gz: OK", `memlint v0.10.0`,
   and `check --format github examples/broken` -> ::error + ::warning
   annotations, exit 1. Archive naming fixed during rehearsal
   (memlint_<ver>_<os>_<arch>.tar.gz, lowercase, no leading v).
   `uvx pre-commit try-repo <this repo> memlint` on a git copy of
   examples/broken with MEMORY.md staged EXPECT Failed, exit 1,
   pointers/dead-ref; the tokens YELLOW on memory/acme.md is absent because
   --changed narrows to the staged file, which is the hook's design. The
   in-CI gate is the new "action" job in ci.yml (uses: ./ on examples/broken,
   asserts outcome == failure); its run id is cited at the tag.
G-all CHECK gofmt -l empty; go vet clean; go test ./... ok (cli, config,
   lint, report); fixture-broken 10 red 4 yellow; fixture-clean clean
   (6 rules, 9 files); fixture-dupids 2 red 0 yellow; examples
   minimal/shared-instructions/decision-log clean, broken 1 red 1 yellow;
   README output block matches (TestReadmeOutputMatchesExample).
G-FBOS CHECK `check --strict ~/Documents/promptkits` EXPECT clean (8 rules,
   618 files, tree 94405c3993ed), exit 0, one INFO ids/known-duplicate
   line that predates this build; same with the flags after the path.
G-CI CHECK CI run 34639081021 EXPECT success: test (ubuntu), test (macos),
   action — all green on the first push. Release run 34639549431 green.
