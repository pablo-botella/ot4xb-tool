// Package docgen builds the pages of a compiled and resolved documentation
// database: a page is a topic or a topic group (every topic that declared
// the same _tg_), plus the index pages. Build returns every page twice over:
// as reference Markdown (Markdown, what gendoc writes, one .md per page) and
// as a tree (Body, what the HTML site renders) parsed from the field values
// with the strict subset of parse.go, ilinks resolved, notes transcluded.
//
// Both follow Draft 4 literally: a page is the concatenation of its segments
// in pos order; every field renders in written order as "label: value", a
// label-hidden field as its bare value, an entry-hidden field not at all, a
// "|:" field as its text; the identity is the title; include-note-id
// transcludes the note's body in place (recursive, cycles cut); {{ilink:
// <target> text}} becomes a link to the target page. Consecutive list-item
// entries stay one list. The tool adds nothing else: the author writes the
// presentation.
//
// The pages carry their logical location too (Trail, Parent, Siblings),
// read from the tables Materialize writes when resolve runs: a database
// that has not been resolved cannot be built.
package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/doctool"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/scan"
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
	cats     []string // categories, in declaration order
	books    []string // books named in the "book:" field
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
	issues  []Issue // what the sources say outside the subset, while building
	// trails records, while the indexes are rendered, where every page is
	// listed first: its logical location (file -> crumbs)
	trails map[string][]Crumb
}

// place records the logical location of a page the first time it is listed.
func (m *model) place(file string, trail []Crumb) {
	if m.trails == nil {
		m.trails = map[string][]Crumb{}
	}
	if _, done := m.trails[file]; !done {
		m.trails[file] = append([]Crumb(nil), trail...)
	}
}

var ilinkRe = regexp.MustCompile(`\{\{\s*ilink\s*:\s*<\s*([A-Za-z_-]+)\s+([^>]+?)\s*>\s*([^}]*)\}\}`)
var inlineRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:\s*([^}]*)\}\}`)

// Page is one rendered page of the documentation, for the writers: the
// Markdown files (Generate) or a static site (dochtml).
type Page struct {
	File       string   // file name, .md
	Title      string   // the identity, or the index title
	Kind       string   // topic kind, "group", or "index" for the composed index pages
	Books      []string // books the page belongs to (topics and groups)
	Categories []string
	Short      string   // one-line description
	Keywords   []string // the kw field
	Source     string   // "path:line" of the first segment (topics and groups)
	Markdown   string   // the page body, LF line ends
	Body       []Entry  // the page body as a tree, for the renderers
	Trail      []Crumb  // the logical location: general index, book index, section, category
	Parent     string   // the file of the last crumb ("" for the general index)
	Siblings   []Crumb  // the other pages under the same parent, by title
}

// A Crumb is one step of a page's logical location: a page and its title.
// File is the .md name; every renderer swaps the extension for its own.
type Crumb struct {
	Title string
	File  string
}

// Build renders every page of the database at dbPath with the configuration
// conf (nil: the built-in one): the topic and group pages first, then the
// index pages (general index, book indexes, sections of their own, category
// pages). Nothing is written.
func Build(dbPath string, conf *doctool.Config, log func(string)) ([]Page, error) {
	if conf == nil {
		conf = doctool.Default()
	}
	db, err := docdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	m, err := load(db)
	if err != nil {
		return nil, err
	}
	m.log = log
	var out []Page
	for _, p := range m.pages {
		pg := Page{File: p.file, Title: p.title, Kind: p.kind, Books: p.books(conf), Categories: p.cats(),
			Short: p.short(), Markdown: m.renderPage(p), Body: m.buildPage(p)}
		// the keywords: the name of every topic of the page (an overload's
		// name without its parameter list), then the kw field, no repeats
		seen := map[string]bool{}
		addKw := func(k string) {
			k = strings.TrimSpace(k)
			if key := strings.ToLower(k); k != "" && !seen[key] {
				seen[key] = true
				pg.Keywords = append(pg.Keywords, k)
			}
		}
		for _, t := range p.topics {
			name, _, _ := strings.Cut(t.ident, "(")
			addKw(name)
		}
		for _, k := range strings.Split(p.keywords(), ",") {
			addKw(k)
		}
		if len(p.topics) > 0 && len(p.topics[0].segments) > 0 {
			s := p.topics[0].segments[0]
			pg.Source = fmt.Sprintf("%s:%d", s.src, s.line)
		}
		out = append(out, pg)
	}
	indexes := m.renderIndexes(conf)
	var names []string
	for name := range indexes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := indexes[name]
		title := strings.TrimSuffix(name, ".md")
		if first, _, ok := strings.Cut(body, "\n"); ok && strings.HasPrefix(first, "# ") {
			title = strings.TrimSpace(first[2:])
		}
		out = append(out, Page{File: name, Title: title, Kind: "index", Markdown: body, Body: m.buildIndex(name, body)})
	}
	if err := attachTrails(db, out); err != nil {
		return nil, err
	}
	if len(m.issues) > 0 {
		// every issue is told, then the build stops: nothing is guessed
		for _, is := range m.issues {
			if log != nil {
				log("error: " + is.Error())
			}
		}
		msg := fmt.Sprintf("%d place(s) outside the documentation subset", len(m.issues))
		for i, is := range m.issues {
			if i == 5 {
				msg += "; ..."
				break
			}
			msg += "; " + is.Error()
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return out, nil
}

// Generate renders the database at dbPath into outDir (created if needed)
// with the documentation configuration conf (nil: the built-in one), one
// Markdown file per page (CRLF). It returns the number of pages written, the
// indexes not counted.
func Generate(dbPath, outDir string, conf *doctool.Config, log func(string)) (int, error) {
	pages, err := Build(dbPath, conf, log)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	n := 0
	for _, p := range pages {
		if err := os.WriteFile(filepath.Join(outDir, p.File), []byte(crlf(p.Markdown)), 0o644); err != nil {
			return 0, err
		}
		if p.Kind != "index" {
			n++
		}
	}
	return n, nil
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
	rows, err = db.QueryAll(`SELECT idtopic, category FROM topic_category ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		t := m.topics[r[0].(int64)]
		if t == nil {
			continue
		}
		c := strings.TrimSpace(r[1].(string))
		dup := false
		for _, have := range t.cats {
			if strings.EqualFold(have, c) {
				dup = true
			}
		}
		if c != "" && !dup {
			t.cats = append(t.cats, c)
		}
	}
	rows, err = db.QueryAll(`SELECT idtopic, book FROM topic_book ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if t := m.topics[r[0].(int64)]; t != nil {
			t.books = append(t.books, strings.ToLower(strings.TrimSpace(r[1].(string))))
		}
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
		if r[1].(int64) == 0 && scan.HeaderKind(f.label) != "" {
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
	n := m.byKey[scan.KindNote+"\x00"+scan.Key(scan.KindNote, noteID)]
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
		v = scan.Dedent(v)
		v = strings.TrimLeft(v, "\n")
	} else {
		v = scan.Dedent(v)
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
	k := scan.HeaderKind(kind)
	if k == "" {
		return nil
	}
	if t := m.byKey[k+"\x00"+scan.Key(k, ident)]; t != nil {
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

// ---------------------------------------------------------------- indexes

// catSlug turns a category name into a file name part: lower case, every
// run of characters outside a-z 0-9 replaced by one '-'.
func catSlug(c string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(c) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// indexKind is the kind a page is filed under: its own, or for a group (an
// overload family, a _tg_ set) the kind of its first topic.
func (p *page) indexKind() string {
	if p.kind == "group" && len(p.topics) > 0 {
		return p.topics[0].kind
	}
	return p.kind
}

// cats returns the categories of a page: the union of its topics', in order.
func (p *page) cats() []string {
	var out []string
	for _, t := range p.topics {
		for _, c := range t.cats {
			dup := false
			for _, have := range out {
				if strings.EqualFold(have, c) {
					dup = true
				}
			}
			if !dup {
				out = append(out, c)
			}
		}
	}
	return out
}

// books returns the books a page belongs to: the union of its topics'
// (each one's fixed, named or default books, per the configuration).
func (p *page) books(conf *doctool.Config) []string {
	var out []string
	for _, t := range p.topics {
		for _, b := range conf.BooksOf(t.kind, t.books) {
			dup := false
			for _, have := range out {
				if have == b {
					dup = true
				}
			}
			if !dup {
				out = append(out, b)
			}
		}
	}
	return out
}

// inBook reports whether the page is listed by book b: it belongs to b or
// to a book b includes.
func (p *page) inBook(conf *doctool.Config, b string) bool {
	for _, have := range p.books(conf) {
		if conf.Contains(b, have) {
			return true
		}
	}
	return false
}

// indexed reports whether the page's kind is listed in indexes at all.
func (p *page) indexed(conf *doctool.Config) bool {
	k := conf.Kind(p.indexKind())
	return k == nil || k.IsIndexed()
}

// matches reports whether the page passes one filter: its kind is the
// filter's (or "*") and one of its categories matches the masks.
func (p *page) matches(f doctool.Filter) bool {
	if f.Kind != "" && f.Kind != "*" && p.indexKind() != f.Kind {
		return false
	}
	return doctool.MatchCategory(f.Category, p.cats())
}

// selectPages returns the pages of the book (the section's own when it
// names one) that pass the section's include and exclude filters, sorted
// by title.
func (m *model) selectPages(conf *doctool.Config, book string, sec *doctool.Section) []*page {
	if sec.Book != "" {
		book = sec.Book
	}
	var out []*page
	for _, p := range m.pages {
		if !p.indexed(conf) || !p.inBook(conf, book) {
			continue
		}
		in := false
		for _, f := range sec.Include {
			if p.matches(f) {
				in = true
				break
			}
		}
		if !in {
			continue
		}
		for _, f := range sec.Exclude {
			if p.matches(f) {
				in = false
				break
			}
		}
		if in {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].title) < strings.ToLower(out[j].title) })
	return out
}

// dominant returns the kind with most pages in the list ("" when the list
// holds one kind only): in a mixed list, every other kind gets its tag.
func dominant(conf *doctool.Config, pages []*page) string {
	count := map[string]int{}
	for _, p := range pages {
		count[p.indexKind()]++
	}
	if len(count) < 2 {
		return ""
	}
	best, n := "", 0
	for _, k := range conf.Kinds { // ties: the first declared kind
		if count[k.Name] > n {
			best, n = k.Name, count[k.Name]
		}
	}
	return best
}

// entry renders one index line, with the kind tag when the kind is not the
// dominant one of a mixed list (dom "": no tags).
func (m *model) entry(conf *doctool.Config, p *page, dom string) string {
	s := fmt.Sprintf("- [%s](%s)", mdIdent(p.title), p.file)
	if dom != "" && p.indexKind() != dom {
		if k := conf.Kind(p.indexKind()); k != nil {
			s += " (" + k.Tag + ")"
		}
	}
	if short := p.short(); short != "" {
		s += " - " + short
	}
	if kw := p.keywords(); kw != "" {
		s += " (kw: " + kw + ")"
	}
	return s
}

// headerField returns the value of the first field with that label among
// the page's topics, header segments first ("" when none): a composed topic
// often writes its desc in the fragment right after the header.
func (p *page) headerField(label string) string {
	for _, headerOnly := range []bool{true, false} {
		for _, t := range p.topics {
			for _, s := range t.segments {
				if s.header != headerOnly {
					continue
				}
				for _, f := range s.fields {
					if f.label == label && strings.TrimSpace(f.value) != "" {
						return strings.TrimSpace(f.value)
					}
				}
			}
		}
	}
	return ""
}

// short is the one-line description of a page for the indexes and the meta
// description: its "short" field, or the first sentence of its "desc" when
// there is none - as plain text: the inline markers reduced to their text
// (an ilink to its label, {{label: v}} to v) so a cut never opens one, and
// code spans and bold to their content.
func (p *page) short() string {
	if s := p.headerField("short"); s != "" {
		return oneLine(plainText(s))
	}
	return firstSentence(plainText(p.headerField("desc")))
}

// plainText reduces the inline constructs of a value to their text.
func plainText(v string) string {
	v = ilinkRe.ReplaceAllStringFunc(v, func(s string) string {
		g := ilinkRe.FindStringSubmatch(s)
		if text := strings.TrimSpace(g[3]); text != "" {
			return text
		}
		return strings.TrimSpace(g[2])
	})
	v = inlineRe.ReplaceAllStringFunc(v, func(s string) string {
		return strings.TrimSpace(inlineRe.FindStringSubmatch(s)[2])
	})
	v = strings.ReplaceAll(v, "**", "")
	return strings.ReplaceAll(v, "`", "")
}

// keywords is the "kw" field of a page, one line, as plain text.
func (p *page) keywords() string { return oneLine(plainText(p.headerField("kw"))) }

// oneLine collapses the white space of a value into single spaces.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// firstSentence returns the first sentence of a text, one line, cut at 160
// characters on a word boundary with "..." when longer.
func firstSentence(s string) string {
	s = oneLine(s)
	if s == "" {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '.' && (i+1 == len(s) || s[i+1] == ' ') && i > 0 && s[i-1] != '.' {
			// not an abbreviation like "e.g." or a name like "x.y": a dot before a space, after a word of 2+ letters
			if i >= 2 && s[i-2] != '.' && s[i-2] != ' ' {
				s = s[:i+1]
				break
			}
		}
	}
	if len(s) > 160 {
		cut := strings.LastIndex(s[:160], " ")
		if cut < 80 {
			cut = 160
		}
		s = strings.TrimRight(s[:cut], " ,;:") + "..."
	}
	return s
}

// renderIndexes builds every index page the configuration asks for: for
// each book, its index pages (each one its sections: a list by name, or a
// list of categories linking to one category page per book and category),
// the sections that are pages of their own, the category pages, and the
// general index. A category page file is the book prefix plus the category
// slug; when a page already owns that name, "-category" is appended and the
// log told. The result maps file names to contents.
func (m *model) renderIndexes(conf *doctool.Config) map[string]string {
	out := map[string]string{}
	taken := map[string]bool{}
	for _, p := range m.pages {
		taken[strings.ToLower(p.file)] = true
	}
	reserve := func(file string) string {
		if taken[file] {
			alt := strings.TrimSuffix(file, ".md") + "-category.md"
			if m.log != nil {
				m.log(fmt.Sprintf("warning: %s: file name taken by a page, written as %s", file, alt))
			}
			file = alt
		}
		taken[file] = true
		return file
	}
	general := conf.Index.Slug + ".md"
	taken[general] = true
	for _, b := range conf.Books {
		for _, ix := range b.Indexes {
			taken[ix.Slug+".md"] = true
			for _, sec := range ix.Sections {
				if sec.Slug != "" {
					taken[sec.Slug+".md"] = true
				}
			}
		}
	}
	type catPage struct {
		name  string
		pages map[*page]bool
	}
	for _, b := range conf.Books {
		catPages := map[string]*catPage{} // lower category -> page
		var catFiles []string
		catFile := func(c string) string {
			key := strings.ToLower(c)
			if cp := catPages[key]; cp != nil {
				return cp.name
			}
			cp := &catPage{name: reserve(b.Prefix + catSlug(c) + ".md"), pages: map[*page]bool{}}
			catPages[key] = cp
			catFiles = append(catFiles, key)
			return cp.name
		}
		for _, ix := range b.Indexes {
			var body strings.Builder
			fmt.Fprintf(&body, "# %s\n\n[%s](%s)\n", ix.Title, conf.Index.Title, general)
			m.place(ix.Slug+".md", []Crumb{{conf.Index.Title, general}})
			for si := range ix.Sections {
				sec := &ix.Sections[si]
				pages := m.selectPages(conf, b.Name, sec)
				// the logical location of what this section lists: general
				// index, book index, the section (its own page, or the index)
				// a section without a page of its own is a heading of the
				// index page: its crumb points at that heading's anchor, and
				// the anchor keeps its pages apart from the other sections'
				secCrumbs := []Crumb{{conf.Index.Title, general}, {ix.Title, ix.Slug + ".md"}}
				secCrumb := Crumb{sec.Title, ix.Slug + ".md#" + catSlug(sec.Title)}
				if sec.Slug != "" {
					secCrumb.File = sec.Slug + ".md"
				}
				inSection := append(secCrumbs, secCrumb)
				dom := dominant(conf, pages)
				var list strings.Builder
				switch sec.By {
				case "category":
					byCat := map[string][]*page{}
					var none []*page
					for _, p := range pages {
						cs := p.cats()
						if len(cs) == 0 {
							none = append(none, p)
						}
						for _, c := range cs {
							byCat[strings.ToLower(c)] = append(byCat[strings.ToLower(c)], p)
						}
					}
					var cats []string
					for c := range byCat {
						cats = append(cats, c)
					}
					sort.Strings(cats)
					for _, c := range cats {
						file := catFile(c)
						cp := catPages[c]
						m.place(file, inSection)
						for _, p := range byCat[c] {
							cp.pages[p] = true
							m.place(p.file, append(inSection, Crumb{c, file}))
						}
						fmt.Fprintf(&list, "- [%s](%s) (%d)\n", c, file, len(byCat[c]))
					}
					if len(none) > 0 {
						fmt.Fprintf(&list, "\n(no category)\n\n")
						for _, p := range none {
							list.WriteString(m.entry(conf, p, dom) + "\n")
							m.place(p.file, inSection)
						}
					}
				default:
					for _, p := range pages {
						list.WriteString(m.entry(conf, p, dom) + "\n")
						m.place(p.file, inSection)
					}
				}
				if sec.Slug != "" {
					var sb strings.Builder
					fmt.Fprintf(&sb, "# %s\n\n[%s](%s) - [%s](%s)\n\n", sec.Title, ix.Title, ix.Slug+".md", conf.Index.Title, general)
					sb.WriteString(list.String())
					out[sec.Slug+".md"] = sb.String()
					m.place(sec.Slug+".md", secCrumbs)
					fmt.Fprintf(&body, "\n## [%s](%s)\n", sec.Title, sec.Slug+".md")
				} else {
					fmt.Fprintf(&body, "\n## %s\n\n", sec.Title)
					body.WriteString(list.String())
				}
			}
			out[ix.Slug+".md"] = body.String()
		}
		back := ""
		if len(b.Indexes) > 0 {
			back = fmt.Sprintf("[%s](%s) - ", b.Indexes[0].Title, b.Indexes[0].Slug+".md")
		}
		for _, key := range catFiles {
			cp := catPages[key]
			var ps []*page
			for p := range cp.pages {
				ps = append(ps, p)
			}
			sort.SliceStable(ps, func(i, j int) bool { return strings.ToLower(ps[i].title) < strings.ToLower(ps[j].title) })
			var cb strings.Builder
			fmt.Fprintf(&cb, "# %s\n\n%s[%s](%s)\n\n", key, back, conf.Index.Title, general)
			dom := dominant(conf, ps)
			for _, p := range ps {
				cb.WriteString(m.entry(conf, p, dom) + "\n")
			}
			out[cp.name] = cb.String()
		}
	}
	// the general index: its sections list books, each with its indexes and
	// the sections that are pages of their own
	var g strings.Builder
	fmt.Fprintf(&g, "# %s\n", conf.Index.Title)
	for _, gs := range conf.Index.Sections {
		if gs.Title != "" {
			fmt.Fprintf(&g, "\n## %s\n", gs.Title)
		}
		g.WriteString("\n")
		for _, name := range gs.Books {
			b := conf.Book(name)
			if b == nil {
				continue
			}
			if len(b.Indexes) == 0 {
				fmt.Fprintf(&g, "- %s\n", b.Title)
				continue
			}
			fmt.Fprintf(&g, "- [%s](%s)\n", b.Title, b.Indexes[0].Slug+".md")
			for i, ix := range b.Indexes {
				if i > 0 {
					fmt.Fprintf(&g, "  - [%s](%s)\n", ix.Title, ix.Slug+".md")
				}
				for _, sec := range ix.Sections {
					if sec.Slug != "" {
						fmt.Fprintf(&g, "  - [%s](%s)\n", sec.Title, sec.Slug+".md")
					}
				}
			}
		}
	}
	out[general] = g.String()
	m.place(general, nil)
	return out
}
