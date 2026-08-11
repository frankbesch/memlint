# Memory index

Live pointers, all of which resolve:

- `memory/existing.md` - the canonical note
- [the same note](memory/existing.md) - markdown link form
- `memory/existing.md#notes` - anchored form, base file resolves
- `docs/CLAUDE.md` - the mirrored contract

Skipped by design, and still not findings in a clean repository:

- `reading/daily` - root not listed in config
- https://example.com/notes - a URL
- `reviews/YYYY-MM-DD-<topic>.html` - a placeholder
- `memory/gone.md#a#b` - more than one "#", not a path+anchor
