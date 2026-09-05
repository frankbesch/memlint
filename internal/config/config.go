// Package config loads and validates .memlint.toml.
//
// Section presence is the enable switch: a rule runs only if its section is
// present in the file. A nil section pointer means "disabled", which is why
// every section is a pointer type.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// FileName is the config file memlint looks for at the target root.
const FileName = ".memlint.toml"

// Config mirrors the .memlint.toml schema. Every field is a pointer so that
// an absent section is distinguishable from an empty one.
type Config struct {
	Mirrors    *Mirrors    `toml:"mirrors"`
	AppendOnly *AppendOnly `toml:"append_only"`
	Blocks     *Blocks     `toml:"blocks"`
	HumanBrief *HumanBrief `toml:"human_brief"`
	Pointers   *Pointers   `toml:"pointers"`
	Junk       *Junk       `toml:"junk"`
	Tokens     *Tokens     `toml:"tokens"`
	IDs        *IDs        `toml:"ids"`
	Stamps     *Stamps     `toml:"stamps"`
	Secrets    *Secrets    `toml:"secrets"`
}

// Stamps requires each listed file to carry a last-verified date stamp no
// older than MaxAgeDays relative to the file's last change.
type Stamps struct {
	Files      []string `toml:"files"`
	MaxAgeDays int      `toml:"max_age_days"`
	// Pattern must capture a YYYY-MM-DD date in its first group.
	Pattern string `toml:"pattern"`
}

// DefaultStampPattern matches "Last verified: 2026-09-05" and close variants.
const DefaultStampPattern = `(?i)last[ -]verified:?\s*(\d{4}-\d{2}-\d{2})`

// EffectivePattern is Pattern, or the default.
func (s *Stamps) EffectivePattern() string {
	if s.Pattern == "" {
		return DefaultStampPattern
	}
	return s.Pattern
}

// Secrets scans files matching Globs for credential-shaped text: built-in
// detectors plus any extra Patterns.
type Secrets struct {
	Globs    []string `toml:"globs"`
	Patterns []string `toml:"patterns"`
}

// Mirrors requires the two sides of each pair to stay byte-identical.
type Mirrors struct {
	Pairs [][]string `toml:"pairs"`
}

// AppendOnly requires each file to still begin with its baseline: the content
// committed at BaseRef, or HEAD when no base was given.
type AppendOnly struct {
	Files []string `toml:"files"`

	// HeaderLines marks the first N lines of every listed file — in the
	// baseline and in the working copy alike — as the one span that may
	// change: a live log's pointer header is rewritten at each rotation.
	// Everything after line N is immutable, or must move verbatim into
	// another listed file (the rotation allowance). Zero, the default, exempts
	// nothing and keeps pre-v0.7 behavior exactly.
	HeaderLines int `toml:"header_lines"`

	// Headers overrides HeaderLines per file (path = N). Keys must name
	// listed files. An archive volume with a longer or shorter header than
	// the live log gets its own window instead of inheriting the shared one.
	Headers map[string]int `toml:"headers"`

	// BaseRef is runtime state, not configuration: the CLI sets it from
	// --base after Load. It is deliberately not a TOML key — the right
	// baseline is invocation-specific (HEAD locally, the base branch in PR
	// CI), and a config would hard-code one context's answer into every
	// context. Empty means HEAD.
	BaseRef string `toml:"-"`
}

// Blocks requires each file to contain exactly one well-formed ownership
// block delimited by the Start and End markers.
type Blocks struct {
	Files []string `toml:"files"`
	Start string   `toml:"start"`
	End   string   `toml:"end"`

	// Mirror additionally requires the content between the markers to be
	// identical in every listed file; the first listed file is the reference.
	// Text outside the block may differ freely.
	Mirror bool `toml:"mirror"`
}

// HumanBrief requires that no commit in a file's git history was authored by
// one of the configured agent identities.
type HumanBrief struct {
	Files        []string `toml:"files"`
	AgentAuthors []string `toml:"agent_authors"`

	// FollowRenames scans history across renames (git log --follow), so an
	// agent commit to the brief under an earlier name stays visible. Off by
	// default: pre-rename history is then a different file, as before.
	FollowRenames bool `toml:"follow_renames"`
}

// Pointers checks repo-path-like references found in Files, but only when the
// reference's first path segment appears in Roots. A Files entry containing
// glob metacharacters is a pattern matched against root-relative paths; the
// rest are literal paths.
type Pointers struct {
	Files []string `toml:"files"`
	Roots []string `toml:"roots"`
}

// Junk reports files and directories matching any of Globs.
type Junk struct {
	Globs []string `toml:"globs"`
}

// Tokens reports watched files whose estimated token count exceeds Budget
// (YELLOW) and, when Limit is set, files past Limit (RED). Limit is the
// optional hard tier: zero or absent disables it, and a set Limit must be
// greater than Budget or the yellow band between the tiers would be empty.
type Tokens struct {
	Watch  []string `toml:"watch"`
	Budget int      `toml:"budget"`
	Limit  int      `toml:"limit"`
}

// IDs requires every id at the start of a line, across all listed files, to
// be unique. Files accepts literals and globs, resolved like [pointers]
// files. Pattern is matched per line and must match at column 1; its first
// capture group is the id (the whole match when there is none). Gaps are
// not findings: a withdrawn id is legitimately absent.
//
// The default pattern requires the entry delimiter ("D-### | ..."), not just
// the id: in a log whose entries wrap by hand, a continuation line can begin
// with a cited id at column 1, and the bare form read eleven such citations
// as entries on the first real run (ruled 2026-09-05).
type IDs struct {
	Files   []string `toml:"files"`
	Pattern string   `toml:"pattern"`

	// Known lists ids whose collision is recorded and reconciled elsewhere
	// (an append-only log cannot edit either entry away). A known id's
	// duplicates report as INFO ids/known-duplicate instead of RED, so the
	// receipt stays visible without failing the run. A known id that never
	// collides is YELLOW ids/known-unused: a stale allowlist entry.
	Known []string `toml:"known"`

	// CitedIn lists files (literals and globs) in which every cited id —
	// CitePattern anywhere in a line — must exist as an entry in Files. The
	// log's own prose is not checked unless it is listed here too.
	CitedIn []string `toml:"cited_in"`
	// CitePattern finds citations; its first capture group (or whole match)
	// is the id. Default: the D-### form, word-bounded.
	CitePattern string `toml:"cite_pattern"`

	// Ordered requires entries within each file to be non-decreasing by the
	// number in the id, so a paste into the middle of a log is caught.
	Ordered bool `toml:"ordered"`
}

// DefaultIDPattern is the decisions-log entry form: "D-### | ..." at the
// start of a line. The delimiter is part of the match on purpose (see IDs).
const DefaultIDPattern = `^(D-\d{3}) \|`

// DefaultCitePattern is the D-### form anywhere in a line, word-bounded so
// D-1000 is not read as D-100.
const DefaultCitePattern = `\b(D-\d{3})\b`

// EffectiveCitePattern is CitePattern, or the default.
func (i *IDs) EffectiveCitePattern() string {
	if i.CitePattern == "" {
		return DefaultCitePattern
	}
	return i.CitePattern
}

// EffectivePattern is Pattern, or the default when none was configured.
func (i *IDs) EffectivePattern() string {
	if i.Pattern == "" {
		return DefaultIDPattern
	}
	return i.Pattern
}

// RuleCount reports how many rules are enabled. Zero is valid: every rule is
// simply disabled, and the run reports clean without claiming to have verified
// anything.
func (c *Config) RuleCount() int {
	n := 0
	for _, enabled := range []bool{
		c.Mirrors != nil, c.AppendOnly != nil, c.Blocks != nil,
		c.HumanBrief != nil, c.Pointers != nil,
		c.Junk != nil, c.Tokens != nil, c.IDs != nil,
		c.Stamps != nil, c.Secrets != nil,
	} {
		if enabled {
			n++
		}
	}
	return n
}

// Load reads and validates <root>/.memlint.toml. Every error it returns is a
// startup error: the caller must exit 2 without running any rule.
func Load(root string) (*Config, error) {
	path := filepath.Join(root, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s found at %s", FileName, root)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg Config
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("%s: unknown key(s): %s", FileName, strings.Join(keys, ", "))
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Mirrors != nil {
		if err := c.Mirrors.validate(); err != nil {
			return fmt.Errorf("[mirrors]: %w", err)
		}
	}
	if c.AppendOnly != nil {
		if err := c.AppendOnly.validate(); err != nil {
			return fmt.Errorf("[append_only]: %w", err)
		}
	}
	if c.Blocks != nil {
		if err := c.Blocks.validate(); err != nil {
			return fmt.Errorf("[blocks]: %w", err)
		}
	}
	if c.HumanBrief != nil {
		if err := c.HumanBrief.validate(); err != nil {
			return fmt.Errorf("[human_brief]: %w", err)
		}
	}
	if c.Pointers != nil {
		if err := c.Pointers.validate(); err != nil {
			return fmt.Errorf("[pointers]: %w", err)
		}
	}
	if c.Junk != nil {
		if err := c.Junk.validate(); err != nil {
			return fmt.Errorf("[junk]: %w", err)
		}
	}
	if c.Tokens != nil {
		if err := c.Tokens.validate(); err != nil {
			return fmt.Errorf("[tokens]: %w", err)
		}
	}
	if c.IDs != nil {
		if err := c.IDs.validate(); err != nil {
			return fmt.Errorf("[ids]: %w", err)
		}
	}
	if c.Stamps != nil {
		if err := c.Stamps.validate(); err != nil {
			return fmt.Errorf("[stamps]: %w", err)
		}
	}
	if c.Secrets != nil {
		if err := c.Secrets.validate(); err != nil {
			return fmt.Errorf("[secrets]: %w", err)
		}
	}
	return nil
}

func (s *Stamps) validate() error {
	if len(s.Files) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set files and max_age_days")
	}
	if err := validMixedPathList("files", s.Files); err != nil {
		return err
	}
	if s.MaxAgeDays <= 0 {
		return fmt.Errorf("max_age_days: must be a positive number of days, got %d", s.MaxAgeDays)
	}
	re, err := regexp.Compile(s.EffectivePattern())
	if err != nil {
		return fmt.Errorf("pattern: invalid regular expression %q: %w", s.Pattern, err)
	}
	if re.NumSubexp() < 1 {
		return fmt.Errorf("pattern: must capture the date in a group, got %q", s.Pattern)
	}
	return nil
}

func (s *Secrets) validate() error {
	if len(s.Globs) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set globs")
	}
	if err := validGlobList("globs", s.Globs); err != nil {
		return err
	}
	for i, p := range s.Patterns {
		if _, err := regexp.Compile(p); err != nil {
			return fmt.Errorf("patterns[%d]: invalid regular expression %q: %w", i, p, err)
		}
	}
	return nil
}

func (m *Mirrors) validate() error {
	if len(m.Pairs) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set pairs")
	}
	seen := map[string]bool{}
	for i, pair := range m.Pairs {
		if len(pair) != 2 {
			return fmt.Errorf("pairs[%d]: expected exactly 2 paths, got %d", i, len(pair))
		}
		for j, p := range pair {
			if err := validRelPath(p); err != nil {
				return fmt.Errorf("pairs[%d][%d]: %w", i, j, err)
			}
		}
		a, b := CleanRel(pair[0]), CleanRel(pair[1])
		if a == b {
			return fmt.Errorf("pairs[%d]: both sides resolve to the same path %q", i, a)
		}
		// [a,b] and [b,a] describe the same check.
		key := a + "\x00" + b
		if a > b {
			key = b + "\x00" + a
		}
		if seen[key] {
			return fmt.Errorf("pairs[%d]: duplicate pair %q / %q", i, a, b)
		}
		seen[key] = true
	}
	return nil
}

func (a *AppendOnly) validate() error {
	if len(a.Files) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set files")
	}
	if a.HeaderLines < 0 {
		return fmt.Errorf("header_lines: must be zero or a positive line count, got %d", a.HeaderLines)
	}
	if err := validRelPathList("files", a.Files); err != nil {
		return err
	}
	listed := map[string]bool{}
	for _, f := range a.Files {
		listed[CleanRel(f)] = true
	}
	for k, n := range a.Headers {
		if !listed[CleanRel(k)] {
			return fmt.Errorf("headers: %q is not in files", k)
		}
		if n < 0 {
			return fmt.Errorf("headers: %q must be zero or a positive line count, got %d", k, n)
		}
	}
	return nil
}

// HeaderFor is the header window for rel: the per-file override, else the
// shared header_lines.
func (a *AppendOnly) HeaderFor(rel string) int {
	for k, n := range a.Headers {
		if CleanRel(k) == rel {
			return n
		}
	}
	return a.HeaderLines
}

func (b *Blocks) validate() error {
	if len(b.Files) == 0 || b.Start == "" || b.End == "" {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set files, start, and end")
	}
	if err := validRelPathList("files", b.Files); err != nil {
		return err
	}
	for _, m := range []struct{ key, val string }{{"start", b.Start}, {"end", b.End}} {
		if strings.ContainsAny(m.val, "\r\n") {
			return fmt.Errorf("%s: marker must be a single line, got %q", m.key, m.val)
		}
	}
	if b.Start == b.End {
		return fmt.Errorf("start and end markers must differ, both are %q", b.Start)
	}
	// A marker containing the other would make every occurrence of the longer
	// marker also count as the shorter one, so the scan could never be trusted.
	if strings.Contains(b.Start, b.End) || strings.Contains(b.End, b.Start) {
		return fmt.Errorf("markers must not contain each other: %q / %q", b.Start, b.End)
	}
	return nil
}

func (h *HumanBrief) validate() error {
	if len(h.Files) == 0 || len(h.AgentAuthors) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set files and agent_authors")
	}
	if err := validRelPathList("files", h.Files); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, a := range h.AgentAuthors {
		if strings.TrimSpace(a) == "" {
			return fmt.Errorf("agent_authors[%d]: must not be empty", i)
		}
		// Matching is case-insensitive, so duplicates are too.
		key := strings.ToLower(a)
		if seen[key] {
			return fmt.Errorf("agent_authors[%d]: duplicate identity %q", i, a)
		}
		seen[key] = true
	}
	return nil
}

func (p *Pointers) validate() error {
	if len(p.Files) == 0 || len(p.Roots) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set files and roots")
	}
	if err := validMixedPathList("files", p.Files); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, r := range p.Roots {
		switch {
		case r == "":
			return fmt.Errorf("roots[%d]: must not be empty", i)
		case filepath.IsAbs(r):
			return fmt.Errorf("roots[%d]: must not be absolute: %q", i, r)
		case strings.ContainsAny(r, `/\`):
			return fmt.Errorf("roots[%d]: must be a single path segment, got %q", i, r)
		case r == "." || r == "..":
			return fmt.Errorf("roots[%d]: invalid segment %q", i, r)
		case seen[r]:
			return fmt.Errorf("roots[%d]: duplicate root %q", i, r)
		}
		seen[r] = true
	}
	return nil
}

func (j *Junk) validate() error {
	if len(j.Globs) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set globs")
	}
	return validGlobList("globs", j.Globs)
}

func (t *Tokens) validate() error {
	if len(t.Watch) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set watch")
	}
	if err := validGlobList("watch", t.Watch); err != nil {
		return err
	}
	if t.Budget <= 0 {
		return fmt.Errorf("budget: must be a positive number of tokens, got %d", t.Budget)
	}
	if t.Limit != 0 && t.Limit <= t.Budget {
		return fmt.Errorf("limit: must be greater than budget (%d) when set, got %d", t.Budget, t.Limit)
	}
	return nil
}

func (i *IDs) validate() error {
	if len(i.Files) == 0 {
		return fmt.Errorf("section is present but empty; remove it to disable the rule, or set files")
	}
	if err := validMixedPathList("files", i.Files); err != nil {
		return err
	}
	// An empty pattern is indistinguishable from an absent one after
	// decoding; both mean the default.
	if _, err := regexp.Compile(i.EffectivePattern()); err != nil {
		return fmt.Errorf("pattern: invalid regular expression %q: %w", i.Pattern, err)
	}
	if _, err := regexp.Compile(i.EffectiveCitePattern()); err != nil {
		return fmt.Errorf("cite_pattern: invalid regular expression %q: %w", i.CitePattern, err)
	}
	if err := validMixedPathList("cited_in", i.CitedIn); err != nil {
		return err
	}
	seen := map[string]bool{}
	for n, k := range i.Known {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("known[%d]: must not be empty", n)
		}
		if seen[k] {
			return fmt.Errorf("known[%d]: duplicate id %q", n, k)
		}
		seen[k] = true
	}
	return nil
}

// IsGlob reports whether a configured entry is a pattern rather than a
// literal path. The metacharacter set is filepath.Match's, plus ** as a
// whole-segment recursive wildcard.
func IsGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// validMixedPathList accepts a list where each entry is either a literal
// relative path or a glob, applying the matching validation to each.
func validMixedPathList(field string, entries []string) error {
	seen := map[string]bool{}
	for i, e := range entries {
		key := e
		if IsGlob(e) {
			if err := validGlob(e); err != nil {
				return fmt.Errorf("%s[%d]: %w", field, i, err)
			}
		} else {
			if err := validRelPath(e); err != nil {
				return fmt.Errorf("%s[%d]: %w", field, i, err)
			}
			key = CleanRel(e)
		}
		if seen[key] {
			return fmt.Errorf("%s[%d]: duplicate entry %q", field, i, key)
		}
		seen[key] = true
	}
	return nil
}

func validRelPathList(field string, paths []string) error {
	seen := map[string]bool{}
	for i, p := range paths {
		if err := validRelPath(p); err != nil {
			return fmt.Errorf("%s[%d]: %w", field, i, err)
		}
		c := CleanRel(p)
		if seen[c] {
			return fmt.Errorf("%s[%d]: duplicate path %q", field, i, c)
		}
		seen[c] = true
	}
	return nil
}

// validRelPath rejects anything that could point outside the target root.
func validRelPath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("must not be empty")
	case filepath.IsAbs(p) || strings.HasPrefix(p, "/"):
		return fmt.Errorf("must be relative to the repository root, got %q", p)
	}
	c := CleanRel(p)
	if c == ".." || strings.HasPrefix(c, "../") || c == "." {
		return fmt.Errorf("must not escape the repository root, got %q", p)
	}
	return nil
}

func validGlobList(field string, globs []string) error {
	seen := map[string]bool{}
	for i, g := range globs {
		if err := validGlob(g); err != nil {
			return fmt.Errorf("%s[%d]: %w", field, i, err)
		}
		if seen[g] {
			return fmt.Errorf("%s[%d]: duplicate glob %q", field, i, g)
		}
		seen[g] = true
	}
	return nil
}

func validGlob(g string) error {
	if g == "" {
		return fmt.Errorf("must not be empty")
	}
	// ** is a whole-segment wildcard (zero or more directories). Inside a
	// segment it would be ambiguous — "a**b" is not "a*b" — so it is refused.
	for _, seg := range strings.Split(g, "/") {
		if strings.Contains(seg, "**") && seg != "**" {
			return fmt.Errorf("invalid glob %q: ** must be a whole path segment", g)
		}
		if _, err := filepath.Match(strings.ReplaceAll(seg, "**", "*"), "probe"); err != nil {
			return fmt.Errorf("invalid glob %q: %w", g, err)
		}
	}
	return nil
}

// CleanRel normalizes a configured path to slash-separated, cleaned form.
func CleanRel(p string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
}
