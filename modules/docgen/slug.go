// Package docgen is the first "link" product of the documentation database:
// one Markdown file per topic, named by a slug, plus an index. Content is
// rendered by re-parsing each segment's raw marker text with the scanner (the
// database stores source fragments, not a second representation), and every
// reference that resolves becomes a link to the target's file; included
// notes are linked, not expanded (that is a later step). No grouping yet.
package docgen

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// maxStem bounds a file name stem; longer ones are cut and hashed so the
// result stays unique and well under path limits.
const maxStem = 100

// slugOf builds the file stem of a topic from its kind and normalized key
// using only a-z 0-9 _ - . : the key is lower-cased, ':' and "::" become '.'
// (Class:member -> class.member, ns::name -> ns.name), any other character
// becomes '-', runs of separators collapse and the ends are trimmed. Case is
// dropped on purpose - Windows file systems would merge Foo.md and FOO.md
// anyway - and Slugs adds a suffix to the topics that then collide.
func slugOf(kind, key string) string {
	var b strings.Builder
	prev := '-'
	for _, r := range strings.ToLower(key) {
		var c rune
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			c = r
		case r == ':':
			c = '.'
		default:
			c = '-'
		}
		if (c == '-' || c == '.') && (prev == '-' || prev == '.') {
			continue
		}
		b.WriteRune(c)
		prev = c
	}
	s := strings.Trim(b.String(), "-.")
	if s == "" {
		s = "x"
	}
	if len(s) > maxStem {
		s = s[:maxStem-7] + "-" + hash6(key)
	}
	return kind + "-" + s
}

// hash6 is a short deterministic discriminator for a string.
func hash6(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return fmt.Sprintf("%06x", h.Sum32()&0xFFFFFF)
}

// SlugWarning reports an explicit-slug problem on a topic (an invalid name,
// or a name several topics claim); the caller attaches the provenance.
type SlugWarning struct {
	ID  int64
	Msg string
}

// ValidSlug reports whether an explicit slug (a `| slug: name` field) uses
// only the file-name alphabet a-z 0-9 _ - . and is not empty. Explicit slugs
// are compared and used lower-cased.
func ValidSlug(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// Slugs assigns every topic a unique file stem. An EXPLICIT slug (the topic's
// own `| slug: name`, lower-cased) wins over the computed one; a computed slug
// that collides with an explicit one, or with another computed one, gets a
// hash of the exact kind|key appended (every member of the group, so the
// result never depends on order). Two topics with the same explicit slug is
// an authoring error: both are suffixed and reported in the warnings.
func Slugs(topics []Topic, explicit map[int64]string) (map[int64]string, []SlugWarning) {
	var warnings []SlugWarning
	type cand struct {
		t   Topic
		exp bool
	}
	groups := map[string][]cand{}
	for _, t := range topics {
		if e, ok := explicit[t.ID]; ok {
			e = strings.ToLower(strings.TrimSpace(e))
			if ValidSlug(e) {
				groups[e] = append(groups[e], cand{t, true})
				continue
			}
			warnings = append(warnings, SlugWarning{t.ID, fmt.Sprintf("%s %s: explicit slug %q is not a file name (a-z 0-9 _ - . only); using the computed one", t.Kind, t.Ident, e)})
		}
		s := slugOf(t.Kind, t.Key)
		groups[s] = append(groups[s], cand{t, false})
	}
	out := make(map[int64]string, len(topics))
	for s, g := range groups {
		if len(g) == 1 {
			out[g[0].t.ID] = s
			continue
		}
		nExp := 0
		for _, c := range g {
			if c.exp {
				nExp++
			}
		}
		for _, c := range g {
			switch {
			case c.exp && nExp == 1:
				out[c.t.ID] = s // the one explicit owner keeps the name
			case c.exp:
				out[c.t.ID] = s + "-" + hash6(c.t.Kind+"|"+c.t.Key)
				warnings = append(warnings, SlugWarning{c.t.ID, fmt.Sprintf("%s %s: explicit slug %q is used by %d topics; suffixed", c.t.Kind, c.t.Ident, s, nExp)})
			default:
				out[c.t.ID] = s + "-" + hash6(c.t.Kind+"|"+c.t.Key)
			}
		}
	}
	return out, warnings
}
