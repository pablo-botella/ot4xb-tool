// The page tree: a page as the writers consume it, built from the fields
// through the zone-1 parser. Content travels raw from the database to the
// renderer - identifiers, placeholders and paths as written - and every
// renderer escapes for its own output. Nothing is escaped here.
package gen

import (
	"fmt"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/doctool/scan"
)

// An Entry is one field of a page as rendered: its label (visible ones
// only, "" for a hidden label, a "|:" text, a heading or an index) and the
// blocks of its value. A renderer shows the label in front of the value's
// first paragraph, or on a line of its own when the value starts with a
// list or a code block.
type Entry struct {
	Label  string
	Blocks []Block
}

// Issue is one thing the sources say outside the subset, with its place.
type Issue struct {
	Src  string // source file, as the database names it
	Line int    // line of the marker's segment
	At   int    // line within the field value, 1-based
	Msg  string
}

func (i Issue) Error() string { return fmt.Sprintf("%s:%d (+%d): %s", i.Src, i.Line, i.At, i.Msg) }

// buildPage builds the tree of a topic or group page.
func (m *model) buildPage(p *page) []Entry {
	if p.kind == "group" {
		out := []Entry{heading(1, p.title)}
		for _, t := range p.topics {
			out = append(out, m.buildTopic(t, 2, nil)...)
		}
		return out
	}
	return m.buildTopic(p.topics[0], 1, nil)
}

func heading(level int, text string) Entry {
	return Entry{Blocks: []Block{&Heading{Level: level, Inlines: []Inline{&Text{Text: text}}}}}
}

// buildTopic builds a topic with its title at the given heading level; with
// level 0 (transclusion) the title is omitted. stack holds the topics being
// transcluded, to cut cycles. The rules are those of renderTopic: fields in
// written order, the identity is the title, hidden entries vanish, an
// include-note-id transcludes the note's entries in place, and within one
// marker the entries after a list item continue that item.
func (m *model) buildTopic(t *topic, level int, stack []int64) []Entry {
	var out []Entry
	if level > 0 {
		out = append(out, heading(level, t.ident))
	}
	for _, s := range t.segments {
		var seg []Entry
		var hidden []bool
		for i, f := range s.fields {
			if s.header && i == 0 {
				continue // the identity is the title
			}
			if f.hideEntry {
				continue
			}
			if f.label == "include-note-id" {
				seg = append(seg, m.transcludeEntries(strings.TrimSpace(f.value), append(stack, t.id))...)
				for range seg[len(hidden):] {
					hidden = append(hidden, false)
				}
				continue
			}
			seg = append(seg, m.buildField(f, s))
			hidden = append(hidden, f.label == "" || f.hideLabel)
		}
		out = append(out, joinMarkerEntries(seg, hidden)...)
	}
	return out
}

func (m *model) transcludeEntries(noteID string, stack []int64) []Entry {
	n := m.byKey[scan.KindNote+"\x00"+scan.Key(scan.KindNote, noteID)]
	if n == nil {
		return []Entry{{Blocks: []Block{&Paragraph{Inlines: []Inline{&Text{Text: "(missing note " + noteID + ")"}}}}}}
	}
	for _, id := range stack {
		if id == n.id {
			return []Entry{{Blocks: []Block{&Paragraph{Inlines: []Inline{&Text{Text: "(include cycle cut: " + noteID + ")"}}}}}}
		}
	}
	return m.buildTopic(n, 0, stack)
}

// buildField parses one field's value. The value is dedented as written
// (the parser trims its lines anyway); the label stays raw.
func (m *model) buildField(f field, s segment) Entry {
	v := strings.TrimLeft(scan.Dedent(f.value), "\n")
	blocks, issues := ParseText(v)
	for _, is := range issues {
		m.issues = append(m.issues, Issue{Src: s.src, Line: int(s.line), At: is.Line, Msg: is.Msg})
	}
	m.resolveBlocks(blocks)
	e := Entry{Blocks: blocks}
	if f.label != "" && !f.hideLabel {
		e.Label = f.label
	}
	return e
}

// resolveBlocks replaces the calls of the marker grammar inside the blocks:
// an ilink becomes a Link to its page (its text alone when the target is
// missing), any other {{label: value}} becomes its bold label and value
// (the value alone when the label is hidden).
func (m *model) resolveBlocks(blocks []Block) {
	for _, b := range blocks {
		switch v := b.(type) {
		case *Heading:
			v.Inlines = m.resolveInlines(v.Inlines)
		case *Paragraph:
			v.Inlines = m.resolveInlines(v.Inlines)
		case *List:
			for i := range v.Items {
				v.Items[i] = m.resolveInlines(v.Items[i])
			}
		}
	}
}

func (m *model) resolveInlines(in []Inline) []Inline {
	var out []Inline
	for _, i := range in {
		c, ok := i.(*Call)
		if !ok {
			out = append(out, i)
			continue
		}
		if c.Name == "ilink" {
			// "<kind ident> text"
			arg := strings.TrimSpace(c.Arg)
			if strings.HasPrefix(arg, "<") {
				if end := strings.IndexByte(arg, '>'); end > 0 {
					ref := strings.Fields(arg[1:end])
					text := strings.TrimSpace(arg[end+1:])
					if len(ref) >= 2 {
						kind, ident := ref[0], strings.Join(ref[1:], " ")
						if text == "" {
							text = ident
						}
						if p := m.target(kind, ident); p != nil {
							out = append(out, &Link{Text: text, Target: p.file})
						} else {
							out = append(out, &Text{Text: text})
						}
						continue
					}
				}
			}
			out = append(out, &Text{Text: arg})
			continue
		}
		label, _, hideLabel := canon(c.Name)
		if !hideLabel {
			out = append(out, &Strong{Text: label + ":"}, &Text{Text: " "})
		}
		out = append(out, &Text{Text: c.Arg})
	}
	return out
}

// joinMarkerEntries is joinMarker over entries: after an entry that is a
// list, the entries of the same marker that follow and are paragraphs only
// continue its last item on the same line - a hidden-label one after " - ",
// a labelled one after a blank with its bold label. Anything else stays a
// separate entry.
func joinMarkerEntries(entries []Entry, hidden []bool) []Entry {
	var out []Entry
	for i, e := range entries {
		if i > 0 && len(out) > 0 {
			if last := lastList(out[len(out)-1]); last != nil && !hasList(e) && allParagraphs(e.Blocks) && len(e.Blocks) > 0 {
				item := &last.Items[len(last.Items)-1]
				sep := " "
				if hidden[i] {
					sep = " - "
				}
				*item = append(*item, &Text{Text: sep})
				if e.Label != "" {
					*item = append(*item, &Strong{Text: e.Label + ":"}, &Text{Text: " "})
				}
				for j, b := range e.Blocks {
					if j > 0 {
						*item = append(*item, &Text{Text: " "})
					}
					*item = append(*item, b.(*Paragraph).Inlines...)
				}
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// lastList returns the list an entry ends with, nil when it ends otherwise.
func lastList(e Entry) *List {
	if len(e.Blocks) == 0 {
		return nil
	}
	l, _ := e.Blocks[len(e.Blocks)-1].(*List)
	return l
}

func hasList(e Entry) bool {
	for _, b := range e.Blocks {
		if _, ok := b.(*List); ok {
			return true
		}
	}
	return false
}

func allParagraphs(blocks []Block) bool {
	for _, b := range blocks {
		if _, ok := b.(*Paragraph); !ok {
			return false
		}
	}
	return true
}

// buildIndex builds the tree of an index page from its Markdown: the
// generator writes the indexes in the subset (headings, paragraphs of links,
// lists of links, one level of nesting in the general index), so the parser
// reads them back.
func (m *model) buildIndex(name, body string) []Entry {
	blocks, issues := parseText(body, true)
	for _, is := range issues {
		m.issues = append(m.issues, Issue{Src: name, Line: 0, At: is.Line, Msg: is.Msg})
	}
	indexLinks(blocks)
	return []Entry{{Blocks: blocks}}
}

// indexLinks fixes the links of an index body, which the generator wrote
// itself as Markdown: every bracket there is a page reference, so it becomes
// a Link (the parser makes a URL of a bracket, as it must for an author's);
// and the code span mdIdent puts around a link text for the Markdown output
// (`_name_`) goes, because in the tree the name is plain text.
func indexLinks(blocks []Block) {
	var fix func(in []Inline)
	fix = func(in []Inline) {
		for k, i := range in {
			if u, ok := i.(*URL); ok {
				i = &Link{Text: u.Text, Target: u.URL}
				in[k] = i
			}
			if l, ok := i.(*Link); ok && len(l.Text) > 2 && l.Text[0] == '`' && l.Text[len(l.Text)-1] == '`' {
				l.Text = l.Text[1 : len(l.Text)-1]
			}
		}
	}
	var lists func(l *List)
	lists = func(l *List) {
		for i, it := range l.Items {
			fix(it)
			if i < len(l.Subs) && l.Subs[i] != nil {
				lists(l.Subs[i])
			}
		}
	}
	for _, b := range blocks {
		switch v := b.(type) {
		case *Heading:
			fix(v.Inlines)
		case *Paragraph:
			fix(v.Inlines)
		case *List:
			lists(v)
		}
	}
}
