package docgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/ot4xb-tool/modules/doccompile"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/docresolve"
)

func src(lines ...string) []byte { return []byte(strings.Join(lines, "\r\n") + "\r\n") }

func TestGenerate(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0777)
	file := filepath.Join(root, "src", "a.cpp")
	os.WriteFile(file, src(
		"/*{{begin-function}}*/",
		"/*{{function: FpQCall",
		"            | syntax: `FpQCall( fpSpec, cPrototype, ... ) -> xResult`",
		"            | category: function-pointer",
		"            | desc: Calls a function through an explicit prototype.",
		"            | param fpSpec: Numeric - The function to call.",
		"            | return: The value returned by the called function.",
		"            | see-also: Widget",
		"            | slug: fpqcall}}*/",
		"XPPRET XPPENTRY FPQCALL( XppParamList pl ) {}",
		"/*{{include-note-id: shared}}*/",
		"/*{{end-function}}*/",
		"/*{{begin-structure}}*/",
		"/*{{structure: WIDGET | desc: a widget}}*/",
		"void w() {",
		"   /*{{gwst-member: hdr type: NMHDR pos: 0 size: 12}}*/",
		"   /*{{gwst-member: cb type: DWORD pos: 12 size: 4}}*/",
		"   /*{{method: Draw( [lForce] ) | desc: draws it}}*/",
		"}",
		"/*{{end-structure}}*/",
		"/*{{structure: WAPIST_NMHDR | desc: the header}}*/",
		"/*{{note-id: shared |: the shared body | include-note-id: deeper}}*/",
		"/*{{note-id: deeper |: and the deeper body}}*/",
	), 0666)
	dbPath := filepath.Join(root, "doc.db")
	if _, err := doccompile.Compile(dbPath, root, []string{file}, nil); err != nil {
		t.Fatal(err)
	}
	d, _ := docdb.Open(dbPath)
	if _, err := docresolve.Resolve(d); err != nil {
		t.Fatal(err)
	}
	d.Close()

	out := filepath.Join(root, "md")
	n, err := Generate(dbPath, out, nil)
	if err != nil || n != 8 { // function, 2 structures, 2 gwst-members, method, 2 notes
		t.Fatalf("Generate: n=%d err=%v", n, err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return string(b)
	}
	// the explicit slug names the function's file; fields are laid out; the
	// see-also resolves to the structure's file; the included note is linked
	fn := read("fpqcall.md")
	for _, want := range []string{
		"# FpQCall", "*function* · function-pointer", "`src/a.cpp:1`",
		"`FpQCall( fpSpec, cPrototype, ... ) -> xResult`",
		"Calls a function through an explicit prototype.",
		"**Parameters**", "- `fpSpec` — Numeric - The function to call.",
		"**Returns** — The value returned",
		"see-also: [Widget](structure-widget.md)",
		// the included note is transcluded on the page, recursively
		"the shared body", "and the deeper body",
	} {
		if !strings.Contains(fn, want) {
			t.Fatalf("fpqcall.md lacks %q:\n%s", want, fn)
		}
	}
	// the slug names the file; it is metadata, never page content
	if strings.Contains(fn, "slug:") {
		t.Fatalf("slug field leaked into the page:\n%s", fn)
	}
	// the structure lists its member with a link to the member's own file
	// a structure page: the layout as a definition, in declaration order, an
	// embedded structure linked to the page where it is defined, scalars plain;
	// the method still in Members; no "includes note" line anywhere
	st := read("structure-widget.md")
	for _, want := range []string{
		"## Structure Definition", "**BEGIN STRUCTURE**",
		"- MEMBER @ [WAPIST_NMHDR](structure-wapist_nmhdr.md) hdr",
		"- MEMBER DWORD cb", "**END STRUCTURE**",
		"## Members", "[WIDGET:Draw](method-widget.draw.md)",
	} {
		if !strings.Contains(st, want) {
			t.Fatalf("structure page lacks %q:\n%s", want, st)
		}
	}
	if strings.Index(st, "hdr") > strings.Index(st, "MEMBER DWORD cb") {
		t.Fatalf("members out of declaration order:\n%s", st)
	}
	if strings.Contains(fn, "includes note") {
		t.Fatalf("include listed instead of transcluded:\n%s", fn)
	}
	mth := read("method-widget.draw.md")
	if !strings.Contains(mth, "`Draw( [lForce] )`") || !strings.Contains(mth, "draws it") {
		t.Fatalf("method page:\n%s", mth)
	}
	note := read("note-shared.md")
	if !strings.Contains(note, "the shared body") {
		t.Fatalf("note page:\n%s", note)
	}
	// the index lists every topic by kind, and the output is CRLF
	idx := read("index.md")
	if !strings.Contains(idx, "## function (1)") || !strings.Contains(idx, "[FpQCall](fpqcall.md)") || !strings.Contains(idx, "\r\n") {
		t.Fatalf("index:\n%s", idx)
	}
	// a second run rewrites nothing (idempotent)
	if _, err := Generate(dbPath, out, func(s string) {
		if strings.Contains(s, "written") && !strings.Contains(s, "(0 file(s) written") {
			t.Fatalf("second run rewrote files: %s", s)
		}
	}); err != nil {
		t.Fatal(err)
	}
}
