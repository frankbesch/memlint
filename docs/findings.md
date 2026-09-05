# Finding codes

Every memlint finding carries a stable machine code, `<rule>/<kind>`. This
page is the code's home. Each entry opens with a plain-English line — what
happened and what to do — followed by the exact mechanics for readers who
want them. JSON output links here via `doc_url`; text output prints the code
in brackets after each message.

Codes are stable: messages may be reworded, codes may not. Additions are
non-breaking; renaming or removing one bumps the JSON `schema_version`.

Three severities. **RED** fails the run. **YELLOW** is advisory and fails
only under `--strict`. **INFO** (green) is a receipt for something memlint
verified and wants you to see; it never fails the run, and a run with only
INFO lines still reports clean.

## unverifiable

**RED** · any rule. memlint was asked to check this file and physically
could not — usually a permission or disk problem. It reports that instead of
guessing. Fix the underlying problem, or stop listing the path.

Unverifiable is deliberately a failure, never a skip: a checker that stays
quiet about a check it could not run would report clean without meaning it.
Every rule emits its own spelling (`mirrors/unverifiable`,
`pointers/unverifiable`, …); they all mean this.

## mirrors/differ

**RED** · Two files that must stay identical copies have drifted apart.
Decide which one is right and copy it over the other.

Comparison is byte-wise; the finding names the first differing byte with its
line and column, plus both file sizes. memlint never autofixes.

## mirrors/one-sided

**RED** · A file exists in one mirrored folder but not in its twin. Create
or delete the counterpart — deliberately.

Directory pairs compare their `*.md` membership; each unmatched member is
reported from the side it exists on.

## mirrors/missing

**RED** · Something listed in `[mirrors]` does not exist at all. Restore it
or remove the pair from the config.

## mirrors/kind-mismatch

**RED** · One side of the pair is a file and the other is a folder, so they
can never be compared. Fix the configured paths.

## mirrors/escape

**RED** · A configured path points outside the repository, where memlint
refuses to follow.

The escape can be lexical (`../`, absolute) or through a symlink; every rule
refuses both.

## append_only/rewritten

**RED** · A file that is only supposed to grow had older content edited or
deleted. Restore the original text, then re-append what you meant to add.

The committed baseline — `HEAD`, or `--base <ref>` when given — must remain
a prefix of the working copy (trailing-newline tolerant). The finding shows
the first divergent line as was/now.

## append_only/missing

**RED** · A file declared append-only is gone from the working tree.

## append_only/escape

**RED** · The configured path points outside the repository, where memlint
refuses to follow.

## append_only/no-baseline

**YELLOW** · There is no committed version to compare against yet, so this
file was not checked either way. Commit it to arm the check.

No git repository, or no blob at the chosen ref (a file created after
`--base` has nothing to have diverged from). YELLOW because the invariant
could not be established — not because it held. Withdrawn for a file that
is the destination of a detected rotation (see `append_only/rotated`): the
moved span is its baseline.

## append_only/rotated

**INFO** · An append-only log was rotated: a block of older entries left
this file and reappeared, unchanged, in another append-only file that is new
in this commit. Nothing to fix — this line is the receipt that the move was
verified. `<src> → <dst> (N lines moved verbatim)`.

Mechanics. The check first strips `header_lines` (default 0) from both the
baseline and the working copy — the header is the only span that may
change. If the body no longer begins with its baseline, memlint isolates the
cut: the baseline lines after the longest shared prefix and before the
shortest baseline tail the working copy still continues with. That cut,
whole lines only, must appear verbatim after the header of another file
listed under `[append_only]` that has no baseline at the ref (untracked, or
created after `--base`). Candidates are tried in config order; the first hit
wins and its own no-baseline YELLOW is withdrawn. A cut of only blank lines
never qualifies. Anything else — an altered or missing moved line, a
destination that is already committed, or none at all — is the plain
`append_only/rewritten`.

## blocks/no-markers

**RED** · The file is supposed to contain a marked agent-owned block, but
neither marker is present. Add the block, or stop listing the file.

## blocks/unterminated

**RED** · An agent-owned block opens but never closes. Add the end marker.

## blocks/end-without-start

**RED** · An agent-owned block closes without ever opening. Add the start
marker, or remove the stray end marker.

## blocks/end-before-start

**RED** · The block's close marker appears above its open marker. Reorder
them.

## blocks/duplicate-start

**RED** · The open marker appears more than once, so where the block starts
is ambiguous. Remove the extra.

## blocks/duplicate-end

**RED** · The close marker appears more than once, so where the block ends
is ambiguous. Remove the extra.

## blocks/missing

**RED** · A file declared to carry an ownership block does not exist.

## blocks/escape

**RED** · The configured path points outside the repository, where memlint
refuses to follow.

## human_brief/agent-commit

**RED** · A file that must stay human-written has an AI-authored commit in
its history. What to do about that — rewrite history, recreate the file,
or accept it and drop the rule — is a human call; editing the working tree
cannot clear it.

Both the commit author and every `Co-Authored-By:` trailer are checked
against the configured identities, by name or email, case-insensitive and
exact. The finding names the most recent offending commit and counts the
earlier ones.

## human_brief/no-baseline

**YELLOW** · There is no git history to inspect, so authorship could not be
verified either way.

No repository, no commits, or no commit touches this file.

## human_brief/escape

**RED** · The configured path points outside the repository, where memlint
refuses to follow.

## pointers/dead-ref

**RED** · One of your notes references a file that does not exist. Fix the
reference, or bring the file back.

References are extracted from inline code spans, markdown link destinations,
and bare tokens, then checked when their first path segment is in `roots`.
For anchored references (`memory/notes.md#section`) the base file is what
was checked. Reported once per source file, at the first occurrence.

## pointers/missing-source

**RED** · A file memlint was told to scan for references is itself missing.

A literal `files` entry that is gone is an error, not a skip — unlike a
glob, which reports `pointers/no-match` instead.

## pointers/no-match

**YELLOW** · A `files` pattern matches nothing, so the coverage it declares
never runs. Rename it to match reality, or remove it.

Globs match root-relative paths only, never basenames.

## pointers/escape

**RED** · A configured source path points outside the repository, where
memlint refuses to follow.

## junk/match

**YELLOW** · A file matches your "should not be here" list — a `.DS_Store`,
a `*.tmp`. Delete it, or narrow the glob if the match is intentional.

Junk globs match both the basename and the root-relative path; a matching
directory is reported once, then skipped.

## tokens/over-budget

**YELLOW** · A watched note has grown past its size budget. Trim it, split
it, or raise the budget on purpose. When `limit` is also set, this is the
soft tier of two — the early warning, not the gate.

The estimate is one token per four runes, rounded up — an approximation,
reported as one; memlint does not run a tokenizer.

## tokens/over-limit

**RED** · A watched note has grown past its hard token limit — the tier
above the budget. Trim it or split it now; raising `limit` should be a
deliberate recalibration, not a reflex.

Set `limit` well above `budget` so the yellow band gives real warning
time. A file over both tiers reports once, at this severity.

## tokens/no-match

**YELLOW** · A watch pattern matches nothing, so the budget check it
declares never runs. Rename it to match reality, or remove it.

Watch globs match root-relative paths only, never basenames.

## ids/duplicate

**RED** · The same id opens a line in two places — two entries claim to be
`D-102`. Every citation of that id is now ambiguous. Give the later entry a
fresh id (or, if it is a straight copy, remove it) and fix anything that
cited it.

Ids are matched per line with the `[ids]` pattern (default
`^(D-\d{3}) \|`, an entry line with its delimiter), and only a match
starting at column 1 counts — a mid-line "see D-001" is a citation, not an
id. The id is the pattern's first capture group. "First"
is the earliest occurrence in source order — literal `files` entries in
config order, then glob matches in walk order — then by line; every later
occurrence is its own finding citing that first one. Gaps (a withdrawn id)
are not findings.

The delimiter is in the default on purpose: in a log whose entries wrap, a
continuation line can begin with a cited id at column 1 (`D-102). (2) …`),
and a bare `^(D-\d{3})` reads it as an entry. A log with a different entry
shape sets its own `pattern`; the column-1 rule still applies.

## ids/missing-source

**RED** · A file named literally in `[ids]` does not exist. Fix the path or
remove it; a glob that matches nothing is the YELLOW `ids/no-match`
instead.

## ids/no-match

**YELLOW** · A `[ids]` files glob matches nothing, so the uniqueness check
it declares covers nothing. Rename it to match reality, or remove it.

Globs match root-relative paths only, never basenames — the `[pointers]`
files rationale.

## ids/escape

**RED** · A configured path points outside the repository, where memlint
refuses to follow.
