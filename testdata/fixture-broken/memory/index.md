# Memory index

Live pointers:

- `memory/existing.md` - the canonical note
- [the same note](memory/existing.md) - markdown link form
- `memory/missing.md` - DEAD, planted defect
- `docs/nope.md` - DEAD, planted defect
- `memory/gone-anchored.md#section` - DEAD base file, planted defect (anchored since v0.6)

Must not be reported:

- `reading/daily` - root not listed in config
- https://example.com/notes - a URL
- https://example.com/notes#section - a URL with a fragment
- `reviews/YYYY-MM-DD-<topic>.html` - a placeholder
- `memory/multi-hash.md#a#b` - more than one "#", not a path+anchor
- `memory/missing.md#section` - anchored, dedups with the bare ref above
- `memory/existing.md#overview` - anchored, base file exists
