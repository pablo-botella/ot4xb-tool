package docresolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/ot4xb-tool/modules/doccompile"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
)

func src(lines ...string) []byte { return []byte(strings.Join(lines, "\r\n") + "\r\n") }

func issues(t *testing.T, d *docdb.DB, code string) []string {
	t.Helper()
	rows, err := d.QueryAll(`SELECT message FROM issues WHERE code = ? ORDER BY rowid`, code)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r[0].(string))
	}
	return out
}

func TestResolve(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0777)
	a := filepath.Join(root, "src", "a.cpp")
	b := filepath.Join(root, "src", "b.cpp")
	os.WriteFile(a, src(
		"/*{{function: Foo | ilink: <class WIDGET> ok | ilink: <function Nope> broken | see-also: Whatever}}*/",
		"/*{{begin-function}}*/",
		"/*{{function: Bar}}*/",
		"int Bar(){}",
		"/*{{include-note-id: shared}}*/",
		"/*{{include-note-id: ghost}}*/",
		"/*{{end-function}}*/",
		"/*{{structure: WIDGET | parent: gwst,GWST}}*/",
		"/*{{structure: GWST}}*/",
		"/*{{note-id: shared |: body}}*/",
		"/*{{note-id: na |: a | include-note-id: nb}}*/",
		"/*{{note-id: nb |: b | include-note-id: na}}*/",
		"/*{{function: Dup}}*/",
	), 0666)
	// second source: reopens WIDGET (fine) and redefines Dup (an issue), and
	// links to the structure by its class name (same family)
	os.WriteFile(b, src(
		"/*{{structure: widget | desc: reopened}}*/",
		"/*{{function: DUP}}*/",
		"/*{{function: Baz | ilink: <structure Widget> family lookup}}*/",
	), 0666)
	dbPath := filepath.Join(root, "doc.db")
	if _, err := doccompile.Compile(dbPath, root, []string{a, b}, nil); err != nil {
		t.Fatal(err)
	}
	d, err := docdb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	rep, err := Resolve(d)
	if err != nil {
		t.Fatal(err)
	}
	// missing: <function Nope>, include ghost  (see-also skipped; parent GWST, class WIDGET, structure Widget all found)
	if rep.Missing != 2 {
		t.Fatalf("missing = %d: %v", rep.Missing, issues(t, d, "resolve/missing"))
	}
	if rep.Skipped != 1 {
		t.Fatalf("skipped = %d", rep.Skipped)
	}
	if rep.Cycles != 1 || len(issues(t, d, "resolve/cycle")) != 1 {
		t.Fatalf("cycles = %d: %v", rep.Cycles, issues(t, d, "resolve/cycle"))
	}
	// duplicates: Dup/DUP (function, twice) -> 1; WIDGET reopened -> none
	dups := issues(t, d, "resolve/duplicate")
	if rep.Duplicates != 1 || len(dups) != 1 || !strings.Contains(dups[0], "Dup") {
		t.Fatalf("duplicates = %d: %v", rep.Duplicates, dups)
	}
	// re-runnable: a second run recomputes, never accumulates
	if _, err := Resolve(d); err != nil {
		t.Fatal(err)
	}
	if n := len(issues(t, d, "resolve/missing")); n != 2 {
		t.Fatalf("issues accumulated across runs: %d", n)
	}
}
