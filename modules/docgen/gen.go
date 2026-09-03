package docgen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/doccompile"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/scandoc"
)

// Topic is a documented thing as stored in the database.
type Topic struct {
	ID               int64
	Kind, Key, Ident string
}

// segment is one contribution of a source to a topic, with its provenance.
type segment struct {
	id, topic int64
	src       string
	line, pos int64
	raw       []byte
}

// ref is a stored reference, with its target resolved when it exists.
type ref struct {
	from            int64
	toKind, toIdent string
	refType         string
	to              int64 // 0 when unresolved
}

// itemKinds are the class/structure auxiliaries: a segment holding one alone
// re-parses only inside a class scope, so its raw text is wrapped in one.
var itemKinds = map[string]bool{
	"method": true, "ivar": true, "property": true,
	"class-method": true, "class-var": true, "class-property": true,
	"gwst-member": true,
}

// reopenable / class family, as in resolve.
func family(kind string) []string {
	if kind == "class" || kind == "structure" {
		return []string{"class", "structure"}
	}
	return []string{kind}
}

type model struct {
	topics   []Topic
	byID     map[int64]Topic
	byKey    map[string]int64 // kind|key
	byName   map[string][]int64
	segs     map[int64][]segment
	refs     map[int64][]ref
	cats     map[int64][]string
	members  map[int64][]int64 // owner topic -> component topics
	inTopic  map[int64][]int64 // topic -> commands living in it
	slugs    map[int64]string
	segCount int
	parsed   map[int64]*scandoc.File // segment id -> its re-parsed raw text
	warnings []string
}

// explicitSlug returns the `| slug:` field of a re-parsed segment, if any:
// on the entity, on its (single) component, or on the note.
func explicitSlug(f *scandoc.File) string {
	for _, e := range f.Entities {
		if v, ok := e.Field("slug"); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		for _, c := range e.Components {
			if v, ok := c.Field("slug"); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	for _, n := range f.Notes {
		if n.Slug != "" {
			return n.Slug
		}
	}
	return ""
}

func load(db *docdb.DB) (*model, error) {
	m := &model{byID: map[int64]Topic{}, byKey: map[string]int64{}, byName: map[string][]int64{},
		segs: map[int64][]segment{}, refs: map[int64][]ref{}, cats: map[int64][]string{},
		members: map[int64][]int64{}, inTopic: map[int64][]int64{}}
	rows, err := db.QueryAll(`SELECT idtopic, kind, key, ident FROM topics ORDER BY kind, key`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		t := Topic{r[0].(int64), r[1].(string), r[2].(string), r[3].(string)}
		m.topics = append(m.topics, t)
		m.byID[t.ID] = t
		m.byKey[t.Kind+"|"+t.Key] = t.ID
		m.byName[strings.ToUpper(t.Key)] = append(m.byName[strings.ToUpper(t.Key)], t.ID)
	}
	rows, err = db.QueryAll(`SELECT s.idseg, s.idtopic, o.src, s.line, s.pos, s.raw FROM segments s JOIN sources o USING(idsrc) ORDER BY s.idtopic, s.pos`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		s := segment{r[0].(int64), r[1].(int64), r[2].(string), r[3].(int64), r[4].(int64), r[5].([]byte)}
		m.segs[s.topic] = append(m.segs[s.topic], s)
		m.segCount++
	}
	rows, err = db.QueryAll(`SELECT idtopic_in, reftokind, reftoident, reftype FROM refs ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		x := ref{r[0].(int64), r[1].(string), r[2].(string), r[3].(string), 0}
		x.to = m.resolve(x.toKind, x.toIdent)
		m.refs[x.from] = append(m.refs[x.from], x)
		if x.refType == "in-topic" && x.to != 0 {
			m.inTopic[x.to] = append(m.inTopic[x.to], x.from)
		}
	}
	rows, err = db.QueryAll(`SELECT idtopic, category FROM topic_category ORDER BY category`)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		m.cats[r[0].(int64)] = append(m.cats[r[0].(int64)], r[1].(string))
	}
	// components belong to the topic whose key is the part before ':'
	for _, t := range m.topics {
		if !itemKinds[t.Kind] {
			continue
		}
		owner, _, ok := strings.Cut(t.Key, ":")
		if !ok {
			continue
		}
		for _, k := range []string{"class", "structure", "cpp-class"} {
			if id, ok := m.byKey[k+"|"+owner]; ok {
				m.members[id] = append(m.members[id], t.ID)
				break
			}
		}
	}
	// re-parse every segment once (cached for rendering) and collect the
	// explicit slugs: the first `| slug:` found among a topic's segments wins
	m.parsed = map[int64]*scandoc.File{}
	explicit := map[int64]string{}
	for _, t := range m.topics {
		for _, s := range m.segs[t.ID] {
			f := reparse(s.raw)
			m.parsed[s.id] = f
			if _, done := explicit[t.ID]; !done {
				if e := explicitSlug(f); e != "" {
					explicit[t.ID] = e
				}
			}
		}
	}
	var sw []SlugWarning
	m.slugs, sw = Slugs(m.topics, explicit)
	for _, w := range sw {
		where := ""
		if segs := m.segs[w.ID]; len(segs) > 0 {
			where = fmt.Sprintf(" (%s:%d)", segs[0].src, segs[0].line)
		}
		m.warnings = append(m.warnings, w.Msg+where)
	}
	return m, nil
}

// resolve finds a reference target: qualified targets by their family and
// key rule; unqualified names (see-also, calls) by name among the Xbase++
// callables and classes, first match wins.
func (m *model) resolve(kind, ident string) int64 {
	if kind != "" {
		for _, k := range family(kind) {
			if id, ok := m.byKey[k+"|"+doccompile.Key(k, ident)]; ok {
				return id
			}
		}
		return 0
	}
	name := strings.ToUpper(strings.TrimSpace(ident))
	for _, k := range []string{"function", "internal-function", "class", "structure", "command"} {
		if id, ok := m.byKey[k+"|"+name]; ok {
			return id
		}
	}
	return 0
}

// link renders a Markdown link to a topic, or plain text when it has no file.
func (m *model) link(id int64, text string) string {
	if id == 0 {
		return text
	}
	if text == "" {
		text = m.byID[id].Ident
	}
	return fmt.Sprintf("[%s](%s.md)", text, m.slugs[id])
}

// reparse re-scans a segment's raw marker text with the one scanner. A lone
// class auxiliary is wrapped in a synthetic class scope so it attaches as a
// component; the wrapper never reaches the output.
func reparse(raw []byte) *scandoc.File {
	text := strings.TrimSpace(string(raw))
	first := text
	if i := strings.Index(first, "/*{{"); i >= 0 {
		first = first[i+4:]
	}
	key := first
	for i := 0; i < len(key); i++ {
		if key[i] == ':' || key[i] == ' ' || key[i] == '\t' || key[i] == '}' {
			key = key[:i]
			break
		}
	}
	if itemKinds[strings.ToLower(strings.TrimSpace(key))] {
		text = "/*{{begin-class}}*/\r\n/*{{class-name: _}}*/\r\n" + text + "\r\n/*{{end-class}}*/"
	}
	return scandoc.ScanText("segment", []byte(text))
}

// firstPos is the document position of a topic's first segment (members are
// listed in the order they were declared).
func (m *model) firstPos(id int64) int64 {
	if segs := m.segs[id]; len(segs) > 0 {
		return segs[0].pos
	}
	return 0
}

// memberType extracts the declared type of a gwst-member from its authored
// text: a `type:` field when the marker carries one, else the token after
// "type:" in the identity line ("u type: _LARGE_INTEGER_ pos: 0 size: 8").
func memberType(c scandoc.Component) string {
	if v, ok := c.Field("type"); ok && strings.TrimSpace(v) != "" {
		return strings.Trim(strings.Fields(v)[0], "`")
	}
	if i := strings.Index(c.Ident, "type:"); i >= 0 {
		if f := strings.Fields(c.Ident[i+len("type:"):]); len(f) > 0 {
			return strings.Trim(f[0], "`")
		}
	}
	return ""
}

// childOf finds the documented class/structure a member type names: the
// type itself, or its WAPIST_ form (the wapist structures carry that prefix
// as their real name). 0 when the type is a plain scalar.
func (m *model) childOf(typ string) int64 {
	if typ == "" {
		return 0
	}
	for _, name := range []string{typ, "WAPIST_" + typ} {
		if id := m.resolve("class", name); id != 0 {
			return id
		}
	}
	return 0
}

// memberLine renders one line of a Structure Definition: MEMBER TYPE name for
// a scalar, MEMBER @ [CLASS](page) name for an embedded structure.
func (m *model) memberLine(id int64) string {
	t := m.byID[id]
	name := t.Ident
	if _, after, ok := strings.Cut(t.Ident, ":"); ok {
		name = after
	}
	typ := ""
	if segs := m.segs[id]; len(segs) > 0 {
		if f := m.parsed[segs[0].id]; f != nil {
			for _, e := range f.Entities {
				for _, c := range e.Components {
					typ = memberType(c)
				}
			}
		}
	}
	if child := m.childOf(typ); child != 0 {
		return fmt.Sprintf("MEMBER @ %s %s", m.link(child, m.byID[child].Ident), name)
	}
	if typ == "" {
		return "MEMBER " + name
	}
	return fmt.Sprintf("MEMBER %s %s", typ, name)
}

// renderNoteBody transcludes a shared note: its body, then the notes it
// includes in turn. path guards against cycles; repetitions are allowed.
func (m *model) renderNoteBody(w *md, id int64, path map[int64]bool) {
	if id == 0 || path[id] {
		return
	}
	path[id] = true
	defer delete(path, id)
	for _, s := range m.segs[id] {
		f := m.parsed[s.id]
		if f == nil {
			continue
		}
		for _, n := range f.Notes {
			if n.Body != "" {
				w.line("%s", strings.ReplaceAll(n.Body, "\n", "\r\n"))
				w.blank()
			}
			for _, dep := range n.Includes {
				m.renderNoteBody(w, m.resolve("note", dep), path)
			}
		}
	}
}

// Generate writes one Markdown file per topic plus index.md into outDir.
// Files whose bytes are already right are not rewritten. Returns the number
// of topic files.
func Generate(dbPath, outDir string, log func(string)) (int, error) {
	db, err := docdb.Open(dbPath)
	if err != nil {
		return 0, err
	}
	m, err := load(db)
	db.Close()
	if err != nil {
		return 0, err
	}
	if log != nil {
		for _, w := range m.warnings {
			log("gendoc: warning: " + w)
		}
	}
	if err := os.MkdirAll(outDir, 0777); err != nil {
		return 0, err
	}
	written := 0
	for _, t := range m.topics {
		body := m.renderTopic(t)
		w, err := writeIfChanged(filepath.Join(outDir, m.slugs[t.ID]+".md"), body)
		if err != nil {
			return written, err
		}
		if w {
			written++
		}
	}
	if _, err := writeIfChanged(filepath.Join(outDir, "index.md"), m.renderIndex()); err != nil {
		return written, err
	}
	if log != nil {
		log(fmt.Sprintf("gendoc: %d topic(s), %d segment(s) -> %s (%d file(s) written, the rest unchanged)",
			len(m.topics), m.segCount, outDir, written))
	}
	return len(m.topics), nil
}

func writeIfChanged(path string, content []byte) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, content) {
		return false, nil
	}
	return true, os.WriteFile(path, content, 0666)
}

// ---- rendering ---------------------------------------------------------

type md struct{ strings.Builder }

func (w *md) line(format string, a ...any) {
	fmt.Fprintf(&w.Builder, format, a...)
	w.WriteString("\r\n")
}

func (w *md) blank() { w.WriteString("\r\n") }

func (m *model) renderTopic(t Topic) []byte {
	var w md
	w.line("# %s", t.Ident)
	meta := "*" + t.Kind + "*"
	if cats := m.cats[t.ID]; len(cats) > 0 {
		meta += " · " + strings.Join(cats, ", ")
	}
	w.line("%s", meta)
	w.blank()
	for i, s := range m.segs[t.ID] {
		if len(m.segs[t.ID]) > 1 {
			w.line("---")
			w.line("*Segment %d of %d*", i+1, len(m.segs[t.ID]))
		}
		w.line("*Source:* `%s:%d`", s.src, s.line)
		w.blank()
		m.renderSegment(&w, t, s)
		// the notes this segment includes are transcluded right here - that is
		// what include-note-id is for (repetitions allowed, cycles cut)
		if f := m.parsed[s.id]; f != nil {
			for _, e := range f.Entities {
				for _, id := range e.NoteRefs {
					m.renderNoteBody(&w, m.resolve("note", id), map[int64]bool{})
				}
			}
		}
	}
	// members go in the order they were declared (document order)
	kids := append([]int64(nil), m.members[t.ID]...)
	sort.SliceStable(kids, func(a, b int) bool { return m.firstPos(kids[a]) < m.firstPos(kids[b]) })
	var layout, others []int64
	for _, k := range kids {
		if m.byID[k].Kind == "gwst-member" {
			layout = append(layout, k)
		} else {
			others = append(others, k)
		}
	}
	if len(layout) > 0 {
		w.line("## Structure Definition")
		w.blank()
		w.line("**BEGIN STRUCTURE**")
		for _, k := range layout {
			w.line("- %s", m.memberLine(k))
		}
		w.line("**END STRUCTURE**")
		w.blank()
	}
	if len(others) > 0 {
		w.line("## Members")
		w.blank()
		for _, k := range others {
			c := m.byID[k]
			w.line("- *%s* %s", c.Kind, m.link(k, c.Ident))
		}
		w.blank()
	}
	if cmds := m.inTopic[t.ID]; len(cmds) > 0 {
		w.line("## Commands")
		w.blank()
		for _, c := range cmds {
			w.line("- %s", m.link(c, ""))
		}
		w.blank()
	}
	if refs := m.refs[t.ID]; len(refs) > 0 {
		var lines []string
		for _, r := range refs {
			switch r.refType {
			case "include":
				// transcluded on the page, not listed
			case "parent", "gwst-parent":
				lines = append(lines, fmt.Sprintf("- %s: %s", r.refType, m.link(r.to, r.toIdent)))
			case "ilink":
				lines = append(lines, fmt.Sprintf("- link: %s", m.link(r.to, r.toIdent)))
			case "see-also", "calls":
				lines = append(lines, fmt.Sprintf("- %s: %s", r.refType, m.link(r.to, r.toIdent)))
			}
		}
		if len(lines) > 0 {
			w.line("## References")
			w.blank()
			for _, l := range lines {
				w.line("%s", l)
			}
			w.blank()
		}
	}
	return []byte(w.String())
}

func (m *model) renderSegment(w *md, t Topic, s segment) {
	f := m.parsed[s.id]
	if f == nil {
		f = reparse(s.raw)
	}
	switch {
	case t.Kind == "markdown-free":
		for _, e := range f.Entities {
			if v, ok := e.Field("body"); ok {
				w.line("%s", strings.ReplaceAll(v, "\n", "\r\n"))
			}
		}
	case t.Kind == "note":
		for _, n := range f.Notes {
			w.line("%s", strings.ReplaceAll(n.Body, "\n", "\r\n"))
		}
	case itemKinds[t.Kind]:
		for _, e := range f.Entities {
			for _, c := range e.Components {
				w.line("`%s`", c.Ident)
				w.blank()
				m.renderFields(w, c.Fields)
			}
		}
	default:
		for _, e := range f.Entities {
			m.renderFields(w, e.Fields)
		}
	}
	w.blank()
}

// renderFields lays the fields of an entity out in reading order: syntax
// first, the description, parameters, return, flags, examples, notes, then
// everything else as "name: value" - unknown fields included, nothing is
// dropped. see-also and ilink are rendered as references at the end of the
// page, category in the header.
func (m *model) renderFields(w *md, fields []scandoc.Field) {
	var params, flags, rest []scandoc.Field
	var desc, ret, syntax []string
	var examples, notes, frees []string
	for _, f := range fields {
		name := strings.ToLower(f.Name)
		first, _, _ := strings.Cut(name, " ")
		switch first {
		case "syntax", "xbase-syntax", "prototype":
			syntax = append(syntax, f.Value)
		case "desc":
			desc = append(desc, f.Value)
		case "param":
			params = append(params, f)
		case "return":
			ret = append(ret, f.Value)
		case "flag":
			flags = append(flags, f)
		case "example":
			examples = append(examples, f.Value)
		case "note":
			notes = append(notes, f.Value)
		case "markdown-free":
			frees = append(frees, f.Value) // raw Markdown embedded in the entity: verbatim, no label
		case "category", "see-also", "ilink", "calls", "parent", "gwst-parent", "topic", "body", "slug":
			// header / references / file name: handled apart, not content
		default:
			rest = append(rest, f)
		}
	}
	for _, s := range syntax {
		w.line("%s", s)
		w.blank()
	}
	for _, d := range desc {
		w.line("%s", d)
		w.blank()
	}
	if len(params) > 0 {
		w.line("**Parameters**")
		w.blank()
		for _, p := range params {
			_, pname, _ := strings.Cut(p.Name, " ")
			w.line("- `%s` — %s", strings.TrimSpace(pname), p.Value)
		}
		w.blank()
	}
	for _, r := range ret {
		w.line("**Returns** — %s", r)
		w.blank()
	}
	if len(flags) > 0 {
		w.line("**Flags**")
		w.blank()
		for _, f := range flags {
			_, fname, _ := strings.Cut(f.Name, " ")
			w.line("- `%s` — %s", strings.TrimSpace(fname), f.Value)
		}
		w.blank()
	}
	for _, e := range examples {
		w.line("**Example**")
		w.blank()
		w.line("%s", strings.ReplaceAll(e, "\n", "\r\n"))
		w.blank()
	}
	for _, n := range notes {
		w.line("> %s", n)
		w.blank()
	}
	for _, r := range frees {
		w.line("%s", strings.ReplaceAll(r, "\n", "\r\n"))
		w.blank()
	}
	for _, f := range rest {
		w.line("**%s:** %s", f.Name, f.Value)
	}
	if len(rest) > 0 {
		w.blank()
	}
}

func (m *model) renderIndex() []byte {
	var w md
	w.line("# Reference index")
	w.blank()
	w.line("%d topics. One file per topic; members and commands are listed on their owner's page.", len(m.topics))
	w.blank()
	byKind := map[string][]Topic{}
	var kinds []string
	for _, t := range m.topics {
		if _, ok := byKind[t.Kind]; !ok {
			kinds = append(kinds, t.Kind)
		}
		byKind[t.Kind] = append(byKind[t.Kind], t)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		ts := byKind[k]
		sort.Slice(ts, func(a, b int) bool { return ts[a].Key < ts[b].Key })
		w.line("## %s (%d)", k, len(ts))
		w.blank()
		for _, t := range ts {
			w.line("- %s", m.link(t.ID, t.Ident))
		}
		w.blank()
	}
	return []byte(w.String())
}
