# Finding codes

Every memlint finding carries a stable machine code, `<rule>/<kind>`. This
page is the code's home: what each one means and what to do about it. JSON
output links here via `doc_url`; text output prints the code in brackets
after each message.

Codes are stable: messages may be reworded, codes may not. Additions are
non-breaking; renaming or removing one bumps the JSON `schema_version`.

## unverifiable

**RED** · any rule. The rule was asked to check this path and could not —
an unreadable file, a permission error, a failed walk. Unverifiable is
deliberately a failure, never a skip: a checker that stays quiet about a
check it could not run would report clean without meaning it. Fix the
underlying I/O problem, or remove the path from the rule's configuration
if it should not be checked.

Every rule emits its own spelling (`mirrors/unverifiable`,
`pointers/unverifiable`, …); they all mean this.

## mirrors/differ

**RED**. The two sides of a configured pair are not byte-identical. The
finding names the first differing byte and its line/column. Decide which
side is canonical and copy it over the other — memlint never autofixes.

## mirrors/one-sided

**RED**. In a directory pair, a markdown file exists on one side with no
counterpart on the other. Create or delete the counterpart deliberately.

## mirrors/missing

**RED**. A configured mirror endpoint does not exist at all. Restore the
file or remove the pair from `[mirrors]`.

## mirrors/kind-mismatch

**RED**. One endpoint is a file and the other a directory. The pair as
configured can never be compared; fix the paths.

## mirrors/escape

**RED**. A configured endpoint resolves outside the repository root
(lexically or through a symlink). Mirrors only operate inside the tree.

## append_only/rewritten

**RED**. The baseline content (at `HEAD`, or at `--base <ref>` when
given) is no longer a prefix of the working copy: something edited or
deleted committed history in a file declared append-only. The finding
reports the first divergent line as was/now. Restore the original span
and re-append what you meant to add.

## append_only/missing

**RED**. A file declared append-only does not exist in the working tree.

## append_only/escape

**RED**. The configured path resolves outside the repository root.

## append_only/no-baseline

**YELLOW**. There is nothing to compare against: no git repository, or
the file has no committed baseline at the chosen ref. The invariant was
not violated — it could not be established. Commit the file, or pass a
`--base` the file exists at.

## blocks/no-markers

**RED**. A file declared to carry an ownership block contains neither
marker. Add the block, or remove the file from `[blocks]`.

## blocks/unterminated

**RED**. A start marker with no end marker after it.

## blocks/end-without-start

**RED**. An end marker with no start marker before it.

## blocks/end-before-start

**RED**. Both markers present, in the wrong order.

## blocks/duplicate-start

**RED**. More than one start marker; the block's extent is ambiguous.

## blocks/duplicate-end

**RED**. More than one end marker; the block's extent is ambiguous.

## blocks/missing

**RED**. A file declared to carry an ownership block does not exist.

## blocks/escape

**RED**. The configured path resolves outside the repository root.

## human_brief/agent-commit

**RED**. The file's git history contains a commit authored — or
co-authored via a `Co-Authored-By:` trailer — by a configured agent
identity. The finding names the most recent offending commit and counts
the earlier ones. History cannot be un-written by editing the working
tree; deciding what to do about it (rewrite, re-create the file, accept
and remove the rule) is a human call.

## human_brief/no-baseline

**YELLOW**. No git repository, no commits, or no commit touches this
file: authorship could not be established either way.

## human_brief/escape

**RED**. The configured path resolves outside the repository root.

## pointers/dead-ref

**RED**. A reference extracted from a pointer source names a path that
does not exist. For anchored references (`memory/notes.md#section`) the
base file is what was checked. Fix the reference or restore the target.

## pointers/missing-source

**RED**. A literal `files` entry — a file declared to be scanned for
references — does not exist. A named file that is gone is an error, not
a skip.

## pointers/no-match

**YELLOW**. A `files` glob matched nothing. A stale source glob is
pointer coverage that silently never runs — rename it to match reality
or remove it.

## pointers/escape

**RED**. A configured source path resolves outside the repository root.

## junk/match

**YELLOW**. A file or directory matches a configured junk glob. Delete
it, or narrow the glob if the match is intentional.

## tokens/over-budget

**YELLOW**. A watched file's estimated token count (one token per four
runes, rounded up — an estimate, not a tokenizer) exceeds the budget.
Split or prune the file, or raise the budget deliberately.

## tokens/no-match

**YELLOW**. A watch glob matched nothing. A stale watch glob is a budget
check that silently never runs — rename it to match reality or remove it.
