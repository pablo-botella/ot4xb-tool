// The zone-1 parser: the strict subset the documentation text is written in.
//
// A field value is text in the subset - the only constructs are ATX headings
// of one to five '#', paragraphs, "- " list items (never nested), fenced code
// blocks (three backticks, an optional language after the opening fence),
// "**strong**", code spans (one backtick) and the
// {{name: argument}} calls of the marker grammar. {{begin-md}} ... {{end-md}}
// delimits a zone-2 block: raw Markdown kept as it is for goldmark.
//
// Anything else is plain text: '_', '*', '~', '<', '\' and '|' mean nothing here,
// so identifiers, placeholders and paths survive untouched. What cannot be
// parsed - a sixth '#', an unclosed "**" or code span, an unclosed fence or
// zone-2 block, a nested "- " - is an Issue with its line, never a guess.
package docgen

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pablo-botella/linereader"
)

// A Block is one of *Heading, *Paragraph, *List, *CodeBlock, *RawMarkdown.
type Block interface{ block() }

// Heading is "# text" to "##### text".
type Heading struct {
	Level   int
	Inlines []Inline
}

// Paragraph is a run of text lines, joined with a space.
type Paragraph struct{ Inlines []Inline }

// List is a run of "- " items; an item is its own inline sequence, its
// continuation lines joined with a space. Subs[i], when not nil, is the
// list nested under item i: hand-written text never nests (an issue), the
// generator's indexes do, one level.
type List struct {
	Items [][]Inline
	Subs  []*List
}

// CodeBlock is a fenced block; Text is verbatim, lines joined with '\n'.
type CodeBlock struct {
	Lang string
	Text string
}

// RawMarkdown is a zone-2 block: the text between {{begin-md}} and
// {{end-md}}, untouched, for goldmark.
type RawMarkdown struct{ Text string }

func (*Heading) block()     {}
func (*Paragraph) block()   {}
func (*List) block()        {}
func (*CodeBlock) block()   {}
func (*RawMarkdown) block() {}

// An Inline is one of *Text, *Strong, *Code, *Link, *Call.
type Inline interface{ inline() }

// Link is "[text](target)": the generator writes them for the indexes, the
// resolved {{ilink}} become them; Target is a page file (.md) and every
// renderer swaps the extension for its own.
type Link struct {
	Text   string
	Target string
}

// Text is plain text, every character literal.
type Text struct{ Text string }

// Strong is "**text**"; nothing nests inside.
type Strong struct{ Text string }

// Code is a code span, "`text`".
type Code struct{ Text string }

// Call is a "{{name: argument}}" of the marker grammar, left for the
// consumer to resolve ({{ilink: ...}}, {{include-note-id: ...}}, ...).
type Call struct {
	Name string
	Arg  string
}

func (*Text) inline()   {}
func (*Strong) inline() {}
func (*Code) inline()   {}
func (*Link) inline()   {}
func (*Call) inline()   {}

// A ParseIssue is one thing the text does not say in the subset. Line is
// 1-based within the parsed text; the caller maps it to its file.
type ParseIssue struct {
	Line int
	Msg  string
}

func (e ParseIssue) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// ParseText parses a field value. The blocks hold whatever was understood;
// the issues say what was not.
func ParseText(text string) ([]Block, []ParseIssue) { return parseText(text, false) }

// parseText is ParseText with, for the generator's own text, one level of
// list nesting allowed.
func parseText(text string, nest bool) ([]Block, []ParseIssue) {
	lr := linereader.NewLineReader(strings.NewReader(text), 0, 0)
	p := &textParser{itemIndent: -1, nest: nest}
	n := 0
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			p.issue(n, "read: %v", err) // cannot happen on a string
			break
		}
		n++
		p.line(n, string(raw))
	}
	p.flush()
	if p.fence != "" {
		p.issue(p.fenceLine, "code block opened here is never closed")
	}
	if p.inMD {
		p.issue(p.mdLine, "{{begin-md}} here is never closed")
	}
	return p.blocks, p.issues
}

type textParser struct {
	blocks []Block
	issues []ParseIssue

	// the open block, if any: a paragraph, a list, a fence or a zone-2 block
	para       []string     // lines of the open paragraph
	items      [][]string   // lines of every item of the open list
	subs       [][][]string // per item, the lines of its nested items (nest only)
	nest       bool         // one level of nesting allowed (the generator's text)
	inList     bool
	itemIndent int    // indentation of the items of the open list
	firstLine  int    // line the open paragraph or list starts on
	fence      string // "```" while inside a code block
	lang       string
	code       []string
	fenceLine  int
	inMD       bool
	md         []string
	mdLine     int
}

func (p *textParser) issue(line int, format string, a ...any) {
	p.issues = append(p.issues, ParseIssue{Line: line, Msg: fmt.Sprintf(format, a...)})
}

func indent(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }

func (p *textParser) line(n int, s string) {
	// the verbatim zones first: a fence, a zone-2 block
	if p.fence != "" {
		if strings.TrimSpace(s) == p.fence {
			p.blocks = append(p.blocks, &CodeBlock{Lang: p.lang, Text: strings.Join(p.code, "\n")})
			p.fence, p.lang, p.code = "", "", nil
			return
		}
		p.code = append(p.code, s)
		return
	}
	if p.inMD {
		if i := strings.Index(s, "{{end-md}}"); i >= 0 {
			if before := s[:i]; strings.TrimSpace(before) != "" {
				p.md = append(p.md, before)
			}
			p.blocks = append(p.blocks, &RawMarkdown{Text: strings.Join(p.md, "\n")})
			p.inMD, p.md = false, nil
			if after := strings.TrimSpace(s[i+len("{{end-md}}"):]); after != "" {
				p.issue(n, "text after {{end-md}} on the same line")
			}
			return
		}
		p.md = append(p.md, s)
		return
	}
	t := strings.TrimSpace(s)
	switch {
	case t == "":
		p.flush()
	case strings.HasPrefix(t, "```"):
		p.flush()
		p.fence, p.lang, p.fenceLine = t[:3], strings.TrimSpace(t[3:]), n
	case strings.HasPrefix(t, "{{begin-md"):
		p.flush()
		p.inMD, p.mdLine = true, n
		rest := strings.TrimPrefix(t, "{{begin-md")
		if rest != "}}" && !strings.HasPrefix(rest, ":") {
			p.issue(n, "malformed {{begin-md}}")
		} else if i := strings.Index(rest, "}}"); i >= 0 && strings.TrimSpace(rest[i+2:]) != "" {
			// text after the mark on the same line belongs to the block
			p.md = append(p.md, strings.TrimSpace(rest[i+2:]))
		}
	case t[0] == '#':
		p.flush()
		level := 0
		for level < len(t) && t[level] == '#' {
			level++
		}
		if level > 5 || level >= len(t) || t[level] != ' ' {
			p.issue(n, "heading: one to five '#' followed by a space")
			return
		}
		p.blocks = append(p.blocks, &Heading{Level: level, Inlines: p.inlines(n, strings.TrimSpace(t[level:]))})
	case strings.HasPrefix(t, "- "):
		if p.para != nil {
			p.flush()
		}
		if !p.inList {
			p.inList, p.firstLine, p.itemIndent = true, n, indent(s)
		} else if indent(s) > p.itemIndent {
			if !p.nest {
				p.issue(n, "nested list item: lists do not nest here (a {{begin-md}} block does)")
			} else {
				last := len(p.items) - 1
				p.subs[last] = append(p.subs[last], []string{strings.TrimSpace(t[2:])})
				return
			}
		}
		p.items = append(p.items, []string{strings.TrimSpace(t[2:])})
		p.subs = append(p.subs, nil)
	default:
		if p.inList {
			last := len(p.items) - 1
			if sub := p.subs[last]; len(sub) > 0 {
				sub[len(sub)-1] = append(sub[len(sub)-1], t)
				return
			}
			p.items[last] = append(p.items[last], t)
			return
		}
		if p.para == nil {
			p.firstLine = n
		}
		p.para = append(p.para, t)
	}
}

// flush closes the open paragraph or list.
func (p *textParser) flush() {
	if p.para != nil {
		p.blocks = append(p.blocks, &Paragraph{Inlines: p.inlines(p.firstLine, strings.Join(p.para, " "))})
		p.para = nil
	}
	if p.inList {
		l := &List{}
		for i, it := range p.items {
			l.Items = append(l.Items, p.inlines(p.firstLine, strings.Join(it, " ")))
			var sub *List
			if len(p.subs[i]) > 0 {
				sub = &List{}
				for _, s := range p.subs[i] {
					sub.Items = append(sub.Items, p.inlines(p.firstLine, strings.Join(s, " ")))
				}
			}
			l.Subs = append(l.Subs, sub)
		}
		p.blocks = append(p.blocks, l)
		p.inList, p.items, p.subs, p.itemIndent = false, nil, nil, -1
	}
}

// inlines parses the inline constructs of one text run; line is the line
// the run starts on, for the issues.
func (p *textParser) inlines(line int, s string) []Inline {
	var out []Inline
	var text strings.Builder
	emit := func() {
		if text.Len() > 0 {
			out = append(out, &Text{Text: text.String()})
			text.Reset()
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '`':
			end := strings.IndexByte(s[i+1:], c)
			if end < 0 {
				p.issue(line, "code span opened with %q is never closed", c)
				text.WriteByte(c)
				i++
				continue
			}
			emit()
			out = append(out, &Code{Text: s[i+1 : i+1+end]})
			i += end + 2
		case c == '*' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "**")
			if end < 0 {
				p.issue(line, "\"**\" is never closed")
				text.WriteString("**")
				i += 2
				continue
			}
			emit()
			out = append(out, &Strong{Text: s[i+2 : i+2+end]})
			i += end + 4
		case c == '[':
			// "[text](target)", text without nesting; any other '[' is text
			close := strings.IndexByte(s[i:], ']')
			if close < 0 || i+close+1 >= len(s) || s[i+close+1] != '(' {
				text.WriteByte(c)
				i++
				continue
			}
			end := strings.IndexByte(s[i+close+2:], ')')
			if end < 0 {
				p.issue(line, "link \"[%s](\" is never closed", s[i+1:i+close])
				text.WriteByte(c)
				i++
				continue
			}
			emit()
			out = append(out, &Link{Text: s[i+1 : i+close], Target: s[i+close+2 : i+close+2+end]})
			i += close + 2 + end + 1
		case c == '{' && i+1 < len(s) && s[i+1] == '{':
			end := strings.Index(s[i+2:], "}}")
			if end < 0 {
				p.issue(line, "\"{{\" is never closed")
				text.WriteString("{{")
				i += 2
				continue
			}
			name, arg, _ := strings.Cut(s[i+2:i+2+end], ":")
			emit()
			out = append(out, &Call{Name: strings.TrimSpace(name), Arg: strings.TrimSpace(arg)})
			i += end + 4
		default:
			text.WriteByte(c)
			i++
		}
	}
	emit()
	return out
}
