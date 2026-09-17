# Tree receipts

Every `check` summary ends with the first 12 hex of the tree it judged:

```text
memvet: clean (8 rules, 608 files checked, tree 0951f4c6ddbd)
```

`memvet fingerprint [path]` prints the full 64-hex value and nothing else.
It is the SHA-256 over every visible regular file's path, size, and content
hash, sorted by path — content-based, so a clone, checkout, or `touch` does
not move it, while one changed byte, one added file, or a config edit does.
Visible means what a commit could contain when git is present (tracked plus
untracked-not-ignored); without git, every file under the root except
`.git/`. `--format json` carries it as `summary.tree`.

The point is the receipt: a checked fact has a shelf life. A wrap that
records `clean … tree 0951f4c6ddbd` can be held to it later with
`memvet check --expect-tree 0951f4c6ddbd`, which turns a tree that moved
in between into one RED `tree/moved` and exit 1. memvet keeps no cache and
writes nothing; the caller stores the fingerprint. (Pattern borrowed from
Graft's fingerprint-before-query refresh.)
