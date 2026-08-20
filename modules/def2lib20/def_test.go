package def2lib20

import (
	"strings"
	"testing"
)

const sample = "LIBRARY ot4xb\r\nEXPORTS\r\n;;------------------------------------------------------------\r\n;; SRC: ot4xb.cpp:OT4XB\r\n;; DOC: PEN\r\n     OT4XB =  _OT4XB\r\n\r\n     APPINSTANCE =  _APPINSTANCE\r\n     GETCURRENTPROCESSHANDLE =  _GETCURRENTPROCESSHANDLE\r\n     PLAIN\r\n     WITHORD = _WITHORD @12\r\n     HIDDEN  PRIVATE\r\n     APPINSTANCE =  _APPINSTANCE\r\n"

func TestParse(t *testing.T) {
	f, err := ParseDef(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if f.Library != "ot4xb" {
		t.Errorf("Library = %q", f.Library)
	}
	want := []Export{
		{Name: "OT4XB", Internal: "_OT4XB", Line: 6},
		{Name: "APPINSTANCE", Internal: "_APPINSTANCE", Line: 8},
		{Name: "GETCURRENTPROCESSHANDLE", Internal: "_GETCURRENTPROCESSHANDLE", Line: 9},
		{Name: "PLAIN", Line: 10},
		{Name: "WITHORD", Internal: "_WITHORD", Ordinal: 12, Line: 11},
		{Name: "HIDDEN", Private: true, Line: 12},
		{Name: "APPINSTANCE", Internal: "_APPINSTANCE", Line: 13},
	}
	if len(f.Exports) != len(want) {
		t.Fatalf("got %d exports, want %d: %+v", len(f.Exports), len(want), f.Exports)
	}
	for i := range want {
		if f.Exports[i] != want[i] {
			t.Errorf("export %d = %+v, want %+v", i, f.Exports[i], want[i])
		}
	}
	list, dups := f.Imports()
	if len(list) != 5 || len(dups) != 1 || dups[0].Name != "APPINSTANCE" || dups[0].Line != 13 {
		t.Errorf("Imports = %d entries, dups %+v", len(list), dups)
	}
	for _, e := range list {
		if e.Private {
			t.Errorf("private export %s in Imports", e.Name)
		}
	}
}

func TestLineEndings(t *testing.T) {
	for _, eol := range []string{"\r\n", "\n", "\r"} {
		src := "LIBRARY x" + eol + "EXPORTS" + eol + "A" + eol + "B = _B" + eol + "C" // no final terminator
		f, err := ParseDef(strings.NewReader(src))
		if err != nil {
			t.Fatalf("%q: %v", eol, err)
		}
		if len(f.Exports) != 3 || f.Exports[2].Name != "C" || f.Exports[2].Line != 5 {
			t.Errorf("%q: exports %+v", eol, f.Exports)
		}
	}
}

func TestErrors(t *testing.T) {
	for _, bad := range []string{"EXPORTS\nA B", "EXPORTS\n=X", "EXPORTS\nA @x", "EXPORTS\nA @0"} {
		if _, err := ParseDef(strings.NewReader(bad)); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}

func TestOtherStatements(t *testing.T) {
	src := "NAME foo\nDESCRIPTION 'x'\nEXPORTS\nA\nSTACKSIZE 1000\nB\n"
	f, err := ParseDef(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	// B comes after STACKSIZE, which ends the EXPORTS section
	if len(f.Exports) != 1 || f.Exports[0].Name != "A" {
		t.Errorf("exports %+v", f.Exports)
	}
}
