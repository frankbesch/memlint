# Oversized note

This note is deliberately larger than the token budget declared in
`.memlint.toml`. The tokens rule is advisory, so it produces a YELLOW finding
rather than a RED one: a file over budget is a cost and attention problem, not
a broken invariant. Nothing here is dead, mirrored, or append-only.

## Why a budget rule exists at all

A memory file is read into a model's context on every single turn. A note that
grows without anyone noticing quietly taxes every request made against the
repository it lives in, and the cost is invisible at the point where the growth
happens. Nobody edits a file and thinks about the aggregate. The rule exists to
put a number on that drift and to put the number somewhere a human will see it,
which is the same reason the rest of memlint exists.

## Why the count is an estimate

memlint does not run a tokenizer. It divides the character count by four and
rounds up, which is close enough to catch a file that doubled in size and not
close enough to argue about. Running a real tokenizer would mean picking a
model family, vendoring a vocabulary, and keeping both current, in exchange for
a number that would still be wrong for whichever model the reader actually
uses. The findings say "estimated tokens" for exactly this reason.

## What this file is not

It is not an example of a well-maintained note. It is padding with a purpose:
enough characters to cross the fixture's declared budget by a comfortable
margin, so that the acceptance test is not sitting one edit away from
flipping. If you are looking for the boundary behavior of the rule, that is in
the token tests, which check the exact budget edge in both directions.
