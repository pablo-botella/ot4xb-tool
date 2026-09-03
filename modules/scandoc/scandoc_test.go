package scandoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// src builds CRLF file content from lines.
func src(lines ...string) []byte {
	return []byte(strings.Join(lines, "\r\n") + "\r\n")
}

// scan writes data to a temp file and scans it.
func scan(t *testing.T, name string, data []byte) *File {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0666); err != nil {
		t.Fatal(err)
	}
	f, err := Scan(p)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func count(f *File, sev Severity) int {
	n := 0
	for _, d := range f.Diags {
		if d.Severity == sev {
			n++
		}
	}
	return n
}

func hasDiag(f *File, substr string) bool {
	for _, d := range f.Diags {
		if strings.Contains(d.Msg, substr) {
			return true
		}
	}
	return false
}

// goodSrc is a trimmed, new-form blend of the real fixtures (fpCall.cpp /
// DrTool.cpp / FileTime.cpp shapes): all four scope kinds, a shared note
// with its loose include, multi-field lines, spans, escapes, backtick and
// tilde fences.
func goodSrc() []byte {
	return src(
		"//----",                                  // 1
		"/*{{begin-note | note-id: shared-one",    // 2
		"             | title: A Shared Note}}*/", // 3
		"/*{{note:",                               // 4
		"Body line one.",                          // 5
		"- bullet two",                            // 6
		"}}*/",                                    // 7
		"/*{{end-note}}*/",                        // 8
		"//----",                                  // 9
		"/*{{begin-function}}*/",                  // 10
		"/*{{function: nDemo",                     // 11
		"            | syntax: `nDemo( nA [, nB] ) -> nResult | nDemo( NIL ) -> nOther`",               // 12
		"            | category: demo/misc",                                                            // 13
		"            | desc: First line of the description",                                            // 14
		"              continued on a second line.",                                                    // 15
		"            | param nA: Numeric - The first parameter. | param nB: Numeric - The second one.", // 16
		"            | flag 0x01: Uses `a | b` bitwise or.",                                            // 17
		"            | note: A pipe escaped \\| stays literal.",                                        // 18
		"            | introduced: 1.8.0.1 - the nB parameter.",                                        // 19
		"            | example: ```",                                                                   // 20
		"   QOut( nDemo( 1 ) )",                                                                        // 21
		"```",                                                                                          // 22
		"            | see-also: nOther",                                                               // 23
		"   }}*/",                                                                                      // 24
		"XPPRET XPPENTRY NDEMO( XppParamList pl )",                                                     // 25
		"{",                                   // 26
		"   _retnl( pl, 0 );",                 // 27
		"}",                                   // 28
		"/*{{include-note-id: shared-one}}*/", // 29
		"/*{{end-function}}*/",                // 30
		"/*{{begin-c-function}}*/",            // 31
		"/*{{c-function: demo_c",              // 32
		"            | syntax: `DWORD demo_c( void )`",              // 33
		"            | header: ot4xb_c_exported.h",                  // 34
		"            | mangled-name: demo_c",                        // 35
		"            | return: DWORD - Zero.",                       // 36
		"   }}*/",                                                   // 37
		"extern \"C\" OT4XB_API DWORD demo_c( void ) { return 0; }", // 38
		"/*{{end-c-function}}*/",                                    // 39
		"/*{{begin-cpp-function}}*/",                                // 40
		"/*{{cpp-function: demo_cpp(LONG)",                          // 41
		"            | mangled-name: ?demo_cpp@@YAJJ@Z",             // 42
		"            | example: ~~~prg",                             // 43
		"? demo_cpp( 5 )",                                           // 44
		"~~~",                                                       // 45
		"   }}*/",                                                   // 46
		"OT4XB_API LONG demo_cpp( LONG v ) { return v; }",           // 47
		"/*{{end-cpp-function}}*/",                                  // 48
		"/*{{begin-class}}*/",                                       // 49
		"/*{{class-name: DEMO_CLASS",                                // 50
		"            | category: demo/misc | desc: A class.",        // 51
		"   }}*/", // 52
		"XPPRET XPPENTRY DEMO_CLASS( XppParamList pl )", // 53
		"{", // 54
		"   pc->Var(\"x\"); /*{{ivar: x | type: Numeric | desc: ignored item marker.}}*/", // 55
		"}",                 // 56
		"/*{{end-class}}*/", // 57
	)
}

func TestScanGood(t *testing.T) {
	f := scan(t, "good.cpp", goodSrc())
	if len(f.Diags) != 0 {
		t.Fatalf("diags = %v", f.Diags)
	}
	if len(f.Entities) != 4 || len(f.Notes) != 1 {
		t.Fatalf("entities=%d notes=%d", len(f.Entities), len(f.Notes))
	}

	e := &f.Entities[0]
	if e.Kind != KindFunction || e.Ident != "nDemo" || e.StartLine != 10 || e.EndLine != 30 {
		t.Fatalf("function entity = %+v", e)
	}
	if v, _ := e.Field("syntax"); v != "`nDemo( nA [, nB] ) -> nResult | nDemo( NIL ) -> nOther`" {
		t.Fatalf("syntax = %q", v)
	}
	if v, _ := e.Field("desc"); v != "First line of the description continued on a second line." {
		t.Fatalf("desc = %q", v)
	}
	params := e.FieldAll("param nA")
	if len(params) != 1 || params[0].Value != "Numeric - The first parameter." {
		t.Fatalf("param nA = %+v", params)
	}
	if _, ok := e.Field("param nB"); !ok {
		t.Fatal("param nB missing (multi-field line)")
	}
	if v, _ := e.Field("flag 0x01"); v != "Uses `a | b` bitwise or." {
		t.Fatalf("flag = %q", v)
	}
	if v, _ := e.Field("note"); v != "A pipe escaped \\| stays literal." {
		t.Fatalf("note = %q", v)
	}
	if v, _ := e.Field("example"); v != "```\n   QOut( nDemo( 1 ) )\n```" {
		t.Fatalf("example = %q", v)
	}
	if v, _ := e.Field("introduced"); v != "1.8.0.1 - the nB parameter." {
		t.Fatalf("introduced = %q", v)
	}
	if len(e.NoteRefs) != 1 || e.NoteRefs[0] != "shared-one" {
		t.Fatalf("noteRefs = %v", e.NoteRefs)
	}
	if len(e.Fields) != 10 {
		t.Fatalf("field count = %d: %+v", len(e.Fields), e.Fields)
	}
	for _, fd := range e.Fields {
		if fd.Unknown {
			t.Fatalf("field %q flagged unknown", fd.Name)
		}
	}

	if e := &f.Entities[1]; e.Kind != KindCFunction || e.Ident != "demo_c" {
		t.Fatalf("c-function entity = %+v", e)
	}
	e2 := &f.Entities[2]
	if e2.Kind != KindCppFunction || e2.Ident != "demo_cpp(LONG)" {
		t.Fatalf("cpp entity = %+v", e2)
	}
	if v, _ := e2.Field("example"); v != "~~~prg\n? demo_cpp( 5 )\n~~~" {
		t.Fatalf("tilde example = %q", v)
	}
	e3 := &f.Entities[3]
	if e3.Kind != KindClass || e3.Ident != "DEMO_CLASS" || len(e3.Fields) != 2 {
		t.Fatalf("class entity = %+v", e3)
	}

	n := &f.Notes[0]
	if n.ID != "shared-one" || n.Title != "A Shared Note" || n.Body != "Body line one.\n- bullet two" ||
		n.StartLine != 2 || n.EndLine != 8 {
		t.Fatalf("note = %+v", n)
	}

	f.Resolve()
	if len(f.Diags) != 0 {
		t.Fatalf("resolve diags = %v", f.Diags)
	}
	if got := f.ExpandNotes("x {{include-note-id: shared-one}} y"); got != "x Body line one.\n- bullet two y" {
		t.Fatalf("expand = %q", got)
	}
}

func TestOldFormTolerated(t *testing.T) {
	f := scan(t, "old.cpp", src(
		"/*{{begin-function}}*/",
		"/*{{function: cDemo( cX ) -> cY",
		"            | desc: Old form.",
		"            | note: {{include-note-id: shared-x}}",
		"   }}*/",
		"XPPRET XPPENTRY CDEMO( XppParamList pl ) {}",
		"/*{{end-function}}*/",
		"/*{{begin-note | note-id: shared-x}}*/",
		"/*{{note:",
		"body",
		"}}*/",
		"/*{{end-note}}*/",
	))
	if count(f, Error) != 0 {
		t.Fatalf("diags = %v", f.Diags)
	}
	if !hasDiag(f, "old-form header value") || !hasDiag(f, "retired form") {
		t.Fatalf("missing migration warnings: %v", f.Diags)
	}
	e := &f.Entities[0]
	if e.Ident != "cDemo( cX ) -> cY" || len(e.NoteRefs) != 1 || e.NoteRefs[0] != "shared-x" {
		t.Fatalf("entity = %+v", e)
	}
	f.Resolve()
	if count(f, Error) != 0 {
		t.Fatalf("resolve diags = %v", f.Diags)
	}
}

func TestComponents(t *testing.T) {
	f := scan(t, "c.cpp", src(
		"/*{{begin-structure}}*/",
		"/*{{structure: WIDGET | category: ui}}*/",
		"XPPRET XPPENTRY WIDGET( XppParamList pl )",
		"{",
		`   pc->Var("cName"); /*{{ivar: cName | type: Character | desc: the name}}*/`,
		"   /*{{method: Draw( [lForce] )",
		"            | return: Self",
		"            | desc: draws it.}}*/",
		`   pc->MethodCB("Draw", d);`,
		"   /*{{class-method: New()}}*/",
		`   pc->ClassMethod("New", nw);`,
		"}",
		"/*{{end-structure}}*/",
	))
	if len(f.Diags) != 0 {
		t.Fatalf("diags = %v", f.Diags)
	}
	if len(f.Entities) != 1 {
		t.Fatalf("entities = %d", len(f.Entities))
	}
	e := &f.Entities[0]
	if e.Kind != KindStructure || e.Ident != "WIDGET" {
		t.Fatalf("entity = %+v", e)
	}
	if len(e.Components) != 3 {
		t.Fatalf("components = %d: %+v", len(e.Components), e.Components)
	}
	if c := e.Components[0]; c.Kind != "ivar" || c.Ident != "cName" || c.Name != "cName" {
		t.Fatalf("ivar = %+v", c)
	} else if v, _ := c.Field("type"); v != "Character" {
		t.Fatalf("ivar type = %q", v)
	}
	// the method keeps its authored signature as Ident, base name as Name
	if c := e.Components[1]; c.Kind != "method" || c.Ident != "Draw( [lForce] )" || c.Name != "Draw" {
		t.Fatalf("method = %+v", c)
	} else if v, _ := c.Field("return"); v != "Self" {
		t.Fatalf("method return = %q", v)
	}
	if c := e.Components[2]; c.Kind != "class-method" || c.Ident != "New()" || c.Name != "New" {
		t.Fatalf("class-method = %+v", c)
	}
	// gwst-member: the "type: X pos: Y size: Z" tail is descriptive text, so the
	// identity anchor (Name) is just the first token.
	gw := scan(t, "gw.cpp", src(
		"/*{{begin-structure}}*/",
		"/*{{structure: LARGE_INTEGER}}*/",
		"XPPRET XPPENTRY LARGE_INTEGER( XppParamList pl ) {",
		"   /*{{gwst-member: u type: _LARGE_INTEGER_ pos: 0 size: 8}}*/",
		"}",
		"/*{{end-structure}}*/",
	))
	if len(gw.Diags) != 0 || len(gw.Entities) != 1 {
		t.Fatalf("gwst: diags=%v entities=%d", gw.Diags, len(gw.Entities))
	}
	if c := gw.Entities[0].Components[0]; c.Kind != "gwst-member" || c.Name != "u" ||
		c.Ident != "u type: _LARGE_INTEGER_ pos: 0 size: 8" {
		t.Fatalf("gwst-member = %+v", c)
	}
	// An item marker outside any scope is not a component (skipped).
	g := scan(t, "loose.cpp", src("/*{{ivar: orphan | type: Numeric}}*/"))
	if len(g.Entities) != 0 || len(g.Diags) != 0 {
		t.Fatalf("loose item: entities=%d diags=%v", len(g.Entities), g.Diags)
	}
}

func TestMarkdownFree(t *testing.T) {
	f := scan(t, "m.cpp", src(
		"/*{{begin-markdown-free}}*/",
		"| col a | col b |",
		"|-------|-------|",
		"see <http://x> and a /*{{fake: marker}}*/ that is NOT parsed",
		"/*{{end-markdown-free}}*/",
	))
	if len(f.Diags) != 0 {
		t.Fatalf("diags = %v", f.Diags)
	}
	if len(f.Entities) != 1 || f.Entities[0].Kind != KindMarkdownFree {
		t.Fatalf("entities = %+v", f.Entities)
	}
	body, _ := f.Entities[0].Field("body")
	if !strings.Contains(body, "| col a | col b |") || !strings.Contains(body, "{{fake: marker}}") {
		t.Fatalf("raw body = %q", body)
	}
	// embedded in an entity scope: a verbatim "markdown-free" field of that
	// entity (no topic of its own), and the entity's raw blob still holds it
	emb := scan(t, "e.cpp", src(
		"/*{{begin-function}}*/",
		"/*{{function: Foo | desc: d}}*/",
		"/*{{begin-markdown-free}}*/",
		"| a | b |",
		"/*{{end-markdown-free}}*/",
		"int Foo(){}",
		"/*{{end-function}}*/",
	))
	if len(emb.Diags) != 0 || len(emb.Entities) != 1 || emb.Entities[0].Kind != KindFunction {
		t.Fatalf("embedded: entities=%+v diags=%v", emb.Entities, emb.Diags)
	}
	if v, ok := emb.Entities[0].Field("markdown-free"); !ok || v != "| a | b |" {
		t.Fatalf("embedded field = %q ok=%v", v, ok)
	}
	if !strings.Contains(strings.Join(emb.Entities[0].Raw, "\n"), "| a | b |") {
		t.Fatalf("embedded block missing from the entity raw: %q", emb.Entities[0].Raw)
	}
	// embedded in a composed note: one more fragment of its body
	nt := scan(t, "n.cpp", src(
		"/*{{begin-note | note-id: raw}}*/",
		"/*{{note: first}}*/",
		"/*{{begin-markdown-free}}*/",
		"| t | u |",
		"/*{{end-markdown-free}}*/",
		"/*{{end-note}}*/",
	))
	if n := nt.NoteByID("raw"); n == nil || !strings.Contains(n.Body, "first") || !strings.Contains(n.Body, "| t | u |") {
		t.Fatalf("note-embedded block: %+v", nt.Notes)
	}
	// unclosed markdown-free is an error, the partial body still captured
	g := scan(t, "u.cpp", src("/*{{begin-markdown-free}}*/", "| x |"))
	if count(g, Error) != 1 || !hasDiag(g, "markdown-free block not closed") {
		t.Fatalf("unclosed diags = %v", g.Diags)
	}
}

func TestTopic(t *testing.T) {
	f := scan(t, "t.cpp", src(
		"/*{{begin-topic}}*/",
		"/*{{topic: Gwst-Commands | title: The GWST commands}}*/",
		"Intro prose for the topic, with a | pipe that is just content.",
		"/*{{command: BEGIN-DYNAMIC-CLASS | desc: opens a dynamic class}}*/",
		"More prose documenting the command, still plain content.",
		"/*{{end-topic}}*/",
	))
	if len(f.Diags) != 0 {
		t.Fatalf("diags = %v", f.Diags)
	}
	var topic, cmd *Entity
	for i := range f.Entities {
		switch f.Entities[i].Kind {
		case KindTopic:
			topic = &f.Entities[i]
		case KindCommand:
			cmd = &f.Entities[i]
		}
	}
	if topic == nil || cmd == nil {
		t.Fatalf("entities = %+v", f.Entities)
	}
	if topic.Ident != "gwst-commands" { // doc-internal id normalized to lowercase
		t.Fatalf("topic id = %q", topic.Ident)
	}
	body, _ := topic.Field("body")
	if !strings.Contains(body, "Intro prose") || !strings.Contains(body, "still plain content") {
		t.Fatalf("topic body = %q", body)
	}
	if strings.Contains(body, "command:") { // the command marker is an anchor, not prose
		t.Fatalf("command marker leaked into body: %q", body)
	}
	if cmd.Ident != "BEGIN-DYNAMIC-CLASS" {
		t.Fatalf("command id = %q", cmd.Ident)
	}
	if v, _ := cmd.Field("topic"); v != "gwst-commands" {
		t.Fatalf("command topic = %q", v)
	}
	if v, _ := cmd.Field("desc"); v != "opens a dynamic class" {
		t.Fatalf("command desc = %q", v)
	}
	// an unclosed topic is an error, its partial body still captured
	g := scan(t, "u.cpp", src("/*{{begin-topic}}*/", "/*{{topic: x}}*/", "body"))
	if !hasDiag(g, "topic not closed") {
		t.Fatalf("unclosed topic diags = %v", g.Diags)
	}
}

func TestILink(t *testing.T) {
	// ParseILink structure
	k, id, text, ok := ParseILink("<function Set_FpCall_Flags> turn flags on")
	if !ok || k != "function" || id != "Set_FpCall_Flags" || text != "turn flags on" {
		t.Fatalf("parse full = %q/%q/%q ok=%v", k, id, text, ok)
	}
	k, id, text, ok = ParseILink("  <method LARGE_INTEGER:New64>  ") // no display text -> id is the text
	if !ok || k != "method" || id != "LARGE_INTEGER:New64" || text != "LARGE_INTEGER:New64" {
		t.Fatalf("parse no-text = %q/%q/%q ok=%v", k, id, text, ok)
	}
	if _, _, _, ok := ParseILink("plain text, not a link"); ok {
		t.Fatalf("expected non-link to be ok=false")
	}
	// field-level lint: good ilink, no diag; malformed and unknown-kind report
	good := scan(t, "g.cpp", src("/*{{function: Foo | ilink: <class patata> the patata class}}*/"))
	if count(good, Error) != 0 {
		t.Fatalf("good ilink diags = %v", good.Diags)
	}
	bad := scan(t, "b.cpp", src("/*{{function: Foo | ilink: nope}}*/"))
	if !hasDiag(bad, "ilink value must start with <kind id>") {
		t.Fatalf("malformed ilink diags = %v", bad.Diags)
	}
	unk := scan(t, "u.cpp", src("/*{{function: Foo | ilink: <bogus Foo> x}}*/"))
	if !hasDiag(unk, "unknown kind") {
		t.Fatalf("unknown-kind ilink diags = %v", unk.Diags)
	}
}

func TestResolveProject(t *testing.T) {
	// file A defines a function and a structure with a method.
	a := scan(t, "a.cpp", src(
		"/*{{function: Foo}}*/",
		"/*{{begin-structure}}*/",
		"/*{{structure: WIDGET}}*/",
		"XPPRET XPPENTRY WIDGET( XppParamList pl ) {",
		"   /*{{method: Draw( [lForce] )}}*/",
		"}",
		"/*{{end-structure}}*/",
	))
	// file B references them, one good and one broken of each kind.
	b := scan(t, "b.cpp", src(
		"/*{{function: UserOk | ilink: <function Foo> the foo | ilink: <method WIDGET:Draw> draws}}*/",
		"/*{{function: UserBad | ilink: <function Nope> x | ilink: <method WIDGET:Missing> y}}*/",
	))
	files := []*File{a, b}
	ResolveProject(files)
	if !hasDiag(b, "ilink target <function Nope> is not defined") {
		t.Fatalf("missing broken entity ilink: %v", b.Diags)
	}
	if !hasDiag(b, "ilink target <method WIDGET:Missing> is not defined") {
		t.Fatalf("missing broken component ilink: %v", b.Diags)
	}
	// the good links must NOT be reported
	if hasDiag(b, "<function Foo>") || hasDiag(b, "<method WIDGET:Draw>") {
		t.Fatalf("good ilink wrongly flagged: %v", b.Diags)
	}
}

func TestResolveProjectDuplicates(t *testing.T) {
	a := scan(t, "a.cpp", src("/*{{function: Dup}}*/"))
	b := scan(t, "b.cpp", src("/*{{function: DUP}}*/")) // Xbase++: case-insensitive clash
	ResolveProject([]*File{a, b})
	if !hasDiag(b, "already defined in") {
		t.Fatalf("cross-file duplicate not reported: %v", b.Diags)
	}
	// a structure may REOPEN across blocks (same identity) without a duplicate error
	c := scan(t, "c.cpp", src("/*{{structure: WIDGET}}*/"))
	d := scan(t, "d.cpp", src("/*{{structure: WIDGET}}*/"))
	ResolveProject([]*File{c, d})
	if hasDiag(d, "already defined") {
		t.Fatalf("structure reopen wrongly flagged as duplicate: %v", d.Diags)
	}
}

func TestResolveProjectCrossFileNote(t *testing.T) {
	// note defined in one file, included from another: resolves project-wide.
	a := scan(t, "a.cpp", src(
		"/*{{begin-note | note-id: shared}}*/",
		"/*{{note: the shared body}}*/",
		"/*{{end-note}}*/",
	))
	b := scan(t, "b.cpp", src(
		"/*{{begin-function}}*/",
		"/*{{function: Q}}*/",
		"/*{{include-note-id: shared}}*/",
		"int Q(){}",
		"/*{{end-function}}*/",
	))
	ResolveProject([]*File{a, b})
	if hasDiag(b, "has no matching begin-note") {
		t.Fatalf("cross-file include wrongly flagged: %v", b.Diags)
	}
	if hasDiag(a, "never included") {
		t.Fatalf("note used across files still flagged unused: %v", a.Diags)
	}
}

func TestNoteCycle(t *testing.T) {
	// Built by hand: a note body embedding {{include-note-id: X}} collides with
	// the note-body }}*/ close when authored, so exercise the graph directly.
	mk := func(id, dep string) Note {
		body := "just text"
		if dep != "" {
			body = "see {{include-note-id: " + dep + "}}"
		}
		return Note{ID: id, Body: body, StartLine: 1}
	}
	// na -> nb -> na  (a cycle), plus a dangling include, plus an unused note.
	fCycle := &File{Path: "c.cpp", Notes: []Note{mk("na", "nb"), mk("nb", "na")}}
	fDangle := &File{Path: "d.cpp", Notes: []Note{mk("solo", "ghost")}}
	ResolveProject([]*File{fCycle, fDangle})
	if !hasDiag(fCycle, "note include cycle") {
		t.Fatalf("cycle not detected: %v", fCycle.Diags)
	}
	if !hasDiag(fDangle, "has no matching begin-note") {
		t.Fatalf("dangling note include not detected: %v", fDangle.Diags)
	}
}

func TestNoteComposition(t *testing.T) {
	// The real TXbFpQParam shape: begin-note opens a note COMPOSED of note:
	// fragments scattered through real code, each a body line, some pulling in
	// another note via include-note-id, closed by end-note.
	f := scan(t, "c.cpp", src(
		"/*{{begin-note | note-id: QCall-Params | title: The params}}*/",
		"BOOL InitParam( XppParamList pl ) {",
		"   /*{{note:",
		"   - `__vo` void ; takes no parameters.}}*/",
		"   switch( q ) {",
		"      case QT_BOOL: return TRUE; /*{{note: - `__bo` QT_BOOL |include-note-id: IO_QT_BOOL }}*/",
		"      case QT_INT32: return TRUE; /*{{note: - `__sl` QT_INT32 |include-note-id: IO_QT_INT32 }}*/",
		"   }",
		"}",
		"/*{{end-note}}*/",
	))
	if len(f.Diags) != 0 {
		t.Fatalf("diags = %v", f.Diags)
	}
	if len(f.Notes) != 1 {
		t.Fatalf("notes = %+v", f.Notes)
	}
	n := f.Notes[0]
	if n.ID != "qcall-params" || n.Title != "The params" {
		t.Fatalf("id/title = %q / %q", n.ID, n.Title)
	}
	// a fragment's alignment indentation is layout, not content: no body line
	// keeps the leading blanks of the marker (relative indentation would)
	for _, l := range strings.Split(n.Body, "\n") {
		if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") {
			t.Fatalf("fragment indentation leaked into the body: %q", n.Body)
		}
	}
	// three fragments -> three body paragraphs, in order; interleaved code ignored
	for _, want := range []string{"`__vo` void", "`__bo` QT_BOOL", "`__sl` QT_INT32"} {
		if !strings.Contains(n.Body, want) {
			t.Fatalf("body missing %q: %q", want, n.Body)
		}
	}
	if strings.Contains(n.Body, "InitParam") || strings.Contains(n.Body, "switch") {
		t.Fatalf("interleaved code leaked into body: %q", n.Body)
	}
	// the two include-note-id fields became note->note deps
	if len(n.Includes) != 2 || n.Includes[0] != "IO_QT_BOOL" || n.Includes[1] != "IO_QT_INT32" {
		t.Fatalf("includes = %v", n.Includes)
	}
}

func TestCompactNote(t *testing.T) {
	// the compact form: one note-id: marker, |: body, |note: caveat, and a
	// note->note dependency via include-note-id.
	f := scan(t, "n.cpp", src(
		"/*{{note-id: Char-Buffer | title: The buffer",
		"   |: CHARACTER variables share an internal buffer",
		"      until one of them changes.",
		"   | note: pass by value only when it will not change.",
		"   | include-note-id: copy-on-write}}*/",
	))
	if len(f.Notes) != 1 {
		t.Fatalf("notes = %+v (diags %v)", f.Notes, f.Diags)
	}
	n := f.Notes[0]
	if n.ID != "char-buffer" { // doc-internal: lowercased
		t.Fatalf("id = %q", n.ID)
	}
	if n.Title != "The buffer" {
		t.Fatalf("title = %q", n.Title)
	}
	if !strings.Contains(n.Body, "internal buffer until one of them changes") {
		t.Fatalf("body = %q", n.Body)
	}
	if !strings.Contains(n.Body, "pass by value only when it will not change") {
		t.Fatalf("note caveat not in body: %q", n.Body)
	}
	if len(n.Includes) != 1 || n.Includes[0] != "copy-on-write" {
		t.Fatalf("includes = %v", n.Includes)
	}
	// identity freeze: a body written as "| body" (no colon) after the id does
	// not get swallowed into the id; it warns and the id stays clean.
	g := scan(t, "b.cpp", src(
		"/*{{note-id: bad-style|  a body written without the colon",
		"    spanning a second line}}*/",
	))
	if len(g.Notes) != 1 || g.Notes[0].ID != "bad-style" {
		t.Fatalf("bad-style note id not clean: %+v", g.Notes)
	}
	if !hasDiag(g, "text after the identity") {
		t.Fatalf("expected identity-freeze warning: %v", g.Diags)
	}
}

// TestRawCapture checks the segment blobs: every construct keeps its own marker
// lines verbatim and in order, never the code between them; a component's
// lines are carved out of its class's blob; a composed note keeps only its
// fragment markers.
func TestRawCapture(t *testing.T) {
	f := scan(t, "r.cpp", src(
		"/*{{begin-function}}*/",
		"/*{{function: Foo",
		"            | desc: a thing}}*/",
		"int Foo() {",
		"   return 0;",
		"}",
		"/*{{include-note-id: shared}}*/",
		"/*{{end-function}}*/",
		"/*{{begin-structure}}*/",
		"/*{{structure: WIDGET}}*/",
		"void w() {",
		"   /*{{method: Draw( [lForce] )",
		"            | desc: draws it.}}*/",
		"   code();",
		"}",
		"/*{{end-structure}}*/",
		"/*{{note-id: shared |: the shared body}}*/",
		"/*{{begin-note | note-id: composed}}*/",
		"BOOL f() {",
		"   case 1: return TRUE; /*{{note: - one |include-note-id: shared }}*/",
		"   /*{{note:",
		"   - two}}*/",
		"}",
		"/*{{end-note}}*/",
	))
	if n := count(f, Error); n != 0 {
		t.Fatalf("errors: %v", f.Diags)
	}
	join := func(r []string) string { return strings.Join(r, "\n") }

	// scoped function: begin, header, include, end - and no C code
	fn := f.Entities[0]
	if fn.Ident != "Foo" || len(fn.Raw) != 5 {
		t.Fatalf("function raw = %q", fn.Raw)
	}
	if got := join(fn.Raw); !strings.Contains(got, "begin-function") || !strings.Contains(got, "| desc: a thing}}*/") ||
		!strings.Contains(got, "include-note-id: shared") || !strings.Contains(got, "end-function") || strings.Contains(got, "return 0") {
		t.Fatalf("function raw = %q", fn.Raw)
	}

	// structure: its blob excludes the method's lines; the method has them
	st := f.Entities[1]
	if st.Ident != "WIDGET" || strings.Contains(join(st.Raw), "method:") || strings.Contains(join(st.Raw), "code()") {
		t.Fatalf("structure raw = %q", st.Raw)
	}
	if len(st.Components) != 1 || len(st.Components[0].Raw) != 2 || !strings.Contains(st.Components[0].Raw[0], "method: Draw") {
		t.Fatalf("component raw = %q", st.Components[0].Raw)
	}

	// compact note: its single marker line
	shared := f.NoteByID("shared")
	if shared == nil || len(shared.Raw) != 1 || !strings.Contains(shared.Raw[0], "note-id: shared") {
		t.Fatalf("compact note raw = %+v", shared)
	}
	// composed note: begin-note, the two fragments (3 lines), end-note - no code
	comp := f.NoteByID("composed")
	if comp == nil || len(comp.Raw) != 5 {
		t.Fatalf("composed note raw = %q", comp.Raw)
	}
	if got := join(comp.Raw); strings.Contains(got, "BOOL f()") || strings.Contains(got, "case 1: return TRUE; /*{{note: - one") == false ||
		!strings.Contains(got, "- two}}*/") || !strings.Contains(got, "end-note") {
		t.Fatalf("composed note raw = %q", comp.Raw)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name    string
		lines   []string
		resolve bool
		errs    int
		substr  string
	}{
		{"empty note (bare text ignored)", []string{
			"/*{{begin-note | note-id: bad}}*/",
			"Bare body line.", // interleaved text/code is ignored now, not an error
			"/*{{end-note}}*/",
		}, false, 0, "is empty"},
		{"dangling include", []string{
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"   }}*/",
			"void f(){}",
			"/*{{include-note-id: nope}}*/",
			"/*{{end-function}}*/",
		}, true, 1, "no matching begin-note"},
		{"mismatched end", []string{
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"   }}*/",
			"/*{{end-class}}*/",
		}, false, 1, "mismatched scope"},
		{"end without begin", []string{
			"/*{{end-function}}*/",
		}, false, 1, "without matching begin"},
		{"begin without end", []string{
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"   }}*/",
		}, false, 1, "without matching end"},
		{"header unclosed at EOF", []string{
			"/*{{begin-function}}*/",
			"/*{{function: f",
		}, false, 3, "not closed with }}*/"},
		{"nested begin", []string{
			"/*{{begin-function}}*/",
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"   }}*/",
			"/*{{end-function}}*/",
		}, false, 1, "never nest"},
		{"duplicate note-id", []string{
			"/*{{begin-note | note-id: x}}*/",
			"/*{{note:",
			"a",
			"}}*/",
			"/*{{end-note}}*/",
			"/*{{begin-note | note-id: x}}*/",
			"/*{{note:",
			"b",
			"}}*/",
			"/*{{end-note}}*/",
		}, false, 1, "duplicate note-id"},
		{"begin-note without id", []string{
			"/*{{begin-note}}*/",
			"/*{{note:",
			"a",
			"}}*/",
			"/*{{end-note}}*/",
		}, false, 1, "without note-id"},
		{"scope without header", []string{
			"/*{{begin-function}}*/",
			"/*{{end-function}}*/",
		}, false, 1, "no header marker"},
		{"stray comment close", []string{
			"*/",
		}, false, 1, "without a matching /*"},
		{"comment open at EOF", []string{
			"/*",
		}, false, 1, "comment still open"},
		{"duplicate identity", []string{
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"   }}*/",
			"/*{{end-function}}*/",
			"/*{{begin-function}}*/",
			"/*{{function: F",
			"   }}*/",
			"/*{{end-function}}*/",
		}, false, 1, "duplicate identity"},
		{"duplicate mangled-name", []string{
			"/*{{begin-c-function}}*/",
			"/*{{c-function: a",
			"            | mangled-name: _x_",
			"   }}*/",
			"/*{{end-c-function}}*/",
			"/*{{begin-c-function}}*/",
			"/*{{c-function: b",
			"            | mangled-name: _x_",
			"   }}*/",
			"/*{{end-c-function}}*/",
		}, false, 1, "duplicate mangled-name"},
		{"fence unclosed before marker close", []string{
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"            | example: ```",
			"   }}*/",
			"/*{{end-function}}*/",
		}, false, 1, "not closed before }}*/"},
		{"include outside scope", []string{
			"/*{{include-note-id: x}}*/",
		}, false, 1, "outside a begin/end scope"},
		{"empty marker", []string{
			"/*{{}}*/",
		}, false, 1, "empty marker"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := scan(t, "t.cpp", src(c.lines...))
			if c.resolve {
				f.Resolve()
			}
			if got := count(f, Error); got != c.errs {
				t.Fatalf("errors = %d, want %d: %v", got, c.errs, f.Diags)
			}
			if !hasDiag(f, c.substr) {
				t.Fatalf("no diag containing %q: %v", c.substr, f.Diags)
			}
		})
	}
}

func TestLint(t *testing.T) {
	t.Run("long line", func(t *testing.T) {
		f := scan(t, "t.cpp", src(
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"            | desc: "+strings.Repeat("x", 120),
			"   }}*/",
			"/*{{end-function}}*/",
		))
		if count(f, Warning) != 1 || !hasDiag(f, "characters long") {
			t.Fatalf("diags = %v", f.Diags)
		}
	})
	t.Run("todo", func(t *testing.T) {
		f := scan(t, "t.cpp", src(
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"            | todo: review",
			"   }}*/",
			"/*{{end-function}}*/",
		))
		if count(f, Warning) != 1 || !hasDiag(f, "todo field") {
			t.Fatalf("diags = %v", f.Diags)
		}
	})
	t.Run("LF endings", func(t *testing.T) {
		f := scan(t, "t.cpp", []byte("int a;\nint b;\n"))
		if count(f, Warning) != 1 || !hasDiag(f, "do not end in CRLF") {
			t.Fatalf("diags = %v", f.Diags)
		}
	})
	t.Run("non-ASCII decoded from 1252", func(t *testing.T) {
		f := scan(t, "t.cpp", src(
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"            | desc: a \x93quoted\x94 word", // 0x93/0x94: 1252 curly quotes
			"   }}*/",
			"/*{{end-function}}*/",
		))
		if count(f, Warning) != 1 || !hasDiag(f, "non-ASCII") {
			t.Fatalf("diags = %v", f.Diags)
		}
		if v, _ := f.Entities[0].Field("desc"); v != "a “quoted” word" {
			t.Fatalf("desc = %q", v)
		}
	})
	t.Run("compact scope-less header", func(t *testing.T) {
		// Draft 2: a scope-less entity marker is the valid COMPACT form, not a
		// warning. The entity is created with no diagnostic.
		f := scan(t, "t.cpp", src(
			"/*{{function: f",
			"            | desc: loose.",
			"   }}*/",
		))
		if len(f.Diags) != 0 || len(f.Entities) != 1 {
			t.Fatalf("diags = %v entities = %d", f.Diags, len(f.Entities))
		}
	})
	t.Run("unknown field flagged", func(t *testing.T) {
		f := scan(t, "t.cpp", src(
			"/*{{begin-function}}*/",
			"/*{{function: f",
			"            | patata: con aceite",
			"            | desc: ok",
			"   }}*/",
			"/*{{end-function}}*/",
		))
		if len(f.Diags) != 0 {
			t.Fatalf("diags = %v", f.Diags)
		}
		e := &f.Entities[0]
		if v, ok := e.Field("patata"); !ok || v != "con aceite" {
			t.Fatalf("patata = %q %v", v, ok)
		}
		if !e.Fields[0].Unknown || e.Fields[1].Unknown {
			t.Fatalf("unknown flags = %+v", e.Fields)
		}
	})
}

func TestSplitFields(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a: 1 | b: 2", []string{"a: 1 ", " b: 2"}},
		{"s: `a | b` | c: 2", []string{"s: `a | b` ", " c: 2"}},
		{"s: a \\| b | c: 2", []string{"s: a \\| b ", " c: 2"}},
		{"s: ``x ` y`` | c: 2", []string{"s: ``x ` y`` ", " c: 2"}},
		{"s: `unpaired | c: 2", []string{"s: `unpaired ", " c: 2"}},
		{"no pipes at all", []string{"no pipes at all"}},
	}
	for _, c := range cases {
		got := splitFields(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("%q -> %q, want %q", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%q -> %q, want %q", c.in, got, c.want)
			}
		}
	}
}

func TestFenceOpen(t *testing.T) {
	cases := []struct {
		in string
		ch byte
		n  int
		ok bool
	}{
		{"```", '`', 3, true},
		{"```prg", '`', 3, true},
		{"~~~", '~', 3, true},
		{"~~~~", '~', 4, true},
		{"see ``` here", 0, 0, false},
		{"x```", 0, 0, false},
		{"`x`", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, c := range cases {
		ch, n, ok := fenceOpen(c.in)
		if ch != c.ch || n != c.n || ok != c.ok {
			t.Fatalf("fenceOpen(%q) = %q %d %v", c.in, ch, n, ok)
		}
	}
}

func TestScanDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.cpp"), goodSrc(), 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.c"), src("int b;"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), src("not source"), 0666); err != nil {
		t.Fatal(err)
	}
	files, err := ScanDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || len(files[0].Entities) != 4 || len(files[1].Entities) != 0 {
		t.Fatalf("files = %+v", files)
	}
}
