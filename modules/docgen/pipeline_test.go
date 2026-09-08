package docgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/ot4xb-tool/modules/doccompile"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/docgen"
	"github.com/pablo-botella/ot4xb-tool/modules/docresolve"
	"github.com/pablo-botella/ot4xb-tool/modules/srcdoc"
)

const srcA = "/*{{begin-note-id}}*/\r\n" +
	"/*{{note-id: shared | title_: The shared note }}*/\r\n" +
	"/*{{|: Body of the shared note. }}*/\r\n" +
	"/*{{end-note-id}}*/\r\n" +
	"/*{{begin-class}}*/\r\n" +
	"/*{{class-name_: POINT | _slug_: point | category: winapi/structures , geometry | desc: A point. }}*/\r\n" +
	"/*{{|:**BEGIN STRUCTURE  POINT** }}*/\r\n" +
	"   /*{{|member_: - MEMBER LONG x |desc_: x coordinate. }}*/\r\n" +
	"   /*{{|member_: - MEMBER @ {{ilink: <slug size> SIZE}} sz |desc_: a size }}*/\r\n" +
	"/*{{|:**END STRUCTURE** }}*/\r\n" +
	"/*{{include-note-id: shared}}*/\r\n" +
	"/*{{end-class}}*/\r\n" +
	"/*{{begin-cpp-function}}*/\r\n" +
	"/*{{cpp-function_: f(LPSTR) | _tg_: f | syntax_: `void f(LPSTR)` }}*/\r\n" +
	"/*{{|desc: First overload. |seealso: See also: {{ilink: <class point> POINT}}, {{ilink: <tg f> f}} }}*/\r\n" +
	"/*{{end-cpp-function}}*/\r\n"

const srcB = "/*{{begin-class}}*/\r\n" +
	"/*{{class-name_: SIZE | _slug_: size | category: geometry | desc: A size. }}*/\r\n" +
	"/*{{end-class}}*/\r\n" +
	"/*{{begin-cpp-function}}*/\r\n" +
	"/*{{cpp-function_: f(LPSTR,BOOL) | _tg_: f | syntax_: `void f(LPSTR,BOOL)` }}*/\r\n" +
	"/*{{|desc: Second overload. |seealso: See also: {{ilink: <function nowhere> nowhere}} }}*/\r\n" +
	"/*{{end-cpp-function}}*/\r\n" +
	"/*{{class-name_: POINT | _slug_: other }}*/\r\n" +
	"/*{{function: Note_Only | short_: Only a note. | _kw_: note, header | note: added from a header }}*/\r\n"

func TestPipeline(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	db, err := docdb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct{ name, src string }{{"src/a.cpp", srcA}, {"src/b.cpp", srcB}} {
		sf := srcdoc.Scan(f.name, []byte(f.src))
		if sf.Errors() != 0 {
			t.Fatalf("%s: %+v", f.name, sf.Issues)
		}
		if _, err := doccompile.CompileFile(db, f.name, sf); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := docresolve.Resolve(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Missing != 1 || rep.Cycles != 0 || rep.DupSlugs != 0 {
		t.Fatalf("resolve: %+v", rep)
	}
	rows, err := db.QueryAll(`SELECT code, message FROM issues ORDER BY code`)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, r := range rows {
		codes = append(codes, r[0].(string))
	}
	if strings.Join(codes, ",") != "compile/slug-conflict,resolve/missing" {
		t.Fatalf("issues: %v", rows)
	}
	// slug rule: the first explicit slug stays
	var slug string
	var flags int64
	if err := db.QueryRow(`SELECT slug, flags FROM topics WHERE kind='class' AND key='POINT'`).Scan(&slug, &flags); err != nil {
		t.Fatal(err)
	}
	if slug != "point" || flags&docdb.FlagSlugExplicit == 0 {
		t.Fatalf("slug %q flags %d", slug, flags)
	}
	var ncat int
	if err := db.QueryRow(`SELECT COUNT(*) FROM topic_category WHERE category = 'geometry'`).Scan(&ncat); err != nil {
		t.Fatal(err)
	}
	if ncat != 2 {
		t.Fatalf("category rows: %d", ncat)
	}
	db.Close()

	out := filepath.Join(dir, "md")
	n, err := docgen.Generate(dbPath, out, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// pages: group f, POINT, SIZE, shared note, Note_Only, index
	if n != 5 {
		t.Fatalf("pages: %d", n)
	}
	point, _ := os.ReadFile(filepath.Join(out, "point.md"))
	p := string(point)
	for _, want := range []string{
		"# POINT\r\n",
		"**category:** winapi/structures , geometry",
		"**desc:** A point.",
		"- MEMBER LONG x - x coordinate.\r\n- MEMBER @ [SIZE](size.md) sz - a size\r\n",
		"**END STRUCTURE**\r\n\r\nThe shared note\r\n\r\nBody of the shared note.",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("point.md lacks %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "slug") {
		t.Errorf("hidden entries leaked:\n%s", p)
	}
	group, _ := os.ReadFile(filepath.Join(out, "f.md"))
	g := string(group)
	for _, want := range []string{"# f\r\n", "## f(LPSTR)\r\n", "## f(LPSTR,BOOL)\r\n", "`void f(LPSTR)`",
		"[POINT](point.md)", "[f](f.md)", "See also: nowhere"} {
		if !strings.Contains(g, want) {
			t.Errorf("f.md lacks %q:\n%s", want, g)
		}
	}
	idx, _ := os.ReadFile(filepath.Join(out, "index.md"))
	if !strings.Contains(string(idx), "- [Xbase++](index-xbase.md)\r\n  - [Alphabetic](index-xbase-alpha.md)\r\n") || !strings.Contains(string(idx), "- [C++ API](index-cpp.md)\r\n") {
		t.Errorf("index:\n%s", idx)
	}
	xb, _ := os.ReadFile(filepath.Join(out, "index-xbase.md"))
	for _, want := range []string{"# Xbase++\r\n", "## By category\r\n", "- [geometry](geometry.md) (2)", "- [winapi/structures](winapi-structures.md) (1)", "(no category)\r\n\r\n- [Note_Only](function-note_only.md) (function) - Only a note. (kw: note, header)\r\n", "## [Alphabetic](index-xbase-alpha.md)"} {
		if !strings.Contains(string(xb), want) {
			t.Errorf("index-xbase lacks %q:\n%s", want, xb)
		}
	}
	geo, _ := os.ReadFile(filepath.Join(out, "geometry.md"))
	for _, want := range []string{"# geometry\r\n", "[Xbase++](index-xbase.md)", "- [POINT](point.md) - A point.\r\n- [SIZE](size.md) - A size.\r\n"} {
		if !strings.Contains(string(geo), want) {
			t.Errorf("geometry.md lacks %q:\n%s", want, geo)
		}
	}
	alpha, _ := os.ReadFile(filepath.Join(out, "index-xbase-alpha.md"))
	if !strings.Contains(string(alpha), "- [Note_Only](function-note_only.md) (function) - Only a note. (kw: note, header)\r\n- [POINT](point.md) - A point.\r\n- [SIZE](size.md) - A size.\r\n") {
		t.Errorf("index-xbase-alpha:\n%s", alpha)
	}
	cpp, _ := os.ReadFile(filepath.Join(out, "index-cpp.md"))
	if !strings.Contains(string(cpp), "- [f](f.md) - First overload.\r\n") {
		t.Errorf("index-cpp:\n%s", cpp)
	}
}
