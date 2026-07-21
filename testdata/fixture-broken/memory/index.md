# Memory index

Live pointers:

- `memory/existing.md` - the canonical note
- [the same note](memory/existing.md) - markdown link form
- `memory/missing.md` - DEAD, planted defect 4
- `docs/nope.md` - DEAD, planted defect 5

Must not be reported:

- `reading/daily` - root not listed in config
- https://example.com/notes - a URL
- `reviews/YYYY-MM-DD-<topic>.html` - a placeholder
- `memory/gone-anchored.md#section` - anchored, skipped in v0.1
- `memory/missing.md#section` - anchored, dedups with the bare ref above
