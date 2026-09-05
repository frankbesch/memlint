# fixture-rotated

An append-only log immediately after a rotation. `baseline/decisions.md` is
the log as last committed; `memory/decisions.md` is the working copy after
D-001 – D-003 were cut out and moved verbatim into
`memory/archive/decisions-vol1.md`, with the live log's header rewritten to
point at the volume.

Against that baseline memlint must report exactly **one INFO** finding —
`append_only/rotated`, three lines moved verbatim — and **no RED or YELLOW**:
the header change is covered by `header_lines = 4`, and the archive's missing
git baseline is established by the rotation itself.

The fixture cannot be checked as a static tree, because a committed file
always equals its own HEAD. `TestFixtureRotated` in
[internal/lint/fixtures_test.go](../../internal/lint/fixtures_test.go) builds
the git state: it commits `baseline/decisions.md` as `memory/decisions.md`,
then swaps in the rotated tree with the archive left untracked. The same test
alters one moved line in the archive and expects the allowance to withdraw:
RED `append_only/rewritten`, exactly as before v0.7.

Nothing in this directory is a real memory repo. Do not copy it as a template.
