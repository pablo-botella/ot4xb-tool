package doccompile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
)

func src(lines ...string) []byte { return []byte(strings.Join(lines, "\r\n") + "\r\n") }

func count(t *testing.T, d *docdb.DB, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := d.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestKey(t *testing.T) {
	for _, c := range []struct{ kind, ident, want string }{
		{"function", "Array2ppMarshall", "ARRAY2PPMARSHALL"},
		{"structure", "large_integer", "LARGE_INTEGER"},
		{"c-function", "_conGetLong", "_conGetLong"},
		{"cpp-function", "json_ns::serialize( XppParamList )", "json_ns::serialize(XppParamList)"},
		{"note", "Con-Get-Long-Ex", "con-get-long-ex"},
		{"topic", "GWST-Commands", "gwst-commands"},
	} {
		if got := Key(c.kind, c.ident); got != c.want {
			t.Fatalf("Key(%s, %q) = %q, want %q", c.kind, c.ident, got, c.want)
		}
	}
	if got := componentKey("structure", "LARGE_INTEGER", "new"); got != "LARGE_INTEGER:NEW" {
		t.Fatalf("componentKey = %q", got)
	}
}

func TestCompileRoundTrip(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0777)
	file := filepath.Join(root, "src", "a.cpp")
	os.WriteFile(file, src(
		"/*{{begin-function}}*/",
		"/*{{function: Foo",
		"            | category: memory/string",
		"            | see-also: Bar, Baz",
		"            | ilink: <structure WIDGET> the widget}}*/",
		"int Foo() { return 0; }",
		"/*{{include-note-id: shared}}*/",
		"/*{{end-function}}*/",
		"/*{{begin-structure}}*/",
		"/*{{structure: WIDGET | parent: gwst,GWST | category: ui}}*/",
		"void w() { /*{{method: Draw( [lForce] ) | desc: draws}}*/ }",
		"/*{{end-structure}}*/",
		"/*{{note-id: shared |: the body | include-note-id: other}}*/",
		"/*{{begin-topic}}*/",
		"/*{{topic: Cmds}}*/",
		"prose",
		"/*{{command: DO-IT | desc: does}}*/",
		"/*{{end-topic}}*/",
	), 0666)
	dbPath := filepath.Join(root, "doc.db")

	n, err := Compile(dbPath, root, []string{file}, nil)
	if err != nil || n != 1 {
		t.Fatalf("Compile: n=%d err=%v", n, err)
	}
	d, err := docdb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// source registered relative to root, normalized
	var s string
	d.QueryRow(`SELECT src FROM sources`).Scan(&s)
	if s != "src/a.cpp" {
		t.Fatalf("src = %q", s)
	}
	// topics: function FOO, structure WIDGET, method WIDGET:DRAW, note shared, topic cmds, command DO-IT
	if got := count(t, d, `SELECT COUNT(*) FROM topics`); got != 6 {
		t.Fatalf("topics = %d", got)
	}
	var key string
	d.QueryRow(`SELECT key FROM topics WHERE kind = 'method'`).Scan(&key)
	if key != "WIDGET:DRAW" {
		t.Fatalf("method key = %q", key)
	}
	// segments follow the file order, packed pos
	rows, err := d.QueryAll(`SELECT t.kind, s.pos FROM segments s JOIN topics t USING(idtopic) ORDER BY s.pos`)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	var last int64 = -1
	for _, r := range rows {
		k, p := r[0].(string), r[1].(int64)
		if p <= last {
			t.Fatalf("pos not increasing: %d after %d", p, last)
		}
		last = p
		kinds = append(kinds, k)
	}
	if strings.Join(kinds, ",") != "function,structure,method,note,topic,command" {
		t.Fatalf("segment order = %v", kinds)
	}
	// the raw blob is the marker text, not the code
	var raw []byte
	d.QueryRow(`SELECT s.raw FROM segments s JOIN topics t USING(idtopic) WHERE t.kind = 'function'`).Scan(&raw)
	if !strings.Contains(string(raw), "begin-function") || strings.Contains(string(raw), "return 0") {
		t.Fatalf("function raw = %q", raw)
	}
	// references: include (entity->note), ilink, 2 see-also (unqualified), gwst-parent, in-topic, note->note include
	if got := count(t, d, `SELECT COUNT(*) FROM refs WHERE reftype = 'include'`); got != 2 {
		t.Fatalf("include refs = %d", got)
	}
	if got := count(t, d, `SELECT COUNT(*) FROM refs WHERE reftype = 'see-also' AND reftokind = ''`); got != 2 {
		t.Fatalf("see-also refs = %d", got)
	}
	if got := count(t, d, `SELECT COUNT(*) FROM refs WHERE reftype = 'ilink' AND reftokind = 'structure' AND reftoident = 'WIDGET'`); got != 1 {
		t.Fatal("ilink ref missing")
	}
	if got := count(t, d, `SELECT COUNT(*) FROM refs WHERE reftype = 'gwst-parent' AND reftoident = 'GWST'`); got != 1 {
		t.Fatal("gwst-parent ref missing")
	}
	if got := count(t, d, `SELECT COUNT(*) FROM refs WHERE reftype = 'in-topic' AND reftoident = 'cmds'`); got != 1 {
		t.Fatal("in-topic ref missing")
	}
	// categories: memory/string on Foo, ui on WIDGET
	if got := count(t, d, `SELECT COUNT(*) FROM topic_category`); got != 2 {
		t.Fatalf("categories = %d", got)
	}

	// recompiling the same file is idempotent: same counts, same source id/pos
	before := count(t, d, `SELECT COUNT(*) FROM segments`)
	d.Close()
	if _, err := Compile(dbPath, root, []string{file}, nil); err != nil {
		t.Fatal(err)
	}
	d, _ = docdb.Open(dbPath)
	defer d.Close() // the earlier defer captured the first handle; this one must close too (Windows holds the file)
	if count(t, d, `SELECT COUNT(*) FROM segments`) != before || count(t, d, `SELECT COUNT(*) FROM sources`) != 1 ||
		count(t, d, `SELECT COUNT(*) FROM topics`) != 6 {
		t.Fatal("recompile is not idempotent")
	}
}
