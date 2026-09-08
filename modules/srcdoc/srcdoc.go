// Package srcdoc is the Draft 4 scanner of the in-source documentation markers
// (spec 03-srcdoc-spec, "Draft 4 - the authoring model"). It reads a source
// file and returns its topics: the nine kinds of page (function, c-function,
// cpp-function, internal-function, debug-c-function, class, cpp-class,
// note-id, topic), each one a header marker followed by its fragments and
// includes in the order written. Everything that is not a topic header is
// content of the enclosing topic; content outside a topic is an error.
//
// Marker grammar recognized here:
//
//	/*{{begin-<kind>}}*/ ... /*{{end-<kind>}}*/   a composed topic (scope)
//	/*{{<kind>_: ident | field: value ...}}*/     the header (class uses class-name)
//	/*{{|label: value | label2: value ...}}*/      a fragment of the open topic
//	/*{{|: text}}*/                               text placed right there
//	/*{{include-note-id: X}}*/                    transclusion of note X
//
// A header outside a scope is a compact topic: complete in its marker, no
// fragments may follow it. Fields start at a "|" that begins a line or follows
// a blank, outside backticks, and is followed by an optional label and ":".
// Label visibility: a leading "_" hides the whole entry, a trailing "_" hides
// the label only; both are stripped before the label is recognized.
//
// Lines are read with linereader (CR, LF and CRLF all end a line); a source
// that is not CRLF is reported, never rejected.
package srcdoc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pablo-botella/linereader"
)

// Kinds of topic. class-name is the header label of a class topic.
const (
	KindFunction         = "function"
	KindCFunction        = "c-function"
	KindCppFunction      = "cpp-function"
	KindInternalFunction = "internal-function"
	KindDebugCFunction   = "debug-c-function"
	KindClass            = "class"
	KindCppClass         = "cpp-class"
	KindNote             = "note-id"
	KindTopic            = "topic"
)

// Kinds lists the nine built-in topic kinds in presentation order.
var Kinds = []string{KindFunction, KindInternalFunction, KindCFunction, KindDebugCFunction,
	KindCppFunction, KindClass, KindCppClass, KindNote, KindTopic}

// KindTable is the set of topic kinds the scanner recognises: for each one
// its name, how its identity is keyed ("upper", "lower", or "" for
// case-sensitive) and its header aliases. The built-in table is the nine
// kinds of Draft 4; a .doc-tool file replaces it through Use.
type KindTable struct {
	names   []string
	byLabel map[string]*kindDef // name and aliases
}

type kindDef struct {
	name, caseIndex string
}

// NewKindTable returns an empty table.
func NewKindTable() *KindTable { return &KindTable{byLabel: map[string]*kindDef{}} }

// Add declares a kind with its case rule and header aliases.
func (t *KindTable) Add(name, caseIndex string, aliases ...string) {
	d := &kindDef{name: name, caseIndex: caseIndex}
	t.names = append(t.names, name)
	t.byLabel[name] = d
	for _, a := range aliases {
		t.byLabel[a] = d
	}
}

// Names lists the kinds in declaration order.
func (t *KindTable) Names() []string { return append([]string{}, t.names...) }

// Has reports whether name is a kind (not an alias).
func (t *KindTable) Has(name string) bool {
	d := t.byLabel[name]
	return d != nil && d.name == name
}

// HeaderKind maps a header label (a kind name or alias) to its kind; "" when
// the label is not a topic header.
func (t *KindTable) HeaderKind(label string) string {
	if d := t.byLabel[label]; d != nil {
		return d.name
	}
	return ""
}

// Key normalizes an identity for lookup according to the kind's case rule.
func (t *KindTable) Key(kind, ident string) string {
	ident = strings.TrimSpace(ident)
	if d := t.byLabel[kind]; d != nil {
		switch d.caseIndex {
		case "upper":
			return strings.ToUpper(ident)
		case "lower":
			return strings.ToLower(ident)
		}
	}
	return ident
}

// DefaultKinds is the built-in table: Xbase++ names (function,
// internal-function, class) are case-insensitive and keyed upper-case, C and
// C++ names keep their case, note ids and topic names are keyed lower-case;
// class-name is an alias of class.
func DefaultKinds() *KindTable {
	t := NewKindTable()
	t.Add(KindFunction, "upper")
	t.Add(KindInternalFunction, "upper")
	t.Add(KindCFunction, "")
	t.Add(KindDebugCFunction, "")
	t.Add(KindCppFunction, "")
	t.Add(KindClass, "upper", "class-name")
	t.Add(KindCppClass, "")
	t.Add(KindNote, "lower")
	t.Add(KindTopic, "lower")
	return t
}

var table = DefaultKinds()

// Use installs the kind table the scanner (and every layer keyed by it)
// works with; nil restores the built-in one.
func Use(t *KindTable) {
	if t == nil {
		t = DefaultKinds()
	}
	table = t
}

// Table returns the kind table in use.
func Table() *KindTable { return table }

// HeaderKind maps a header label to its topic kind ("class-name" -> class);
// "" when the label is not a topic header.
func HeaderKind(label string) string { return table.HeaderKind(label) }

// Key normalizes an identity for lookup with the kind's case rule.
func Key(kind, ident string) string { return table.Key(kind, ident) }

// isKind reports whether name is a kind of the table in use.
func isKind(name string) bool { return table.Has(name) }

// MarkerKind tells what a marker is.
type MarkerKind int

const (
	MkBegin    MarkerKind = iota + 1 // begin-<kind>
	MkEnd                            // end-<kind>
	MkHeader                         // <kind>: ident ...
	MkFragment                       // |label: ... or |: text
	MkInclude                        // include-note-id: X (alone)
)

// Field is one "label: value" entry of a marker. Label is canonical (its
// visibility underscores stripped); an empty Label is the "|:" text entry.
// Value keeps the continuation lines verbatim (joined with "\n"): the renderer
// dedents them.
type Field struct {
	Label     string
	HideEntry bool
	HideLabel bool
	Value     string
	Line      int
}

// Marker is one /*{{ }}*/ marker.
type Marker struct {
	Kind     MarkerKind
	Scope    string  // begin/end: the scope kind; header: the topic kind
	Ident    string  // header: the identity as written
	Fields   []Field // header (identity excluded) and fragment fields
	Line     int     // first line, 1-based
	EndLine  int
	Raw      string // the marker text as in the source, lines joined with "\n"
	Trailing bool   // the marker follows code on its line
}

// Topic is one page-level unit: its header marker and, for a composed topic,
// the fragments and includes written inside its scope, in order.
type Topic struct {
	Kind    string
	Ident   string
	Key     string
	Line    int
	Compact bool
	Markers []*Marker // Markers[0] is the header
}

// Field returns the first header field with that label, or nil.
func (t *Topic) Field(label string) *Field {
	for i := range t.Markers[0].Fields {
		if t.Markers[0].Fields[i].Label == label {
			return &t.Markers[0].Fields[i]
		}
	}
	return nil
}

// Issue is a diagnostic tied to a line.
type Issue struct {
	Line     int
	Severity string // "error" | "warning"
	Code     string
	Message  string
}

// File is the outcome of scanning one source.
type File struct {
	Name    string
	Topics  []*Topic
	Issues  []Issue
	Lines   int
	NonCRLF int // lines not ended by CRLF (a warning for Xbase++ sources)
}

func (f *File) issue(line int, sev, code, msg string) {
	f.Issues = append(f.Issues, Issue{Line: line, Severity: sev, Code: code, Message: msg})
}

// Errors counts the error-severity issues.
func (f *File) Errors() int {
	n := 0
	for _, is := range f.Issues {
		if is.Severity == "error" {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- lines

func splitLines(src []byte) (lines [][]byte, nonCRLF int) {
	lr := linereader.NewLineReader(bytes.NewReader(src), 0, uint64(len(src))+1)
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			break
		}
		lines = append(lines, bytes.Clone(raw))
		switch lr.LastEolType {
		case linereader.EolLf, linereader.EolCr:
			nonCRLF++
		}
	}
	return lines, nonCRLF
}

// ---------------------------------------------------------------- lexing

const (
	open  = "/*{{"
	close = "}}*/"
)

// lexMarkers extracts every marker of the file with its raw text and lines.
func lexMarkers(f *File, lines [][]byte) []*Marker {
	var out []*Marker
	for i := 0; i < len(lines); i++ {
		s := string(lines[i])
		p := strings.Index(s, open)
		if p < 0 {
			continue
		}
		mk := &Marker{Line: i + 1, Trailing: strings.TrimSpace(s[:p]) != ""}
		rest := s[p+len(open):]
		var body []string
		j := i
		for {
			q := strings.Index(rest, close)
			if q >= 0 {
				body = append(body, rest[:q])
				break
			}
			body = append(body, rest)
			j++
			if j >= len(lines) {
				f.issue(mk.Line, "error", "unterminated-marker", "marker opened here is never closed with }}*/")
				break
			}
			rest = string(lines[j])
		}
		mk.EndLine = j + 1
		mk.Raw = strings.Join(body, "\n")
		out = append(out, mk)
		i = j
	}
	return out
}

// fieldStart reports whether a field starts at text[i] ('|' there): the '|'
// must begin the text, a line, or follow a blank, and be followed by an
// optional label and a ':'. It returns the label as written and the index
// after the ':'.
func fieldStart(text string, i int) (label string, after int, ok bool) {
	if text[i] != '|' {
		return "", 0, false
	}
	if i > 0 && !(text[i-1] == ' ' || text[i-1] == '\t' || text[i-1] == '\n') {
		return "", 0, false
	}
	j := i + 1
	for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
		j++
	}
	k := j
	for k < len(text) && (isLabelByte(text[k])) {
		k++
	}
	label = text[j:k]
	m := k
	for m < len(text) && (text[m] == ' ' || text[m] == '\t') {
		m++
	}
	if m >= len(text) || text[m] != ':' {
		return "", 0, false
	}
	if label == "" && m != j {
		return "", 0, false // "|   :" with blanks but no label: not a field
	}
	return label, m + 1, true
}

func isLabelByte(c byte) bool {
	return c == '_' || c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// splitFields cuts a marker body into its fields. head is the text before the
// first field (the header "kind: ident" or empty for a fragment).
func splitFields(text string, firstLine int) (head string, fields []Field, oddTicks []int) {
	type cut struct {
		start, valueAt int
		label          string
	}
	var cuts []cut
	inTick := false
	fence := false
	md := false // inside {{begin-md}} ... {{end-md}}: no field starts, like a fence
	line := firstLine
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\n' {
			if inTick {
				// a code span does not cross a line here: an odd backtick on
				// a line is reported and stops hiding the fields that follow
				oddTicks = append(oddTicks, line)
				inTick = false
			}
			line++
		}
		if c == '{' && (strings.HasPrefix(text[i:], "{{begin-md}}") || strings.HasPrefix(text[i:], "{{begin-md:")) {
			md = true
			if e := strings.Index(text[i:], "}}"); e >= 0 {
				i += e + 1
			}
			continue
		}
		if c == '{' && strings.HasPrefix(text[i:], "{{end-md}}") {
			md = false
			i += len("{{end-md}}") - 1
			continue
		}
		if md {
			continue
		}
		if c == '`' {
			if strings.HasPrefix(text[i:], "```") {
				fence = !fence
				i += 2
				continue
			}
			if !fence {
				inTick = !inTick
			}
			continue
		}
		if inTick || fence {
			continue
		}
		if c == '|' {
			if label, after, ok := fieldStart(text, i); ok {
				cuts = append(cuts, cut{start: i, valueAt: after, label: label})
				i = after - 1
			}
		}
	}
	if inTick {
		oddTicks = append(oddTicks, line)
	}
	if len(cuts) == 0 {
		return strings.TrimSpace(text), nil, oddTicks
	}
	head = strings.TrimSpace(text[:cuts[0].start])
	for n, ct := range cuts {
		end := len(text)
		if n+1 < len(cuts) {
			end = cuts[n+1].start
		}
		val := text[ct.valueAt:end]
		val = strings.TrimPrefix(val, " ")
		val = strings.TrimRight(val, " \t\n")
		fd := Field{Value: val, Line: firstLine + strings.Count(text[:ct.start], "\n")}
		fd.Label, fd.HideEntry, fd.HideLabel = canonLabel(ct.label)
		fields = append(fields, fd)
	}
	return head, fields, oddTicks
}

func (f *File) oddTickIssues(lines []int) {
	for _, l := range lines {
		f.issue(l, "warning", "odd-backtick", "a backtick is not closed on this line: the fields after it would be hidden; write a literal backtick as text")
	}
}

// canonLabel strips the visibility underscores: only the first and the last
// one count.
func canonLabel(l string) (label string, hideEntry, hideLabel bool) {
	if l == "" {
		return "", false, false
	}
	if strings.HasPrefix(l, "_") && len(l) > 1 {
		hideEntry = true
		l = l[1:]
	}
	if strings.HasSuffix(l, "_") && len(l) > 1 {
		hideLabel = true
		l = l[:len(l)-1]
	}
	return l, hideEntry, hideLabel
}

// parseMarker classifies a lexed marker and fills Scope/Ident/Fields.
func parseMarker(f *File, mk *Marker) {
	text := mk.Raw
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "begin-") {
		mk.Kind = MkBegin
		mk.Scope = strings.TrimSpace(strings.TrimPrefix(t, "begin-"))
		return
	}
	if strings.HasPrefix(t, "end-") {
		mk.Kind = MkEnd
		mk.Scope = strings.TrimSpace(strings.TrimPrefix(t, "end-"))
		return
	}
	// leading blanks before a "|" would defeat fieldStart's line/blank rule
	// only in the "i > 0" check; normalize by trimming the left side.
	lead := len(text) - len(strings.TrimLeft(text, " \t\n"))
	body := text[lead:]
	if strings.HasPrefix(body, "|") {
		mk.Kind = MkFragment
		var odd []int
		_, mk.Fields, odd = splitFields(body, mk.Line+strings.Count(text[:lead], "\n"))
		f.oddTickIssues(odd)
		return
	}
	// header or include: the first entry is "label: value" without a "|"
	head, fields, odd := splitFields(body, mk.Line)
	f.oddTickIssues(odd)
	label, val := head, ""
	if p := strings.Index(head, ":"); p >= 0 {
		label, val = strings.TrimSpace(head[:p]), strings.TrimSpace(head[p+1:])
	} else {
		f.issue(mk.Line, "error", "bad-marker", fmt.Sprintf("marker head %q is not 'label: value'", firstLine(head)))
		mk.Kind = MkFragment
		mk.Fields = fields
		return
	}
	lab, hideEntry, hideLabel := canonLabel(label)
	if lab == "include-note-id" {
		mk.Kind = MkInclude
		mk.Fields = append([]Field{{Label: lab, Value: val, Line: mk.Line}}, fields...)
		return
	}
	kind := HeaderKind(lab)
	if kind == "" {
		f.issue(mk.Line, "error", "unknown-head", fmt.Sprintf("%q is not a topic kind; content must start with '|'", label))
		mk.Kind = MkFragment
		mk.Fields = append([]Field{{Label: lab, HideEntry: hideEntry, HideLabel: hideLabel, Value: val, Line: mk.Line}}, fields...)
		return
	}
	mk.Kind = MkHeader
	mk.Scope = kind
	mk.Ident = firstLine(val)
	// the identity entry is kept as the first field so its visibility travels
	mk.Fields = append([]Field{{Label: lab, HideEntry: hideEntry, HideLabel: hideLabel, Value: mk.Ident, Line: mk.Line}}, fields...)
}

func firstLine(s string) string {
	if p := strings.IndexByte(s, '\n'); p >= 0 {
		return strings.TrimSpace(s[:p])
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------- scanning

// Scan parses one source given its content. name is only used for reporting.
func Scan(name string, data []byte) *File {
	f := &File{Name: name}
	lines, nonCRLF := splitLines(data)
	f.Lines = len(lines)
	f.NonCRLF = nonCRLF
	if nonCRLF > 0 {
		f.issue(0, "warning", "not-crlf", fmt.Sprintf("%d line(s) not ended by CRLF", nonCRLF))
	}
	markers := lexMarkers(f, lines)
	for _, mk := range markers {
		parseMarker(f, mk)
	}
	var scope *Topic // open composed topic (nil = top level)
	var scopeKind string
	var scopeLine int
	for _, mk := range markers {
		switch mk.Kind {
		case MkBegin:
			if !isKind(mk.Scope) {
				f.issue(mk.Line, "error", "unknown-kind", fmt.Sprintf("begin-%s: unknown topic kind", mk.Scope))
				continue
			}
			if scope != nil || scopeKind != "" {
				f.issue(mk.Line, "error", "nested-scope", fmt.Sprintf("begin-%s inside the %s scope opened at line %d", mk.Scope, scopeKind, scopeLine))
				closeScope(f, &scope, &scopeKind, scopeLine)
			}
			scopeKind, scopeLine = mk.Scope, mk.Line
		case MkEnd:
			if scopeKind == "" {
				f.issue(mk.Line, "error", "stray-end", fmt.Sprintf("end-%s without a begin", mk.Scope))
				continue
			}
			if mk.Scope != scopeKind {
				f.issue(mk.Line, "error", "scope-mismatch", fmt.Sprintf("end-%s closes a begin-%s (line %d)", mk.Scope, scopeKind, scopeLine))
			}
			closeScope(f, &scope, &scopeKind, scopeLine)
		case MkHeader:
			if scopeKind == "" {
				t := newTopic(mk)
				t.Compact = true
				f.Topics = append(f.Topics, t)
				continue
			}
			if mk.Scope != scopeKind {
				f.issue(mk.Line, "error", "header-mismatch", fmt.Sprintf("%s header inside a begin-%s scope", mk.Scope, scopeKind))
			}
			if scope != nil {
				f.issue(mk.Line, "error", "second-header", fmt.Sprintf("second header in the scope of %s %s (line %d); one scope, one topic", scope.Kind, scope.Ident, scope.Line))
				continue
			}
			scope = newTopic(mk)
			f.Topics = append(f.Topics, scope)
		case MkFragment, MkInclude:
			if scope == nil {
				what := "fragment"
				if mk.Kind == MkInclude {
					what = "include-note-id"
				}
				if scopeKind != "" {
					f.issue(mk.Line, "error", "content-before-header", fmt.Sprintf("%s before the header of the begin-%s scope (line %d)", what, scopeKind, scopeLine))
				} else {
					f.issue(mk.Line, "error", "content-outside-topic", fmt.Sprintf("%s outside any topic: nothing to attach it to", what))
				}
				continue
			}
			scope.Markers = append(scope.Markers, mk)
		}
	}
	if scopeKind != "" {
		f.issue(scopeLine, "error", "unclosed-scope", fmt.Sprintf("begin-%s never closed", scopeKind))
		closeScope(f, &scope, &scopeKind, scopeLine)
	}
	return f
}

func newTopic(mk *Marker) *Topic {
	return &Topic{Kind: mk.Scope, Ident: mk.Ident, Key: Key(mk.Scope, mk.Ident), Line: mk.Line, Markers: []*Marker{mk}}
}

func closeScope(f *File, scope **Topic, scopeKind *string, scopeLine int) {
	if *scope == nil && *scopeKind != "" {
		f.issue(scopeLine, "error", "empty-scope", fmt.Sprintf("begin-%s without a header", *scopeKind))
	}
	*scope = nil
	*scopeKind = ""
}

// ScanFile reads and scans one file.
func ScanFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Scan(path, data), nil
}

// Extensions is the set of source extensions the doc scanner looks into.
var Extensions = map[string]bool{".cpp": true, ".h": true, ".c": true, ".prg": true, ".ch": true, ".xbmac": false}

// ScanDir scans every source of a directory tree (extensions in Extensions),
// in sorted path order.
func ScanDir(root string) ([]*File, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if Extensions[strings.ToLower(filepath.Ext(p))] {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []*File
	for _, p := range paths {
		f, err := ScanFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// Categories splits a category field value on commas.
func Categories(v string) []string {
	var out []string
	for _, c := range strings.Split(v, ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// Dedent removes the common leading blank run of the continuation lines of a
// value (the first line is left as is): the renderer's view of a multi-line
// field.
func Dedent(v string) string {
	lines := strings.Split(v, "\n")
	if len(lines) < 2 {
		return v
	}
	common := -1
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return v
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = lines[i][common:]
	}
	return strings.Join(lines, "\n")
}
