// Package docgen renders a compiled (and resolved) documentation database
// into loose reference Markdown: one .md per page, a page being a topic or a
// topic group (every topic that declared the same _tg_), plus an index.md.
//
// Rendering follows Draft 4 literally: a page is the concatenation of its
// segments in pos order; every field renders in written order as
// "**label:** value", a label-hidden field as its bare value, an entry-hidden
// field not at all, a "|:" field as its text; the identity is the title;
// include-note-id transcludes the note's rendered body in place (recursive,
// cycles cut); {{ilink: <target> text}} becomes a Markdown link to the target
// page. Multi-line values are dedented; consecutive list-item entries stay one
// list. The tool adds nothing else: the author writes the presentation.
package docgen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/srcdoc"
)

type field struct {
	label     string
	value     string
	hideEntry bool
	hideLabel bool
}

type segment struct {
	idseg  int64
	pos    int64
	line   int64
	src    string
	fields []field
	header bool // the first field is a topic identity
}

type topic struct {
	id       int64
	kind     string
	key      string
	ident    string
	slug     string
	idtg     int64
	segments []segment
}

type page struct {
	slug   string
	file   string
	title  string
	kind   string // topic kind, or "group"
	topics []*topic
}

type model struct {
	topics  map[int64]*topic
	byKey   map[string]*topic // kind + "\x00" + key
	pages   []*page
	pageOf  map[int64]*page  // topic id -> page
	bySlug  map[string]*page // lower slug -> page
	byGroup map[string]*page // group key -> page
	log     func(string)
}

var ilinkRe = regexp.MustCompile(`\{\{\s*ilink\s*:\s*<\s*([A-Za-z_-]+)\s+([^>]+?)\s*>\s*([^}]*)\}\}`)
var inlineRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:\s*([^}]*)\}\}`)

// Generate renders the database at dbPath into outDir (created if needed).
// It returns the number of pages written.
func Generate(dbPath, outDir string, log func(string)) (int, error) {
	db, err := docdb.Open(dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	m, err := load(db)
	if err != nil {
		return 0, err
	}
	m.log = log
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	for _, p := range m.pages {
		body := m.renderPage(p)
		if err := os.WriteFile(filepath.Join(outDir, p.file), []byte(crlf(body)), 0o644); err != nil {
			return 0, err
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.md"), []byte(crlf(m.renderIndex())), 0o644); err != nil {
		return 0, err
	}
	return len(m.pages), nil
}

func crlf(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// ---------------------------------------------------------------- loading

func load(db *docdb.DB) (*model, error) {
	m := &model{topics: map[int64]*topic{}, byKey: map[string]*topic{}, pageOf: map[int64]*page{},
		bySlug: map[string]*page{}, byGroup: map[string]*page{}}
	rows, err := db.QueryAll(`SELECT idtopic, kind, key, ident, slug, idtg FROM topics`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		t := &topic{id: r[0].(int64), kind: r[1].(string), key: r[2].(string), ident: r[3].(string), slug: r[4].(string), idtg: r[5].(int64)}
		m.topics[t.id] = t
		m.byKey[t.kind+"\x00"+t.key] = t
	}
	srcs := map[int64]string{}
	rows, err = db.QueryAll(`SELECT idsrc, src FROM sources`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		srcs[r[0].(int64)] = r[1].(string)
	}
	rows, err = db.QueryAll(`SELECT idseg, idtopic, idsrc, pos, line FROM segments ORDER BY pos`)
	if err != nil {
		return nil, err
	}
	segOwner := map[int64]*topic{}
	segAt := map[int64]int{}
	for _, r := range rows {
		t := m.topics[r[1].(int64)]
		if t == nil {
			continue
		}
		t.segments = append(t.segments, segment{idseg: r[0].(int64), src: srcs[r[2].(int64)], pos: r[3].(int64), line: r[4].(int64)})
		segOwner[r[0].(int64)] = t
		segAt[r[0].(int64)] = len(t.segments) - 1
	}
	rows, err = db.QueryAll(`SELECT idseg, seq, label, value, hide_entry, hide_label FROM fields ORDER BY idseg, seq`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		t := segOwner[r[0].(int64)]
		if t == nil {
			continue
		}
		s := &t.segments[segAt[r[0].(int64)]]
		f := field{label: r[2].(string), value: r[3].(string), hideEntry: r[4].(int64) != 0, hideLabel: r[5].(int64) != 0}
		if r[1].(int64) == 0 && srcdoc.HeaderKind(f.label) != "" {
			s.header = true
		}
		s.fields = append(s.fields, f)
	}
	// pages: groups first (they carry several topics), then loose topics
	type group struct {
		id   int64
		name string
		key  string
		slug string
		pos  int64
	}
	var groups []group
	rows, err = db.QueryAll(`SELECT idtg, name, key, slug, pos FROM topic_groups ORDER BY pos`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		groups = append(groups, group{r[0].(int64), r[1].(string), r[2].(string), r[3].(string), r[4].(int64)})
	}
	topicPos := func(t *topic) int64 {
		if len(t.segments) == 0 {
			return 0
		}
		return t.segments[0].pos
	}
	var pages []*page
	for _, g := range groups {
		p := &page{slug: g.slug, title: g.name, kind: "group"}
		for _, t := range m.topics {
			if t.idtg == g.id {
				p.topics = append(p.topics, t)
			}
		}
		sort.Slice(p.topics, func(i, j int) bool { return topicPos(p.topics[i]) < topicPos(p.topics[j]) })
		if len(p.topics) == 0 {
			continue
		}
		m.byGroup[g.key] = p
		pages = append(pages, p)
	}
	var loose []*topic
	for _, t := range m.topics {
		if t.idtg == 0 {
			loose = append(loose, t)
		}
	}
	sort.Slice(loose, func(i, j int) bool { return topicPos(loose[i]) < topicPos(loose[j]) })
	for _, t := range loose {
		pages = append(pages, &page{slug: t.slug, title: t.ident, kind: t.kind, topics: []*topic{t}})
	}
	// files: unique names, case-insensitively; a duplicate slug gets a suffix
	used := map[string]int{}
	for _, p := range pages {
		base := strings.ToLower(p.slug)
		if base == "" {
			base = "page"
		}
		n := used[base]
		used[base] = n + 1
		p.file = base + ".md"
		if n > 0 {
			p.file = fmt.Sprintf("%s-%d.md", base, n+1)
		}
		if _, dup := m.bySlug[base]; !dup {
			m.bySlug[base] = p
		}
		for _, t := range p.topics {
			m.pageOf[t.id] = p
		}
	}
	m.pages = pages
	return m, nil
}

// ---------------------------------------------------------------- rendering

func (m *model) renderPage(p *page) string {
	var b strings.Builder
	if p.kind == "group" {
		fmt.Fprintf(&b, "# %s\n\n", mdIdent(p.title))
		for _, t := range p.topics {
			b.WriteString(m.renderTopic(t, "## ", nil))
			b.WriteString("\n")
		}
		return b.String()
	}
	b.WriteString(m.renderTopic(p.topics[0], "# ", nil))
	return b.String()
}

// renderTopic renders a topic with its title at the given heading level; with
// an empty level (transclusion) the title is omitted. stack holds the topics
// being transcluded, to cut cycles.
func (m *model) renderTopic(t *topic, level string, stack []int64) string {
	var b strings.Builder
	if level != "" {
		fmt.Fprintf(&b, "%s%s\n\n", level, mdIdent(t.ident))
	}
	var entries []string
	for _, s := range t.segments {
		var segEntries []string
		var hidden []bool
		for i, f := range s.fields {
			if s.header && i == 0 {
				continue // the identity is the title
			}
			if f.hideEntry {
				continue
			}
			if f.label == "include-note-id" {
				segEntries = append(segEntries, m.transclude(strings.TrimSpace(f.value), append(stack, t.id)))
				hidden = append(hidden, false)
				continue
			}
			segEntries = append(segEntries, m.renderField(f))
			hidden = append(hidden, f.label == "" || f.hideLabel)
		}
		entries = append(entries, joinMarker(segEntries, hidden)...)
	}
	b.WriteString(joinEntries(entries))
	return b.String()
}

func (m *model) transclude(noteID string, stack []int64) string {
	n := m.byKey[srcdoc.KindNote+"\x00"+srcdoc.Key(srcdoc.KindNote, noteID)]
	if n == nil {
		return fmt.Sprintf("*(missing note %s)*", noteID)
	}
	for _, id := range stack {
		if id == n.id {
			return fmt.Sprintf("*(include cycle cut: %s)*", noteID)
		}
	}
	return strings.TrimRight(m.renderTopic(n, "", stack), "\n")
}

func (m *model) renderField(f field) string {
	// md regions first, on the text as written: the outer dedent must not
	// touch them (raw keeps its bytes) and their own dedent is per region
	v, restore := m.mdRegions(f.value)
	if strings.HasPrefix(v, "\n") {
		// the value starts on its own line: every line, the first included,
		// shares the indentation to strip
		v = srcdoc.Dedent(v)
		v = strings.TrimLeft(v, "\n")
	} else {
		v = srcdoc.Dedent(v)
	}
	v = restore(m.inlineText(v))
	if f.label == "" || f.hideLabel {
		return v
	}
	if strings.HasPrefix(f.value, "\n") { // "| params:" then the list on the next lines
		return "**" + f.label + ":**\n" + v
	}
	return "**" + f.label + ":** " + v
}

// joinMarker joins the entries of ONE marker: after a list-item entry, the
// entries that follow in the same marker continue that item on the same line
// (a hidden-label one prefixed by " - ", a labelled one by a blank), their
// continuation lines indented to stay inside the item. Anything else stays a
// separate entry.
func joinMarker(entries []string, hidden []bool) []string {
	var out []string
	for i, e := range entries {
		if i > 0 && isItem(entries[i-1]) && !isItem(e) && !strings.Contains(entries[i-1], "\n\n") && len(out) > 0 {
			sep := " "
			if hidden[i] {
				sep = " - "
			}
			e = strings.ReplaceAll(e, "\n", "\n  ")
			out[len(out)-1] += sep + e
			entries[i] = out[len(out)-1] // keep the joined text as the previous item
			continue
		}
		out = append(out, e)
	}
	return out
}

// inline replaces the inline markers of a value: ilinks become Markdown links,
// any other {{label: value}} renders as its bold label.
func (m *model) inline(v string) string {
	v, restore := m.mdRegions(v)
	return restore(m.inlineText(v))
}

// mdRe matches an inline {{begin-md[: attrs]}} ... {{end-md}} region.
var mdRe = regexp.MustCompile(`(?s)\{\{begin-md(?::([^}]*))?\}\}(.*?)\{\{end-md\}\}`)

// mdRegion renders one md region (a mdRe match): the marks disappear; with
// "raw" the text is kept byte for byte, otherwise the lines lose their common
// indentation and ilinks / inline markers are replaced. Attributes are a
// comma list after the colon: {{begin-md: raw}}.
func (m *model) mdRegion(s string) string {
	g := mdRe.FindStringSubmatch(s)
	raw := false
	for _, a := range strings.Split(g[1], ",") {
		if strings.TrimSpace(a) == "raw" {
			raw = true
		}
	}
	body := g[2]
	if raw {
		return body
	}
	lines := strings.Split(body, "\n")
	common := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	if common > 0 {
		for i, l := range lines {
			if strings.TrimSpace(l) == "" {
				lines[i] = ""
				continue
			}
			lines[i] = l[common:]
		}
	}
	return m.inlineText(strings.Join(lines, "\n"))
}

// mdRegions cuts the md regions out of a value before the rest is dedented
// and rendered, and puts their rendering back afterwards.
func (m *model) mdRegions(v string) (out string, restore func(string) string) {
	var rendered []string
	out = mdRe.ReplaceAllStringFunc(v, func(s string) string {
		rendered = append(rendered, m.mdRegion(s))
		return fmt.Sprintf("\x00%d\x00", len(rendered)-1)
	})
	return out, func(s string) string {
		for i, r := range rendered {
			s = strings.Replace(s, fmt.Sprintf("\x00%d\x00", i), r, 1)
		}
		return s
	}
}

// inlineText replaces the inline markers of a value: ilinks become Markdown
// links, any other {{label: value}} renders as its bold label.
func (m *model) inlineText(v string) string {
	v = ilinkRe.ReplaceAllStringFunc(v, func(s string) string {
		g := ilinkRe.FindStringSubmatch(s)
		kind, ident, text := g[1], strings.TrimSpace(g[2]), strings.TrimSpace(g[3])
		if text == "" {
			text = ident
		}
		if p := m.target(kind, ident); p != nil {
			return "[" + mdIdent(text) + "](" + p.file + ")"
		}
		return mdIdent(text)
	})
	return inlineRe.ReplaceAllStringFunc(v, func(s string) string {
		g := inlineRe.FindStringSubmatch(s)
		label, _, hideLabel := canon(g[1])
		if hideLabel {
			return strings.TrimSpace(g[2])
		}
		return "**" + label + ":** " + strings.TrimSpace(g[2])
	})
}

func canon(l string) (string, bool, bool) {
	he, hl := false, false
	if strings.HasPrefix(l, "_") && len(l) > 1 {
		he = true
		l = l[1:]
	}
	if strings.HasSuffix(l, "_") && len(l) > 1 {
		hl = true
		l = l[:len(l)-1]
	}
	return l, he, hl
}

func (m *model) target(kind, ident string) *page {
	switch kind {
	case "slug":
		return m.bySlug[strings.ToLower(ident)]
	case "tg":
		return m.byGroup[ident]
	}
	k := srcdoc.HeaderKind(kind)
	if k == "" {
		return nil
	}
	if t := m.byKey[k+"\x00"+srcdoc.Key(k, ident)]; t != nil {
		return m.pageOf[t.id]
	}
	return nil
}

// joinEntries separates entries with a blank line, except between two
// consecutive list items, which stay one list.
func joinEntries(entries []string) string {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			if isItem(entries[i-1]) && isItem(e) {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}
		b.WriteString(e)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// mdIdent protects an identity used as Markdown text: a leading or trailing
// "_" or "*" would read as emphasis (_LARGE_INTEGER_ loses its underscores),
// so such a name goes in a code span. Text the author already marked up
// (a code span, bold, a link, HTML) is left alone.
func mdIdent(s string) string {
	if s == "" || strings.HasPrefix(s, "`") || strings.HasPrefix(s, "**") || strings.HasPrefix(s, "[") || strings.HasPrefix(s, "<") {
		return s
	}
	if strings.HasPrefix(s, "_") || strings.HasSuffix(s, "_") || strings.HasPrefix(s, "*") || strings.HasSuffix(s, "*") {
		return "`" + s + "`"
	}
	return s
}

func isItem(s string) bool {
	s = strings.TrimLeft(s, " ")
	return strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ")
}

func (m *model) renderIndex() string {
	var b strings.Builder
	b.WriteString("# Index\n")
	kinds := append([]string{}, srcdoc.Kinds...)
	kinds = append(kinds, "group")
	for _, k := range kinds {
		var ps []*page
		for _, p := range m.pages {
			if p.kind == k {
				ps = append(ps, p)
			}
		}
		if len(ps) == 0 {
			continue
		}
		sort.Slice(ps, func(i, j int) bool { return strings.ToLower(ps[i].title) < strings.ToLower(ps[j].title) })
		fmt.Fprintf(&b, "\n## %s (%d)\n\n", k, len(ps))
		for _, p := range ps {
			fmt.Fprintf(&b, "- [%s](%s)\n", mdIdent(p.title), p.file)
		}
	}
	return b.String()
}
