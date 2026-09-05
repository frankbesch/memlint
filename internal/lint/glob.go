package lint

import (
	"regexp"
	"strings"
	"sync"
)

// globMatch reports whether rel matches pattern. It is path.Match plus one
// addition: a whole-segment "**" matches zero or more path segments, so
// "memory/**/*.md" covers memory/a.md and memory/deep/er/a.md alike. Every
// other metacharacter keeps path.Match's meaning — "*" and "?" never cross a
// "/", "[...]" is a class, "." is literal. Patterns are validated at config
// load (config.validGlob), so an unparsable one cannot reach here.
func globMatch(pattern, rel string) bool {
	return globRegexp(pattern).MatchString(rel)
}

var (
	globCacheMu sync.Mutex
	globCache   = map[string]*regexp.Regexp{}
)

func globRegexp(pattern string) *regexp.Regexp {
	globCacheMu.Lock()
	defer globCacheMu.Unlock()
	if re, ok := globCache[pattern]; ok {
		return re
	}
	re := regexp.MustCompile(GlobToRegexp(pattern))
	globCache[pattern] = re
	return re
}

// GlobToRegexp translates a glob into an anchored regular expression. Exported
// so the config loader can validate with the same translator the checker
// matches with — one grammar, not two.
func GlobToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	segs := strings.Split(pattern, "/")
	for i, seg := range segs {
		last := i == len(segs)-1
		if seg == "**" {
			if last {
				b.WriteString(".*")
			} else {
				b.WriteString("(?:[^/]*/)*")
			}
			continue
		}
		b.WriteString(segmentRegexp(seg))
		if !last {
			b.WriteString("/")
		}
	}
	b.WriteString("$")
	return b.String()
}

func segmentRegexp(seg string) string {
	var b strings.Builder
	for i := 0; i < len(seg); i++ {
		switch c := seg[i]; c {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '[':
			j := strings.IndexByte(seg[i+1:], ']')
			if j < 0 {
				b.WriteString(regexp.QuoteMeta("["))
				continue
			}
			class := seg[i+1 : i+1+j]
			if strings.HasPrefix(class, "^") || strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + strings.ReplaceAll(class, `\`, `\\`) + "]")
			i += j + 1
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}
