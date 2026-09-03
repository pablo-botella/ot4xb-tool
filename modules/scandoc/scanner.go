package scandoc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/linereader"
)

const (
	markerOpen  = "/*{{"
	markerClose = "}}*/"
)

// Scan parses one source file. The returned error is I/O only: grammar and
// lint problems come back in File.Diags. The file is never modified.
func Scan(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return scanBytes(path, data), nil
}

// ScanText parses in-memory source text (the same way Scan parses a file);
// name is what diagnostics and File.Path carry. The documentation database
// stores each segment's raw marker text and re-parses it through here.
func ScanText(name string, data []byte) *File { return scanBytes(name, data) }

// ScanDir scans every C/C++ source (.cpp, .c, .h, .hpp) directly inside dir,
// in name order. Files without markers come back with an empty model.
func ScanDir(dir string) ([]*File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*File
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".cpp", ".c", ".h", ".hpp":
			f, err := Scan(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			out = append(out, f)
		}
	}
	return out, nil
}

// FSM states.
const (
	stIdle        = iota // normal source (or scope body); watching for markers
	stHeader             // inside a header marker, accumulating fields
	stFence              // inside a code fence within a header field
	stNoteMarker         // inside a begin-note marker (note-id / title)
	stNoteCollect        // between begin-note and end-note: gathering note: fragments (code ignored)
	stNoteFrag           // inside a multi-line /*{{note: ...}}*/ fragment
	stRaw                // inside begin-markdown-free: verbatim body, not parsed
	stTopic              // inside a topic scope: collecting free-form content
)

// Continuation targets: what a plain (non-field) chunk inside a marker
// appends to.
const (
	contNone = iota
	contIdent
	contField
	contTitle
)

var beginKind = map[string]Kind{
	"begin-c-function":        KindCFunction,
	"begin-cpp-function":      KindCppFunction,
	"begin-function":          KindFunction,
	"begin-class":             KindClass,
	"begin-structure":         KindStructure,
	"begin-cpp-class":         KindCppClass,
	"begin-internal-function": KindInternalFunction,
	"begin-debug-c-function":  KindDebugCFunction,
	"begin-topic":             KindTopic,
}

var endKind = map[string]Kind{
	"end-c-function":        KindCFunction,
	"end-cpp-function":      KindCppFunction,
	"end-function":          KindFunction,
	"end-class":             KindClass,
	"end-structure":         KindStructure,
	"end-cpp-class":         KindCppClass,
	"end-internal-function": KindInternalFunction,
	"end-debug-c-function":  KindDebugCFunction,
	"end-topic":             KindTopic,
}

// keywordKind maps a header keyword to its kind. class keeps `class-name` (its
// identity field); structure/cpp-class and the function twins use `<kind>:`
// with the name as the value.
var keywordKind = map[string]Kind{
	"c-function":        KindCFunction,
	"cpp-function":      KindCppFunction,
	"function":          KindFunction,
	"class-name":        KindClass,
	"structure":         KindStructure,
	"cpp-class":         KindCppClass,
	"internal-function": KindInternalFunction,
	"debug-c-function":  KindDebugCFunction,
	"topic":             KindTopic,
	"note-id":           KindNote, // the compact note form: a single note-id: marker
}

// itemKinds is the vocabulary of class/structure auxiliary item markers; each
// becomes a Component of the enclosing scope. gwst-class is a bare flag,
// handled apart; fragment markers (desc, param, ...) are not entities.
var itemKinds = map[string]bool{
	"method": true, "ivar": true, "property": true,
	"class-method": true, "class-var": true, "class-property": true,
	"gwst-member": true,
}

// knownFields is the vocabulary of field names the grammar gives a meaning
// to today. The set is open: a name outside it is kept and transported all
// the same, only flagged Field.Unknown (no diagnostic). Names that carry a
// parameter ("param nFlags", "flag 0x01") are matched by their first word.
var knownFields = map[string]bool{
	"access": true, "calls": true, "category": true, "class-function": true,
	"class-method": true, "class-property": true, "class-var": true,
	"deprecated": true, "desc": true, "example": true, "flag": true,
	"gwst-class": true, "gwst-member": true, "gwst-parent": true,
	"header": true, "ilink": true, "introduced": true, "ivar": true,
	"mangled-name": true, "method": true, "note": true, "param": true,
	"parent": true, "pos": true, "property": true, "prototype": true,
	"return": true, "see-also": true, "since": true, "size": true, "slug": true,
	"syntax": true, "todo": true, "type": true, "xbase-syntax": true,
}

// componentBaseName extracts a component's identity anchor from its authored
// value: a method/class-method keeps the name before its "(" (the argument list
// is display, not identity - Xbase++ has no overloads); every other item keeps
// the first whitespace-delimited token (a var-like name, or the name of a
// gwst-member whose trailing "type: X pos: Y size: Z" is descriptive text).
func componentBaseName(itemKind, ident string) string {
	ident = strings.TrimSpace(ident)
	if itemKind == "method" || itemKind == "class-method" {
		if before, _, ok := strings.Cut(ident, "("); ok {
			return strings.TrimSpace(before)
		}
	}
	for i := 0; i < len(ident); i++ {
		if ident[i] == ' ' || ident[i] == '\t' {
			return ident[:i]
		}
	}
	return ident
}

// knownField reports whether a field name's first word is in the known
// vocabulary (case-insensitive).
func knownField(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] == ' ' || name[i] == '\t' {
			name = name[:i]
			break
		}
	}
	return knownFields[strings.ToLower(name)]
}

// Line-ending kinds; anything but CRLF is a lint finding.
const (
	eolCRLF = iota
	eolLF
	eolCR
	eolNone // last line without terminator
)

// srcLine is one physical line: text decoded from Windows-1252, terminator
// stripped; blen is the raw byte length without the terminator.
type srcLine struct {
	text string
	n    int // 1-based line number
	blen int
	eol  int
}

// cp1252High maps bytes 0x80-0x9F to their Windows-1252 code points (the
// rest of the high half coincides with Latin-1).
var cp1252High = [32]rune{
	0x20AC, 0x0081, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x008D, 0x017D, 0x008F,
	0x0090, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x009D, 0x017E, 0x0178,
}

func decode1252(b []byte) string {
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	var sb strings.Builder
	sb.Grow(len(b) + 8)
	for _, c := range b {
		switch {
		case c < 0x80:
			sb.WriteByte(c)
		case c < 0xA0:
			sb.WriteRune(cp1252High[c-0x80])
		default:
			sb.WriteRune(rune(c))
		}
	}
	return sb.String()
}

// splitSrcLines reads the source line by line with linereader (CR, LF and CRLF
// all end a line, as everywhere else in the tool), decoding each line from
// Windows-1252 and recording what terminated it. maxLineSize is len(data)+1: no
// line can exceed the whole input, so "line too long" cannot happen and io.EOF
// is the only way the loop ends.
func splitSrcLines(data []byte) []srcLine {
	lr := linereader.NewLineReader(bytes.NewReader(data), 0, uint64(len(data))+1)
	var out []srcLine
	n := 0
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			break
		}
		n++
		eol := eolNone // EolEof: the last line carried no terminator
		switch lr.LastEolType {
		case linereader.EolCrLf:
			eol = eolCRLF
		case linereader.EolLf:
			eol = eolLF
		case linereader.EolCr:
			eol = eolCR
		}
		out = append(out, srcLine{decode1252(raw), n, len(raw), eol})
	}
	return out
}

type scanner struct {
	f     *File
	state int
	cont  int

	// open begin-* scope (scopes never nest)
	scope           *Entity
	scopeHeaderDone bool

	// header being parsed; either the scope's or a stand-alone one
	hdr         *Entity
	hdrOwned    bool   // hdr == scope
	hdrStart    int    // header marker line, for diagnostics
	hdrItemKind string // non-empty while parsing a class-auxiliary item marker (a Component)
	identDone   bool   // the identity is frozen: a field separator '|' has appeared

	// open code fence inside a header field
	fenceChar  byte
	fenceLen   int
	fenceStart int

	// shared note being parsed (begin-note ... end-note, composed of note: fragments)
	note      *Note
	noteBody  []string // one entry per collected note: fragment text
	frag      []string // the current multi-line note: fragment being accumulated
	fragStart int      // line the current fragment opened on

	// raw markdown-free block being collected; rawEmbed says where it belongs:
	// 0 = a topic of its own, 1 = inside an entity scope (a verbatim field of
	// it), 2 = inside a composed note (a fragment of it)
	rawStart int
	rawBody  []string
	rawEmbed int

	// open topic scope body (free-form content) being collected
	topicBody  []string
	topicStart int

	// raw marker text of the construct being built (segment blob): every marker
	// and marker-continuation line, never the code between them. Handed over at
	// each finalize point; an item's lines are carved out for its Component.
	raw          []string
	rawItemStart int // index in raw where the current item marker began, -1 if none

	inComment bool
	lastLine  int
}

func scanBytes(path string, data []byte) *File {
	f := &File{Path: path}
	s := &scanner{f: f, state: stIdle, cont: contNone, rawItemStart: -1}
	badEol, firstBadEol := 0, 0
	for _, ln := range splitSrcLines(data) {
		if ln.eol != eolCRLF {
			badEol++
			if firstBadEol == 0 {
				firstBadEol = ln.n
			}
		}
		wasInComment := s.inComment
		s.trackComments(ln)
		s.captureRaw(ln, wasInComment) // before dispatch: a closing line must be in the buffer when its construct finalizes
		switch s.state {
		case stIdle:
			s.idleLine(ln, wasInComment)
		case stHeader:
			s.headerLine(ln)
		case stFence:
			s.fenceLine(ln)
		case stNoteMarker:
			s.noteMarkerLine(ln)
		case stNoteCollect:
			s.noteCollectLine(ln)
		case stNoteFrag:
			s.noteFragLine(ln)
		case stRaw:
			s.rawLine(ln)
		case stTopic:
			s.topicLine(ln)
		}
		s.lastLine = ln.n
	}
	s.eof()
	s.checkDuplicates()
	if badEol > 0 {
		s.diag(Warning, firstBadEol, "%d line(s) do not end in CRLF", badEol)
	}
	return f
}

func (s *scanner) diag(sev Severity, line int, format string, a ...any) {
	s.f.Diags = append(s.f.Diags, Diag{Severity: sev, Line: line, Msg: fmt.Sprintf(format, a...)})
}

// trackComments follows the /* ... */ state across the whole file, with
// line-comment and string-literal awareness outside comments, so that a
// stray */ (or a comment left open at EOF) is reported. Doc markers are
// ordinary comments to it.
func (s *scanner) trackComments(ln srcLine) {
	t := ln.text
	var inStr byte
	for i := 0; i < len(t); i++ {
		c := t[i]
		if s.inComment {
			if c == '*' && i+1 < len(t) && t[i+1] == '/' {
				s.inComment = false
				i++
			}
			continue
		}
		if inStr != 0 {
			switch c {
			case '\\':
				i++
			case inStr:
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
		case '/':
			if i+1 < len(t) {
				switch t[i+1] {
				case '/':
					return
				case '*':
					s.inComment = true
					i++
				}
			}
		case '*':
			if i+1 < len(t) && t[i+1] == '/' {
				s.diag(Error, ln.n, "*/ without a matching /*")
				i++
			}
		}
	}
}

// lintDocLine applies the lint rules that concern documentation lines:
// length (120 normally, 140 inside code fences) and non-ASCII bytes.
func (s *scanner) lintDocLine(ln srcLine, limit int) {
	if ln.blen > limit {
		s.diag(Warning, ln.n, "line is %d characters long (limit %d)", ln.blen, limit)
	}
	for _, r := range ln.text {
		if r > 0x7F {
			s.diag(Warning, ln.n, "non-ASCII character %q", r)
			break
		}
	}
}

// markerContent returns the text between /*{{ and }}*/ (or to end of line
// when the marker does not close on it) of a chunk starting at /*{{.
func markerContent(t string) (content string, closed bool) {
	rest := t[len(markerOpen):]
	if before, _, ok := strings.Cut(rest, markerClose); ok {
		return strings.TrimSpace(before), true
	}
	return strings.TrimSpace(rest), false
}

// splitMarkerKey splits a marker content into its first word and the rest;
// colon tells whether the word introduces a value ("function:", "note:").
func splitMarkerKey(content string) (key string, colon bool, rest string) {
	for i := 0; i < len(content); i++ {
		switch content[i] {
		case ':':
			return content[:i], true, content[i+1:]
		case ' ', '\t':
			return content[:i], false, content[i+1:]
		}
	}
	return content, false, ""
}

// runLen returns the length of the run of character c starting at t[i].
func runLen(t string, i int, c byte) int {
	j := i
	for j < len(t) && t[j] == c {
		j++
	}
	return j - i
}

// splitFields splits marker text on '|' separators, with the markdown
// rules and no regular expressions: a CommonMark code span (a run of N
// backticks closed by a run of exactly N on the same line) is opaque - a
// '|' inside it is text; an unpaired backtick is a plain literal character;
// a backslash removes the structural meaning of the next character
// ("\|", "\`", "\\"). Everything is kept as written - the caller stores
// values raw.
func splitFields(t string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(t); {
		switch t[i] {
		case '\\':
			i += 2 // escaped character: no structural meaning
		case '`':
			n := runLen(t, i, '`')
			if j := findRun(t, i+n, '`', n); j >= 0 {
				i = j + n // closed span: skip it whole, content is opaque
			} else {
				i += n // unpaired: literal characters
			}
		case '|':
			parts = append(parts, t[start:i])
			start = i + 1
			i++
		default:
			i++
		}
	}
	return append(parts, t[start:])
}

// findRun finds the start of the next run of EXACTLY n characters c in t
// at or after from (CommonMark: a longer run does not close an n-run), or
// -1.
func findRun(t string, from int, c byte, n int) int {
	for i := from; i < len(t); {
		if t[i] != c {
			i++
			continue
		}
		r := runLen(t, i, c)
		if r == n {
			return i
		}
		i += r
	}
	return -1
}

// fenceOpen reports whether a field value ends with an opening code fence:
// its last blank-separated token is a run of 3+ backticks or 3+ tildes,
// optionally continued by an info string (` ```prg `) free of fence
// characters. Returns the fence character and run length.
func fenceOpen(value string) (ch byte, n int, ok bool) {
	i := strings.LastIndexAny(value, " \t")
	tok := value[i+1:]
	if tok == "" {
		return 0, 0, false
	}
	c := tok[0]
	if c != '`' && c != '~' {
		return 0, 0, false
	}
	r := runLen(tok, 0, c)
	if r < 3 {
		return 0, 0, false
	}
	if strings.ContainsAny(tok[r:], "`~") {
		return 0, 0, false
	}
	return c, r, true
}

// isFenceClose reports whether a trimmed line closes a fence of n
// characters c: a run of at least n of c and nothing else.
func isFenceClose(t string, c byte, n int) bool {
	if len(t) < n {
		return false
	}
	return runLen(t, 0, c) == len(t)
}

// idleLine watches normal source (and scope bodies) for markers. Field
// alignment is writing convention, not grammar: everything is matched after
// trimming blanks. Item and fragment markers (method:, ivar:, flag ..., a
// free note:) are not scope material and are skipped. wasInComment guards
// against lines that only look like markers because they sit inside an open
// comment.
func (s *scanner) idleLine(ln srcLine, wasInComment bool) {
	idx := strings.Index(ln.text, markerOpen)
	if idx < 0 || wasInComment {
		return
	}
	s.lintDocLine(ln, 120)
	content, closed := markerContent(ln.text[idx:])
	if content == "" {
		s.diag(Error, ln.n, "empty marker: /*{{ with nothing after")
		return
	}
	key, colon, rest := splitMarkerKey(content)
	kl := strings.ToLower(key)
	if !colon {
		switch kl {
		case "begin-note":
			s.note = &Note{StartLine: ln.n}
			s.rawBegin()
			s.noteBody = nil
			s.frag = nil
			s.cont = contNone
			s.noteMarkerText(rest, ln.n)
			if closed {
				s.closeNoteMarker()
			} else {
				s.state = stNoteMarker
			}
			return
		case "end-note":
			s.diag(Error, ln.n, "end-note without begin-note")
			return
		case "begin-markdown-free":
			s.rawStart = ln.n
			s.rawBody = nil
			if s.scope != nil {
				s.rawEmbed = 1 // part of the open entity; its lines stay in the entity's raw buffer
			} else {
				s.rawEmbed = 0
				s.rawBegin()
			}
			s.state = stRaw
			return
		case "end-markdown-free":
			s.diag(Error, ln.n, "end-markdown-free without begin-markdown-free")
			return
		}
		if k, ok := beginKind[kl]; ok {
			s.beginScope(k, closed, ln.n)
			return
		}
		if k, ok := endKind[kl]; ok {
			s.endScope(k, ln.n)
			return
		}
		return // some other self-standing marker (gwst-class, ...): not ours
	}
	if kl == "include-note-id" {
		if !closed {
			s.diag(Error, ln.n, "include-note-id marker not closed with }}*/")
			return
		}
		s.recordInclude(strings.TrimSpace(rest), ln.n)
		return
	}
	if k, ok := keywordKind[kl]; ok {
		s.beginHeader(k, kl, rest, closed, ln.n)
		return
	}
	if s.scope != nil && s.hdrItemKind == "" && itemKinds[kl] {
		s.beginItem(kl, rest, closed, ln.n)
		return
	}
	// any other "kind: value" item or fragment marker: not ours
}

// captureRaw keeps the raw text of every marker line and marker-continuation
// line in s.raw - never the C/C++ between markers. It runs BEFORE the line is
// dispatched, so the line that closes a construct is already in the buffer
// when that construct finalizes and takes it (takeRaw / takeItemRaw).
func (s *scanner) captureRaw(ln srcLine, wasInComment bool) {
	switch s.state {
	case stHeader, stFence, stNoteMarker, stNoteFrag, stRaw, stTopic:
		s.raw = append(s.raw, ln.text)
	default: // stIdle, stNoteCollect: only marker lines, not the interleaved code
		if !wasInComment && strings.Contains(ln.text, markerOpen) {
			s.raw = append(s.raw, ln.text)
		}
	}
}

// rawBegin makes the construct starting on the line just captured the owner of
// the buffer: anything captured before it (a stray ignored marker between two
// constructs) is dropped.
func (s *scanner) rawBegin() {
	if n := len(s.raw); n > 1 {
		s.raw = s.raw[n-1:]
	}
	s.rawItemStart = -1
}

// takeRaw hands the whole buffer to the construct being finalized.
func (s *scanner) takeRaw() []string {
	r := s.raw
	s.raw = nil
	s.rawItemStart = -1
	return r
}

// takeItemRaw hands an item marker's lines (from where the item began) to its
// Component and removes them from the enclosing scope's buffer: a component is
// a segment of its own, not part of the class's blob.
func (s *scanner) takeItemRaw() []string {
	if s.rawItemStart < 0 || s.rawItemStart > len(s.raw) {
		return nil
	}
	r := append([]string(nil), s.raw[s.rawItemStart:]...)
	s.raw = s.raw[:s.rawItemStart]
	s.rawItemStart = -1
	return r
}

// popRaw removes and returns the last captured line (a one-line construct that
// must not stay in the enclosing buffer, e.g. a command inside a topic).
func (s *scanner) popRaw() []string {
	n := len(s.raw)
	if n == 0 {
		return nil
	}
	r := []string{s.raw[n-1]}
	s.raw = s.raw[:n-1]
	return r
}

// closeRaw finishes a markdown-free block at line n according to where it
// was opened: on its own it is a topic (an entity of its own); inside an
// entity scope it becomes a verbatim "markdown-free" field of that entity,
// its lines staying in the entity's raw buffer; inside a composed note it is
// one more fragment of the note's body.
func (s *scanner) closeRaw(n int) {
	body := strings.Join(s.rawBody, "\n")
	switch s.rawEmbed {
	case 1:
		s.scope.Fields = append(s.scope.Fields, Field{Name: "markdown-free", Value: body, Line: s.rawStart})
		s.state = stIdle
	case 2:
		if strings.TrimSpace(body) != "" {
			s.noteBody = append(s.noteBody, body)
		}
		s.state = stNoteCollect
	default:
		s.f.Entities = append(s.f.Entities, Entity{
			Kind:      KindMarkdownFree,
			Fields:    []Field{{Name: "body", Value: body, Line: s.rawStart}},
			StartLine: s.rawStart,
			EndLine:   n,
			Raw:       s.takeRaw(),
		})
		s.state = stIdle
	}
	s.rawBody = nil
	s.rawEmbed = 0
}

// rawLine collects a verbatim line of a markdown-free block, watching only for
// the closing /*{{end-markdown-free}}*/ marker - nothing inside is parsed, so a
// pipe, angle bracket or even a /*{{ there is literal content.
func (s *scanner) rawLine(ln srcLine) {
	if idx := strings.Index(ln.text, markerOpen); idx >= 0 {
		content, _ := markerContent(ln.text[idx:])
		key, _, _ := splitMarkerKey(content)
		if strings.EqualFold(strings.TrimSpace(key), "end-markdown-free") {
			s.closeRaw(ln.n)
			return
		}
	}
	s.rawBody = append(s.rawBody, ln.text)
}

// beginItem starts parsing a class/structure auxiliary item marker (method,
// ivar, ...) as a Component of the current scope, reusing the header machinery
// (headerText/parseField/fenceLine); finishHeader routes it to the scope.
func (s *scanner) beginItem(itemKind, first string, closed bool, n int) {
	s.hdr = &Entity{StartLine: n}
	s.hdrOwned = false
	s.hdrItemKind = itemKind
	s.hdrStart = n
	s.rawItemStart = len(s.raw) - 1 // the item's first line was just captured
	s.identDone = false
	s.cont = contIdent
	s.headerText(first, n)
	if closed {
		if s.state == stFence {
			s.diag(Error, n, "code fence not closed before }}*/")
			s.state = stHeader
		}
		s.finishHeader(n)
		return
	}
	s.state = stHeader
}

func (s *scanner) beginScope(k Kind, closed bool, n int) {
	if !closed {
		s.diag(Error, n, "begin-%s marker not closed with }}*/", k)
		return
	}
	if s.scope != nil {
		s.diag(Error, n, "begin-%s inside begin-%s: scopes never nest", k, s.scope.Kind)
		return
	}
	s.scope = &Entity{Kind: k, StartLine: n}
	s.scopeHeaderDone = false
	s.rawBegin()
}

func (s *scanner) endScope(k Kind, n int) {
	if s.scope == nil {
		s.diag(Error, n, "end-%s without matching begin-%s", k, k)
		return
	}
	if s.scope.Kind != k {
		s.diag(Error, n, "end-%s closes begin-%s (mismatched scope)", k, s.scope.Kind)
	}
	if !s.scopeHeaderDone {
		s.diag(Error, n, "begin-%s scope has no header marker", s.scope.Kind)
	}
	s.scope.EndLine = n
	s.scope.Raw = s.takeRaw()
	s.f.Entities = append(s.f.Entities, *s.scope)
	s.scope = nil
}

// recordInclude registers a loose /*{{include-note-id: X}}*/ marker on the
// open scope (the only place an include is legal).
func (s *scanner) recordInclude(id string, n int) {
	if s.scope == nil {
		s.diag(Error, n, "include-note-id outside a begin/end scope")
		return
	}
	if id == "" {
		s.diag(Error, n, "include-note-id without an id")
		return
	}
	s.scope.NoteRefs = append(s.scope.NoteRefs, id)
	s.scope.noteRefLines = append(s.scope.noteRefLines, n)
}

// beginHeader starts a header marker: the scope's one, or a tolerated
// stand-alone header (emitted with a warning). The header value is the
// entity's identity key, not its signature.
func (s *scanner) beginHeader(k Kind, keyword, first string, closed bool, n int) {
	if s.scope != nil && !s.scopeHeaderDone {
		if k != s.scope.Kind {
			s.diag(Error, n, "header %s: does not match begin-%s", keyword, s.scope.Kind)
		}
		s.hdr = s.scope
		s.hdrOwned = true
		s.scopeHeaderDone = true
	} else {
		// Compact form (Draft 2): a scope-less entity marker is valid - all
		// fields inline, no fence, used when nothing inside needs its own
		// marker. Not a warning.
		s.hdr = &Entity{Kind: k, StartLine: n}
		s.hdrOwned = false
		s.rawBegin()
	}
	s.hdrStart = n
	s.identDone = false
	s.cont = contIdent
	s.headerText(first, n)
	if closed {
		s.finishHeader(n)
		return
	}
	s.state = stHeader
}

// headerLine accumulates one line of a header marker.
func (s *scanner) headerLine(ln srcLine) {
	s.lintDocLine(ln, 120)
	t := strings.TrimSpace(ln.text)
	closing := false
	if before, ok := strings.CutSuffix(t, markerClose); ok {
		closing = true
		t = strings.TrimSpace(before)
	} else if strings.Contains(t, "*/") {
		s.diag(Error, ln.n, "header marker closed by a bare */ (expected }}*/)")
		s.finishHeader(ln.n)
		return
	}
	s.headerText(t, ln.n)
	if closing {
		if s.state == stFence {
			s.diag(Error, ln.n, "code fence not closed before }}*/")
			s.state = stHeader
		}
		s.finishHeader(ln.n)
	}
}

// headerText processes marker text (one physical line, or the tail of the
// opening line): a leading chunk continues the identity or the current
// field; every '|' outside code spans and escapes starts a new field, so
// one line may carry several fields. At the end of the line, a field value
// ending in an opening fence switches to Fence.
func (s *scanner) headerText(t string, n int) {
	if t == "" {
		return
	}
	segs := splitFields(t)
	if lead := strings.TrimSpace(segs[0]); lead != "" {
		s.appendCont(lead, n)
	}
	if len(segs) > 1 {
		s.identDone = true // a '|' appeared: the identity value is complete
	}
	for _, seg := range segs[1:] {
		s.parseField(seg, n)
	}
	if s.cont == contField {
		fd := &s.hdr.Fields[len(s.hdr.Fields)-1]
		if ch, fn, ok := fenceOpen(fd.Value); ok {
			s.state = stFence
			s.fenceChar = ch
			s.fenceLen = fn
			s.fenceStart = n
		}
	}
}

// parseField parses one "name: value" segment into a new field of the
// current header. Values are raw markdown: nothing is unescaped, spans and
// fences keep their delimiters.
func (s *scanner) parseField(seg string, n int) {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return // blank fields are ignored
	}
	name, value, ok := strings.Cut(seg, ":")
	if !ok {
		s.diag(Warning, n, "field without ':': %q", seg)
		return
	}
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	s.hdr.Fields = append(s.hdr.Fields, Field{Name: name, Value: value, Line: n, Unknown: !knownField(name)})
	s.cont = contField
}

// appendCont folds a continuation chunk into the identity or the current
// field, joined with one blank.
func (s *scanner) appendCont(t string, n int) {
	switch s.cont {
	case contIdent:
		if s.identDone {
			// The identity ended at the first '|'; later bare text is a malformed
			// field (e.g. a note body written as "| body" instead of "|: body").
			s.diag(Warning, n, "text after the identity and a '|': expected a field 'name: value'")
			return
		}
		if s.hdr.Ident == "" {
			s.hdr.Ident = t
		} else {
			s.hdr.Ident += " " + t
		}
	case contField:
		fd := &s.hdr.Fields[len(s.hdr.Fields)-1]
		if fd.Value == "" {
			fd.Value = t
		} else {
			fd.Value += " " + t
		}
	default:
		s.diag(Warning, n, "continuation line without a field")
	}
}

// fenceLine copies fence lines verbatim into the current field's value
// (fence delimiters included) until the closing run.
func (s *scanner) fenceLine(ln srcLine) {
	fd := &s.hdr.Fields[len(s.hdr.Fields)-1]
	t := strings.TrimSpace(ln.text)
	if isFenceClose(t, s.fenceChar, s.fenceLen) {
		fd.Value += "\n" + t
		s.state = stHeader
		return
	}
	if strings.HasSuffix(t, markerClose) {
		s.diag(Error, ln.n, "code fence not closed before }}*/")
		s.state = stHeader
		s.finishHeader(ln.n)
		return
	}
	s.lintDocLine(ln, 140)
	fd.Value += "\n" + ln.text
}

// finishHeader closes the header marker: old-form values and field-form
// includes are surfaced (migration debt), todo fields are surfaced, and a
// stand-alone header becomes an entity of its own (a scope header waits for
// its end-* marker).
func (s *scanner) finishHeader(n int) {
	e := s.hdr
	if s.hdrItemKind != "" {
		// A class/structure auxiliary item marker -> a Component of the scope.
		if e.Ident == "" {
			s.diag(Error, s.hdrStart, "%s item marker has no name", s.hdrItemKind)
		}
		s.surfaceFieldDebt(e)
		if s.scope != nil {
			s.scope.Components = append(s.scope.Components, Component{
				Kind:   s.hdrItemKind,
				Ident:  e.Ident,
				Name:   componentBaseName(s.hdrItemKind, e.Ident),
				Fields: e.Fields,
				Line:   s.hdrStart,
				Raw:    s.takeItemRaw(),
			})
		}
		s.hdr = nil
		s.hdrItemKind = ""
		s.cont = contNone
		s.state = stIdle
		return
	}
	if e.Ident == "" {
		s.diag(Error, s.hdrStart, "%s header has no identity", e.Kind)
	} else {
		switch e.Kind {
		case KindFunction, KindClass:
			if strings.Contains(e.Ident, "(") {
				s.diag(Warning, s.hdrStart, "old-form header value (a signature): expected the identity key (the name)")
			}
		case KindCFunction, KindCppFunction:
			if strings.ContainsAny(e.Ident, " \t") {
				s.diag(Warning, s.hdrStart, "old-form header value (a prototype): expected the identity key (name, or name(TYPES) for a cpp overload)")
			}
		}
	}
	if e.Kind == KindNote && !s.hdrOwned {
		// The compact note form: a single /*{{note-id: X |: body | note: ... |
		// include-note-id: dep}}*/ marker, parsed by the header machinery, becomes
		// a shared Note (not an Entity).
		s.finishCompactNote(e, n)
		return
	}
	s.surfaceFieldDebt(e)
	if s.hdrOwned && s.scope != nil && s.scope.Kind == KindTopic {
		// A topic scope's header is done; its body is free-form content,
		// collected verbatim until end-topic. Doc-internal id: lowercased.
		s.scope.Ident = strings.ToLower(s.scope.Ident)
		s.topicBody = nil
		s.topicStart = n + 1
		s.hdr = nil
		s.cont = contNone
		s.state = stTopic
		return
	}
	if !s.hdrOwned {
		e.EndLine = n
		e.Raw = s.takeRaw()
		s.f.Entities = append(s.f.Entities, *e)
	}
	s.hdr = nil
	s.cont = contNone
	s.state = stIdle
}

// topicLine collects one line of a topic's free-form content, watching only for
// its own inner markers: /*{{end-topic}}*/ closes the topic; /*{{command: id
// ...}}*/ registers a referenceable command entity that lives in this topic (its
// marker is an anchor, not prose). Every other line is verbatim content.
func (s *scanner) topicLine(ln srcLine) {
	if idx := strings.Index(ln.text, markerOpen); idx >= 0 {
		content, closed := markerContent(ln.text[idx:])
		key, colon, rest := splitMarkerKey(content)
		kl := strings.ToLower(strings.TrimSpace(key))
		switch {
		case !colon && kl == "end-topic":
			s.closeTopic(ln.n)
			return
		case colon && kl == "command":
			s.topicCommand(rest, closed, ln.n)
			return
		}
	}
	s.lintDocLine(ln, 120)
	s.topicBody = append(s.topicBody, ln.text)
}

// topicCommand registers a command authored inside the open topic: "id | field:
// value | ..." on a single compact marker line. The command is a top-level
// entity (uniform (kind,id) resolution) carrying a "topic" field with its
// container's id; its fields are optional (a command is content, not rigid).
func (s *scanner) topicCommand(rest string, closed bool, n int) {
	if !closed {
		s.diag(Error, n, "command marker not closed with }}*/ (a command is a single compact marker)")
		return
	}
	segs := splitFields(rest)
	id := strings.TrimSpace(segs[0])
	cmd := Entity{Kind: KindCommand, Ident: id, StartLine: n, EndLine: n}
	cmd.Raw = s.popRaw() // its own segment: the marker line leaves the topic's buffer
	cmd.Fields = append(cmd.Fields, Field{Name: "topic", Value: s.scope.Ident, Line: n})
	for _, seg := range segs[1:] {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		name, value, ok := strings.Cut(seg, ":")
		if !ok {
			s.diag(Warning, n, "command field without ':': %q", seg)
			continue
		}
		name = strings.TrimSpace(name)
		cmd.Fields = append(cmd.Fields, Field{Name: name, Value: strings.TrimSpace(value), Line: n, Unknown: !knownField(name)})
	}
	if id == "" {
		s.diag(Error, n, "command marker has no id")
	}
	s.f.Entities = append(s.f.Entities, cmd)
}

// closeTopic finishes the open topic scope: its collected content becomes a
// verbatim "body" field, and the entity is emitted.
func (s *scanner) closeTopic(n int) {
	if s.scope.Ident == "" {
		s.diag(Error, s.scope.StartLine, "topic has no id")
	}
	s.scope.Fields = append(s.scope.Fields, Field{Name: "body", Value: strings.Join(s.topicBody, "\n"), Line: s.topicStart})
	s.scope.EndLine = n
	s.scope.Raw = s.takeRaw()
	s.f.Entities = append(s.f.Entities, *s.scope)
	s.scope = nil
	s.topicBody = nil
	s.state = stIdle
}

// finishCompactNote turns a compact note-id: header (parsed into a temporary
// entity) into a shared Note: the empty-name |: fields and the note: fields are
// its body, title: is the heading, and each include-note-id: field is a
// note->note dependency (composition). ids are lowercased (doc-internal).
func (s *scanner) finishCompactNote(e *Entity, n int) {
	note := Note{ID: strings.ToLower(strings.TrimSpace(e.Ident)), StartLine: s.hdrStart, EndLine: n}
	note.Raw = s.takeRaw()
	var body []string
	for _, fd := range e.Fields {
		switch strings.ToLower(strings.TrimSpace(fd.Name)) {
		case "": // |: the body
			body = append(body, fd.Value)
		case "note": // an extra caveat/body paragraph
			body = append(body, fd.Value)
		case "title":
			if note.Title == "" {
				note.Title = fd.Value
			}
		case "slug":
			note.Slug = strings.TrimSpace(fd.Value)
		case "include-note-id":
			if id := strings.TrimSpace(fd.Value); id != "" {
				note.Includes = append(note.Includes, id)
			}
		}
	}
	note.Body = strings.Join(body, "\n")
	if note.ID == "" {
		s.diag(Error, s.hdrStart, "note-id marker has no id")
	} else if s.f.NoteByID(note.ID) != nil {
		s.diag(Error, s.hdrStart, "duplicate note-id %q", note.ID)
	}
	s.f.Notes = append(s.f.Notes, note)
	s.hdr = nil
	s.cont = contNone
	s.state = stIdle
}

// surfaceFieldDebt reports the migration debt carried in an entity's or
// component's fields: todo fields, and the retired include-note-id-inside-a-
// field form (whose ids are still collected onto e.NoteRefs).
func (s *scanner) surfaceFieldDebt(e *Entity) {
	for _, fd := range e.Fields {
		if strings.EqualFold(fd.Name, "todo") {
			s.diag(Warning, fd.Line, "todo field: %s", fd.Value)
		}
		if strings.EqualFold(fd.Name, "ilink") {
			s.f.checkILink(fd.Value, fd.Line)
		}
		if ids := extractIncludes(fd.Value); len(ids) > 0 {
			s.diag(Warning, fd.Line, "include-note-id inside a field is the retired form: use the loose /*{{include-note-id: X}}*/ marker")
			for _, id := range ids {
				e.NoteRefs = append(e.NoteRefs, id)
				e.noteRefLines = append(e.noteRefLines, fd.Line)
			}
		}
	}
}

// noteMarkerLine accumulates the rest of a multi-line begin-note marker.
func (s *scanner) noteMarkerLine(ln srcLine) {
	s.lintDocLine(ln, 120)
	t := strings.TrimSpace(ln.text)
	closing := false
	if before, ok := strings.CutSuffix(t, markerClose); ok {
		closing = true
		t = strings.TrimSpace(before)
	}
	s.noteMarkerText(t, ln.n)
	if closing {
		s.closeNoteMarker()
	}
}

// noteMarkerText parses "| note-id: x | title: y" segments (or a title
// continuation) of a begin-note marker.
func (s *scanner) noteMarkerText(t string, n int) {
	t = strings.TrimSpace(t)
	if t == "" {
		return
	}
	if !strings.HasPrefix(t, "|") {
		if s.cont == contTitle && s.note.Title != "" {
			s.note.Title += " " + t
		} else {
			s.diag(Warning, n, "unexpected text in begin-note marker: %q", t)
		}
		return
	}
	for _, seg := range splitFields(t[1:]) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		name, value, ok := strings.Cut(seg, ":")
		if !ok {
			s.diag(Warning, n, "begin-note field without ':': %q", seg)
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		switch strings.ToLower(name) {
		case "note-id":
			s.note.ID = value
			s.cont = contNone
		case "title":
			s.note.Title = value
			s.cont = contTitle
		case "include-note-id":
			if value != "" {
				s.note.Includes = append(s.note.Includes, value)
			}
			s.cont = contNone
		case "slug":
			s.note.Slug = value
			s.cont = contNone
		default:
			s.diag(Warning, n, "unknown begin-note field %q", name)
		}
	}
}

func (s *scanner) closeNoteMarker() {
	if s.note.ID == "" {
		s.diag(Error, s.note.StartLine, "begin-note without note-id")
	}
	s.state = stNoteCollect
}

// noteCollectLine walks the region between begin-note and end-note. A shared
// note is COMPOSED: it gathers every /*{{note: ...}}*/ fragment (its text is a
// body paragraph, its include-note-id fields are note->note deps) until
// /*{{end-note}}*/; the real C/C++ code interleaved with the fragments is not
// note content and is ignored (only the /*{{ }}*/ comments carry doc). This
// replaces the old rule that the body had to be one comment right after
// begin-note.
func (s *scanner) noteCollectLine(ln srcLine) {
	idx := strings.Index(ln.text, markerOpen)
	if idx < 0 {
		return // interleaved code / blank / line comment: not note content
	}
	content, closed := markerContent(ln.text[idx:])
	key, colon, rest := splitMarkerKey(content)
	kl := strings.ToLower(strings.TrimSpace(key))
	switch {
	case !colon && kl == "end-note":
		s.registerNote(ln.n)
	case !colon && kl == "begin-markdown-free":
		// raw Markdown inside a composed note: one more fragment, verbatim
		s.rawStart = ln.n
		s.rawBody = nil
		s.rawEmbed = 2
		s.state = stRaw
	case !colon && kl == "begin-note":
		s.diag(Error, ln.n, "begin-note inside begin-note: notes never nest")
	case colon && kl == "note":
		// A note: fragment often rides at the end of a real code line (case ...:
		// { ... } /*{{note: ...}}*/); its length is driven by the code, so the
		// line is not length-linted here. Multi-line fragment bodies (pure doc)
		// are linted in noteFragLine.
		s.fragStart = ln.n
		s.frag = []string{rest}
		if closed {
			s.finishFrag()
		} else {
			s.state = stNoteFrag
		}
	}
	// any other marker inside a note region is not note content: ignore.
}

// noteFragLine accumulates a multi-line /*{{note: ...}}*/ fragment until }}*/.
func (s *scanner) noteFragLine(ln srcLine) {
	t := ln.text
	if before, ok := strings.CutSuffix(strings.TrimRight(t, " \t"), markerClose); ok {
		s.frag = append(s.frag, before)
		s.finishFrag()
		return
	}
	s.lintDocLine(ln, 120)
	s.frag = append(s.frag, t)
}

// dedent removes the indentation the lines of a multi-line fragment share -
// the alignment under the marker, which is layout, not content - while keeping
// every line's indentation relative to it (a code block inside a note keeps
// its shape). Blank lines and lines of only blanks do not take part.
func dedent(lines []string) []string {
	common := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := 0
		for n < len(l) && (l[n] == ' ' || l[n] == '\t') {
			n++
		}
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			out[i] = ""
			continue
		}
		out[i] = l[common:]
	}
	return out
}

// finishFrag turns the accumulated fragment into one body paragraph and pulls
// its include-note-id fields onto the note's dependency list. The leading text
// (before any '|') is the body; the '|' fields are parsed for include-note-id.
func (s *scanner) finishFrag() {
	full := strings.Join(dedent(s.frag), "\n")
	s.frag = nil
	segs := splitFields(full)
	if body := strings.TrimSpace(segs[0]); body != "" {
		s.noteBody = append(s.noteBody, body)
	}
	for _, seg := range segs[1:] {
		name, value, ok := strings.Cut(seg, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "include-note-id") {
			if id := strings.TrimSpace(value); id != "" {
				s.note.Includes = append(s.note.Includes, id)
			}
		}
	}
	s.state = stNoteCollect
}

func (s *scanner) registerNote(n int) {
	s.note.ID = strings.ToLower(s.note.ID) // doc-internal id: normalized to lowercase
	if s.note.ID != "" && s.f.NoteByID(s.note.ID) != nil {
		s.diag(Error, s.note.StartLine, "duplicate note-id %q", s.note.ID)
	}
	if len(s.noteBody) == 0 && len(s.note.Includes) == 0 {
		s.diag(Warning, s.note.StartLine, "shared note %q is empty (no note: fragments before end-note)", s.note.ID)
	}
	s.note.Body = strings.Join(s.noteBody, "\n")
	s.note.EndLine = n
	s.note.Raw = s.takeRaw()
	s.f.Notes = append(s.f.Notes, *s.note)
	s.note = nil
	s.noteBody = nil
	s.frag = nil
	s.state = stIdle
}

// eof reports whatever is still open when the file ends.
func (s *scanner) eof() {
	switch s.state {
	case stFence:
		s.diag(Error, s.fenceStart, "code fence not closed at end of file")
		s.state = stHeader
		s.finishHeader(s.lastLine)
	case stHeader:
		s.diag(Error, s.hdrStart, "header marker not closed with }}*/")
		s.finishHeader(s.lastLine)
	case stNoteMarker, stNoteCollect, stNoteFrag:
		if s.state == stNoteFrag {
			s.finishFrag()
		}
		s.diag(Error, s.note.StartLine, "shared note not closed with /*{{end-note}}*/")
		s.registerNote(s.lastLine)
	case stRaw:
		s.diag(Error, s.rawStart, "markdown-free block not closed with /*{{end-markdown-free}}*/")
		s.closeRaw(s.lastLine)
	case stTopic:
		s.diag(Error, s.scope.StartLine, "topic not closed with /*{{end-topic}}*/")
		s.closeTopic(s.lastLine)
	}
	if s.scope != nil {
		s.diag(Error, s.scope.StartLine, "begin-%s without matching end-%s", s.scope.Kind, s.scope.Kind)
		s.scope.EndLine = s.lastLine
		s.scope.Raw = s.takeRaw()
		s.f.Entities = append(s.f.Entities, *s.scope)
		s.scope = nil
	}
	if s.inComment {
		s.diag(Error, s.lastLine, "comment still open at end of file")
	}
}

// checkDuplicates enforces the in-file half of the identity rules: no two
// scopes may share an identity key (per kind; Xbase++ names compare
// case-insensitively, C/C++ names case-sensitively, blanks collapse) and no
// two scopes may share a mangled-name (the decorated symbol is unique per
// overload by construction).
func (s *scanner) checkDuplicates() {
	type key struct {
		k  Kind
		id string
	}
	ids := make(map[key]int)
	mangled := make(map[string]int)
	for i := range s.f.Entities {
		e := &s.f.Entities[i]
		if e.Ident != "" {
			k := key{e.Kind, identKey(e.Kind, e.Ident)}
			if first, dup := ids[k]; dup {
				s.diag(Error, e.StartLine, "duplicate identity %q (first at line %d)", e.Ident, first)
			} else {
				ids[k] = e.StartLine
			}
		}
		for _, fd := range e.FieldAll("mangled-name") {
			if fd.Value == "" {
				continue
			}
			if first, dup := mangled[fd.Value]; dup {
				s.diag(Error, fd.Line, "duplicate mangled-name %q (first at line %d)", fd.Value, first)
			} else {
				mangled[fd.Value] = fd.Line
			}
		}
	}
}

// identKey normalizes an identity for comparison: blanks collapse away,
// and Xbase++ names (function, class) fold case while C/C++ names keep it.
func identKey(k Kind, id string) string {
	var sb strings.Builder
	for i := 0; i < len(id); i++ {
		if id[i] != ' ' && id[i] != '\t' {
			sb.WriteByte(id[i])
		}
	}
	out := sb.String()
	// Case per world (Draft 2): Xbase++ identities are case-insensitive; C/C++
	// ones (c-function, cpp-function, cpp-class, and debug-c-function which is a
	// c-function) are case-sensitive - C symbols cannot be folded.
	switch k {
	case KindFunction, KindClass, KindStructure, KindInternalFunction,
		KindCommand, KindTopic:
		out = strings.ToLower(out)
	}
	return out
}
