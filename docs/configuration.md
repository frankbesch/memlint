# Configuration

`.memvet.toml`, at the root you point memvet at. **A section's presence is
what enables its rule.** Omit a section and that rule does not run.

```toml
[mirrors]
pairs = [
  ["CLAUDE.md", "docs/CLAUDE.md"],
  ["sync/left", "sync/right"],
]

[append_only]
files = ["memory/decisions.md"]

[blocks]
files = ["CLAUDE.md", "AGENTS.md"]
start = "<!-- AGENT:START -->"
end = "<!-- AGENT:END -->"

[human_brief]
files = ["INSTRUCTIONS.md"]
agent_authors = ["openwiki[bot]", "claude"]

[pointers]
files = ["memory/index.md"]
roots = ["memory", "docs"]

[junk]
globs = ["*.tmp", ".DS_Store"]

[tokens]
watch = ["memory/*.md"]
budget = 2000

[ids]
files = ["memory/decisions.md", "memory/archive/*.md"]
```

memvet rejects any key it does not recognize, along with empty sections,
absolute paths, paths containing `..`, invalid globs, and duplicate entries.
A typo in a config is a check that silently never ran, which is the one failure
mode a checker must not have.

A config with no sections at all is valid. It exits 0 and says
`memvet: clean (no rules enabled)` — never a bare "clean" that could be
mistaken for verification.


Each rule's keys are documented in [rules.md](rules.md). Ready-made configs for
common layouts are in [recipes.md](recipes.md).
