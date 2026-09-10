package scan

import (
	"strings"
	"testing"
)

const sampleClass = "/*{{begin-class}}*/\r\n" +
	"/*{{class-name_: WAPIST_NMDATETIMEFORMATQUERY\r\n" +
	"            | _slug_: wapist_nmdatetimeformatquery\r\n" +
	"            | class-function: WAPIST_NMDATETIMEFORMATQUERY\r\n" +
	"            | parent: {{ilink: <class gwst> gwst}}\r\n" +
	"            | category: winapi/structures , commctrl\r\n" +
	"            | desc: Wrapper over the WinApi NMDATETIMEFORMATQUERY structure. Defined as NMDATETIMEFORMATQUERY in\r\n" +
	"              ot4xb_wapist_map.ch.\r\n" +
	"   }}*/\r\n" +
	"/*{{|:**BEGIN STRUCTURE  NMDATETIMEFORMATQUERY** }}*/\r\n" +
	"XB_BEGIN_STRUCTURE( NMDATETIMEFORMATQUERY )\r\n" +
	"   /*{{|member_: - MEMBER @ {{ilink: <slug wapist_nmhdr> wapist_NMHDR}} nmhdr |desc_: NHDR structure }}*/\r\n" +
	"   _XBST_NMHDR ( nmhdr  )\r\n" +
	"   pc->Var(\"x\"); /*{{|ivar_: - VAR x | type: Numeric | desc_: trailing marker }}*/\r\n" +
	"XB_END_STRUCTURE\r\n" +
	"/*{{|:**END STRUCTURE** }}*/\r\n" +
	"/*{{include-note-id: wapist-map}}*/\r\n" +
	"/*{{end-class}}*/\r\n"

func TestClassScope(t *testing.T) {
	f := Scan("a.cpp", []byte(sampleClass))
	if f.Errors() != 0 {
		t.Fatalf("issues: %+v", f.Issues)
	}
	if len(f.Topics) != 1 {
		t.Fatalf("topics: %d", len(f.Topics))
	}
	tp := f.Topics[0]
	if tp.Kind != KindClass || tp.Ident != "WAPIST_NMDATETIMEFORMATQUERY" || tp.Key != "WAPIST_NMDATETIMEFORMATQUERY" || tp.Compact {
		t.Fatalf("topic: %+v", tp)
	}
	if len(tp.Markers) != 6 {
		t.Fatalf("markers: %d", len(tp.Markers))
	}
	h := tp.Markers[0]
	if h.Fields[0].Label != "class-name" || !h.Fields[0].HideLabel || h.Fields[0].HideEntry {
		t.Fatalf("identity field: %+v", h.Fields[0])
	}
	if s := tp.Field("slug"); s == nil || s.Value != "wapist_nmdatetimeformatquery" || !s.HideEntry || !s.HideLabel {
		t.Fatalf("slug: %+v", s)
	}
	if p := tp.Field("parent"); p == nil || p.Value != "{{ilink: <class gwst> gwst}}" {
		t.Fatalf("parent: %+v", p)
	}
	if c := Categories(tp.Field("category").Value); len(c) != 2 || c[1] != "commctrl" {
		t.Fatalf("categories: %v", c)
	}
	d := tp.Field("desc")
	if d == nil || !strings.HasSuffix(Dedent(d.Value), "\not4xb_wapist_map.ch.") || d.HideLabel {
		t.Fatalf("desc: %+v", d)
	}
	if m := tp.Markers[1]; m.Kind != MkFragment || m.Fields[0].Label != "" || m.Fields[0].Value != "**BEGIN STRUCTURE  NMDATETIMEFORMATQUERY**" {
		t.Fatalf("text fragment: %+v", m)
	}
	m := tp.Markers[2]
	if m.Kind != MkFragment || len(m.Fields) != 2 || m.Fields[0].Label != "member" || !m.Fields[0].HideLabel ||
		m.Fields[0].Value != "- MEMBER @ {{ilink: <slug wapist_nmhdr> wapist_NMHDR}} nmhdr" || m.Fields[1].Label != "desc" {
		t.Fatalf("member fragment: %+v", m.Fields)
	}
	if m := tp.Markers[3]; !m.Trailing || m.Fields[0].Value != "- VAR x" || m.Fields[1].Label != "type" {
		t.Fatalf("trailing fragment: %+v", m)
	}
	if m := tp.Markers[5]; m.Kind != MkInclude || m.Fields[0].Value != "wapist-map" {
		t.Fatalf("include: %+v", m)
	}
}

func TestFunctionAndFragmentFields(t *testing.T) {
	src := "/*{{begin-function}}*/\n" +
		"/*{{function_: ft64_SetTs\n" +
		"            | syntax_: `ft64_SetTs( pft | x )`\n" +
		"            | category: date-time/filetime\n" +
		"   }}*/\n" +
		"/*{{|desc: Stores a timestamp string.\n" +
		"    | params:\n" +
		"    - `pft` FILETIME64 extended pointer - Destination.\n" +
		"    - `cTimeStamp` Character - see `a | b: c` literal.\n" +
		"\n" +
		"    Returns NIL.\n" +
		"\n" +
		"    |note: The pft parameter uses OT4XB extended pointer handling.\n" +
		"    |seealso: See also: {{ilink: <class FILETIME64> FILETIME64}} }}*/\n" +
		"code();\n" +
		"/*{{end-function}}*/\n"
	f := Scan("b.cpp", []byte(src))
	if f.Errors() != 0 {
		t.Fatalf("issues: %+v", f.Issues)
	}
	if f.NonCRLF == 0 {
		t.Fatalf("LF input must be reported")
	}
	tp := f.Topics[0]
	if tp.Kind != KindFunction || tp.Key != "FT64_SETTS" {
		t.Fatalf("topic: %+v", tp)
	}
	if s := tp.Field("syntax"); s == nil || s.Value != "`ft64_SetTs( pft | x )`" {
		t.Fatalf("syntax with a | inside backticks: %+v", s)
	}
	fr := tp.Markers[1]
	labels := []string{}
	for _, fd := range fr.Fields {
		labels = append(labels, fd.Label)
	}
	if strings.Join(labels, ",") != "desc,params,note,seealso" {
		t.Fatalf("fragment labels: %v", labels)
	}
	if !strings.Contains(fr.Fields[1].Value, "Returns NIL.") || !strings.Contains(fr.Fields[1].Value, "`a | b: c`") {
		t.Fatalf("params value: %q", fr.Fields[1].Value)
	}
	if fr.Fields[2].Line != 13 {
		t.Fatalf("note line: %d", fr.Fields[2].Line)
	}
}

func TestCompactAndErrors(t *testing.T) {
	src := "/*{{ topic: misc | category: commands , ot4xb.ch | command_: SWAP a , b |desc_: Swap }}*/\r\n" +
		"/*{{ topic: misc | command_: DEFAULT var := val |desc_: Set default }}*/\r\n" +
		"/*{{|command_: loose }}*/\r\n" +
		"/*{{include-note-id: x}}*/\r\n" +
		"/*{{begin-topic}}*/\r\n" +
		"/*{{|: text before header }}*/\r\n" +
		"/*{{topic_: t2}}*/\r\n" +
		"/*{{end-function}}*/\r\n" +
		"/*{{begin-note}}*/\r\n" +
		"/*{{note-id: n1 | title_: T }}*/\r\n" +
		"/*{{|: body }}*/\r\n"
	f := Scan("c.cpp", []byte(src))
	if len(f.Topics) != 4 {
		t.Fatalf("topics: %d (%+v)", len(f.Topics), f.Issues)
	}
	if !f.Topics[0].Compact || f.Topics[0].Key != "misc" || len(f.Topics[0].Markers[0].Fields) != 4 {
		t.Fatalf("compact topic: %+v", f.Topics[0].Markers[0].Fields)
	}
	if f.Topics[1].Key != "misc" {
		t.Fatalf("scattered compact topic: %+v", f.Topics[1])
	}
	codes := map[string]int{}
	for _, is := range f.Issues {
		codes[is.Code]++
	}
	for _, c := range []string{"content-outside-topic", "content-before-header", "scope-mismatch", "unknown-kind"} {
		if codes[c] == 0 {
			t.Errorf("missing issue %s: %+v", c, f.Issues)
		}
	}
	if codes["content-outside-topic"] != 3 {
		t.Errorf("loose fragment + loose include: %+v", f.Issues)
	}
}

func TestCanonLabel(t *testing.T) {
	cases := map[string][3]any{"desc": {"desc", false, false}, "desc_": {"desc", false, true},
		"_slug_": {"slug", true, true}, "_tg_": {"tg", true, true}, "_x": {"x", true, false}, "a_b": {"a_b", false, false}}
	for in, want := range cases {
		l, he, hl := canonLabel(in)
		if l != want[0] || he != want[1] || hl != want[2] {
			t.Errorf("%s -> %s %v %v", in, l, he, hl)
		}
	}
}

func TestMdInline(t *testing.T) {
	src := "/*{{begin-topic}}*/\r\n" +
		"/*{{topic_: t }}*/\r\n" +
		"/*{{|desc: Flags {{begin-md}}\r\n" +
		"| flag | meaning |\r\n" +
		"|---|---|\r\n" +
		"| {{ilink: <class X> X}} | copy | not: a field |\r\n" +
		"{{end-md}} | note: after }}*/\r\n" +
		"/*{{end-topic}}*/\r\n"
	f := Scan("d.cpp", []byte(src))
	if f.Errors() != 0 || len(f.Topics) != 1 {
		t.Fatalf("topics: %+v issues %+v", f.Topics, f.Issues)
	}
	m := f.Topics[0].Markers[1]
	if len(m.Fields) != 2 || m.Fields[0].Label != "desc" || m.Fields[1].Label != "note" || m.Fields[1].Value != "after" {
		t.Fatalf("fields: %+v", m.Fields)
	}
	if !strings.Contains(m.Fields[0].Value, "| not: a field |") || !strings.HasSuffix(m.Fields[0].Value, "{{end-md}}") {
		t.Fatalf("desc value %q", m.Fields[0].Value)
	}
}

// TestCodeCapture: begin-code ... end-code inside a composed topic turns the
// source lines between them into one fragment - a hidden-label "code" field
// holding them as a fence, in position - and the language is optional. A "|"
// in that code is code, never a field separator.
func TestCodeCapture(t *testing.T) {
	src := "/*{{begin-topic}}*/\r\n" +
		"/*{{topic: demo | desc: shows code }}*/\r\n" +
		"/*{{begin-code: xbase}}*/\r\n" +
		"proc main\r\n" +
		"   ? {|x| x}   // a | is code here\r\n" +
		"return\r\n" +
		"/*{{end-code}}*/\r\n" +
		"/*{{|note: after}}*/\r\n" +
		"/*{{end-topic}}*/\r\n"
	f := Scan("a.prg", []byte(src))
	if f.Errors() != 0 {
		t.Fatalf("issues: %+v", f.Issues)
	}
	tp := f.Topics[0]
	if len(tp.Markers) != 3 {
		t.Fatalf("markers: %d", len(tp.Markers))
	}
	c := tp.Markers[1]
	if c.Kind != MkFragment || c.Line != 3 || c.EndLine != 7 || len(c.Fields) != 1 {
		t.Fatalf("code fragment: %+v", c)
	}
	fd := c.Fields[0]
	want := "```xbase\nproc main\n   ? {|x| x}   // a | is code here\nreturn\n```"
	if fd.Label != "code" || !fd.HideLabel || fd.HideEntry || fd.Line != 3 || fd.Value != want {
		t.Fatalf("code field: %+v", fd)
	}
	if n := tp.Markers[2]; n.Kind != MkFragment || n.Fields[0].Label != "note" {
		t.Fatalf("the note after the code: %+v", n)
	}
	// no language: a bare fence; an empty capture is a bare fence with nothing in it
	src = "/*{{begin-topic}}*/\r\n/*{{topic: d}}*/\r\n/*{{begin-code}}*/\r\nx\r\n/*{{end-code}}*/\r\n" +
		"/*{{begin-code}}*/\r\n/*{{end-code}}*/\r\n/*{{end-topic}}*/\r\n"
	f = Scan("b.prg", []byte(src))
	if f.Errors() != 0 {
		t.Fatalf("issues: %+v", f.Issues)
	}
	if v := f.Topics[0].Markers[1].Fields[0].Value; v != "```\nx\n```" {
		t.Fatalf("bare fence: %q", v)
	}
	if v := f.Topics[0].Markers[2].Fields[0].Value; v != "```\n\n```" {
		t.Fatalf("empty capture: %q", v)
	}
}

// TestCodeCaptureIssues: what a capture refuses, each with its code.
func TestCodeCaptureIssues(t *testing.T) {
	cases := []struct{ src, code string }{
		// outside any topic at all
		{"/*{{begin-code}}*/\r\nx\r\n/*{{end-code}}*/\r\n", "content-outside-topic"},
		// a compact topic cannot hold one
		{"/*{{topic: d}}*/\r\n/*{{begin-code}}*/\r\nx\r\n/*{{end-code}}*/\r\n", "content-outside-topic"},
		// before the header of its scope
		{"/*{{begin-topic}}*/\r\n/*{{begin-code}}*/\r\nx\r\n/*{{end-code}}*/\r\n/*{{topic: d}}*/\r\n/*{{end-topic}}*/\r\n", "content-before-header"},
		// end-code with nothing open
		{"/*{{begin-topic}}*/\r\n/*{{topic: d}}*/\r\n/*{{end-code}}*/\r\n/*{{end-topic}}*/\r\n", "stray-end"},
		// a second end-code
		{"/*{{begin-topic}}*/\r\n/*{{topic: d}}*/\r\n/*{{begin-code}}*/\r\nx\r\n/*{{end-code}}*/\r\n/*{{end-code}}*/\r\n/*{{end-topic}}*/\r\n", "stray-end"},
		// a marker inside the capture
		{"/*{{begin-topic}}*/\r\n/*{{topic: d}}*/\r\n/*{{begin-code}}*/\r\nx\r\n/*{{|note: n}}*/\r\n/*{{end-code}}*/\r\n/*{{end-topic}}*/\r\n", "code-open"},
		// the pair does not nest
		{"/*{{begin-topic}}*/\r\n/*{{topic: d}}*/\r\n/*{{begin-code}}*/\r\n/*{{begin-code}}*/\r\nx\r\n/*{{end-code}}*/\r\n/*{{end-topic}}*/\r\n", "code-open"},
		// never closed
		{"/*{{begin-topic}}*/\r\n/*{{topic: d}}*/\r\n/*{{begin-code}}*/\r\nx\r\n", "unclosed-code"},
	}
	for _, c := range cases {
		f := Scan("a.prg", []byte(c.src))
		found := false
		for _, is := range f.Issues {
			if is.Code == c.code {
				found = true
			}
		}
		if !found {
			t.Errorf("%q:\nwant issue %s, got %+v", c.src, c.code, f.Issues)
		}
	}
}
