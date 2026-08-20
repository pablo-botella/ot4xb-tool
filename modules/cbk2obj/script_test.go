package cbk2obj

import (
	"strings"
	"testing"
)

func parseSample(t *testing.T) *Script {
	t.Helper()
	s, diags, err := ParseFile("testdata/sample.cbk")
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diags = %v", diags)
	}
	return s
}

func TestParseSample(t *testing.T) {
	s := parseSample(t)
	want := []Callback{
		{Name: "MyWndProc", Ret: DWord, Params: []Kind{DWord, DWord, DWord, DWord}, Conv: StdCall, Line: 3},
		{Name: "MyEnum", Ret: Bool, Params: []Kind{DWord, DWord}, Conv: StdCall, Line: 4},
		{Name: "MyCmp", Ret: DWord, Params: []Kind{DWord, Double, Bool}, Conv: StdCall, Line: 5},
		{Name: "MyVoid", Ret: Void, Params: nil, Conv: CDecl, Line: 11},
		{Name: "MyQ", Ret: QWord, Params: []Kind{QWord, Float, Word, Byte}, Conv: StdCall, Line: 13}, // USING CDECL was spent by MyVoid
		{Name: "MyDbl", Ret: Double, Params: []Kind{DWord}, Conv: CDecl, Line: 20},
	}
	if s.Version != "001.000.010" {
		t.Errorf("Version = %q", s.Version)
	}
	if len(s.Callbacks) != len(want) {
		t.Fatalf("got %d callbacks: %+v", len(s.Callbacks), s.Callbacks)
	}
	for i, w := range want {
		g := s.Callbacks[i]
		if g.Name != w.Name || g.Ret != w.Ret || g.Conv != w.Conv || g.Line != w.Line || len(g.Params) != len(w.Params) {
			t.Errorf("callback %d = %+v, want %+v", i, g, w)
			continue
		}
		for j := range w.Params {
			if g.Params[j] != w.Params[j] {
				t.Errorf("callback %s param %d = %v, want %v", w.Name, j, g.Params[j], w.Params[j])
			}
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		diags []Diag // nil = no errors
		ncb   int
	}{
		{"unknown command", "FOO BAR\r\n", []Diag{{1, "unknow command"}}, 0},
		{"USING alone", "USING\r\n", []Diag{{1, "unknow command"}}, 0},
		{"USING other", "USING FASTCALL\r\n", []Diag{{1, "unknow command"}}, 0},
		{"__CDECL alone (C7)", "__CDECL\r\nBEGIN CALLBACK A RETURNS VOID\r\nEND CALLBACK\r\n", nil, 1},
		{"template exact match (C9)", "CALLBACK PROC x\r\n", []Diag{{1, "unknow template"}}, 0},
		{"template without name", "CALLBACK WNDPROC\r\n", []Diag{{1, "Bad Syntax"}}, 0},
		{"BEGIN without filler word", "BEGIN CALLBACK X DWORD\r\n", []Diag{{1, "Bad Syntax"}}, 0},
		{"BEGIN bad type", "BEGIN CALLBACK X RETURNS foo\r\n", []Diag{{1, "invalid type FOO"}}, 0},
		{"PARAM outside", "PARAM DWORD\r\n", []Diag{{1, "PARAM defined outside CALLBACK"}}, 0},
		{"PARAM VOID (C11)", "BEGIN CALLBACK A RETURNS VOID\r\nPARAM VOID\r\nEND CALLBACK\r\n", []Diag{{2, "invalid type VOID"}}, 1},
		{"PARAM unknown type (C11)", "BEGIN CALLBACK A RETURNS VOID\r\nPARAM foo\r\nEND CALLBACK\r\n", []Diag{{2, "invalid type FOO"}}, 1},
		{"PARAM without type", "BEGIN CALLBACK A RETURNS VOID\r\nPARAM\r\nEND CALLBACK\r\n", []Diag{{2, "Bad Syntax"}}, 1},
		{"END without BEGIN", "END CALLBACK\r\n", []Diag{{1, "END CALLBACK no match BEGIN CALLBACK"}}, 0},
		{"nested BEGIN", "BEGIN CALLBACK A RETURNS DWORD\r\nBEGIN CALLBACK B RETURNS DWORD\r\nEND CALLBACK\r\n", []Diag{{2, "Unclosed control structures"}}, 1},
		{"template inside BEGIN", "BEGIN CALLBACK A RETURNS DWORD\r\nCALLBACK WNDPROC B\r\nEND CALLBACK\r\n", []Diag{{2, "Unclosed control structures"}}, 1},
		{"duplicate name", "CALLBACK WNDPROC Abc\r\nCALLBACK WNDPROC aBC\r\n", []Diag{{2, "Dupe function name aBC"}}, 1},
		{"version too high", "XPPCBK VERSION 001.000.018\r\n", []Diag{{1, "THIS SCRIPT REQUIRE XPPCBK VERSION >= 001.000.018"}}, 0},
		{"version ok", "XPPCBK VERSION 001.000.017\r\n", nil, 0},
		{"version missing", "XPPCBK VERSION\r\n", []Diag{{1, "Bad Syntax"}}, 0},
		{"bad function name", "BEGIN CALLBACK 1abc RETURNS DWORD\r\nEND CALLBACK\r\n", []Diag{{1, "invalid function name 1abc"}, {2, "END CALLBACK no match BEGIN CALLBACK"}}, 0},
		{"unclosed at EOF", "BEGIN CALLBACK A RETURNS DWORD\r\nPARAM DWORD", []Diag{{2, "Unclosed control structures"}}, 0},
		{"comment and blank lines", "// only a comment\r\n\r\n   \r\nCALLBACK TIMERPROC T // trailing\r\n", nil, 1},
		{"tabs", "BEGIN\tCALLBACK\tA\tRETURNS\tDWORD\t// c\r\n\tPARAM\tBOOL\r\nEND\tCALLBACK\r\n", nil, 1},
	}
	for _, c := range cases {
		s, diags, err := Parse(strings.NewReader(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(diags) != len(c.diags) {
			t.Errorf("%s: diags = %v, want %v", c.name, diags, c.diags)
		} else {
			for i := range diags {
				if diags[i] != c.diags[i] {
					t.Errorf("%s: diag %d = %v, want %v", c.name, i, diags[i], c.diags[i])
				}
			}
		}
		if len(s.Callbacks) != c.ncb {
			t.Errorf("%s: %d callbacks, want %d", c.name, len(s.Callbacks), c.ncb)
		}
	}
}

func TestLineEndings(t *testing.T) {
	for _, eol := range []string{"\r\n", "\n", "\r"} {
		s, diags, err := Parse(strings.NewReader("CALLBACK WNDPROC A" + eol + "CALLBACK WNDPROC B" + eol + "CALLBACK WNDPROC C"))
		if err != nil || len(diags) != 0 {
			t.Fatalf("%q: %v %v", eol, err, diags)
		}
		if len(s.Callbacks) != 3 || s.Callbacks[2].Name != "C" || s.Callbacks[2].Line != 3 {
			t.Errorf("%q: %+v", eol, s.Callbacks)
		}
	}
}

func TestTemplatesAndConventions(t *testing.T) {
	src := "CALLBACK msgboxcallback M\r\nCALLBACK KeyboardProc K\r\nCALLBACK EnumChildProc E\r\n" +
		"USING CDECL\r\nCALLBACK WNDPROC W\r\nBEGIN CALLBACK C1 RETURNS VOID\r\nEND CALLBACK\r\nBEGIN CALLBACK C2 RETURNS VOID\r\nEND CALLBACK\r\n"
	s, diags, err := Parse(strings.NewReader(src))
	if err != nil || len(diags) != 0 {
		t.Fatalf("%v %v", err, diags)
	}
	get := func(name string) Callback {
		for _, cb := range s.Callbacks {
			if cb.Name == name {
				return cb
			}
		}
		t.Fatalf("%s missing", name)
		return Callback{}
	}
	if m := get("M"); m.Ret != Void || len(m.Params) != 1 || m.Conv != StdCall {
		t.Errorf("MSGBOXCALLBACK = %+v", m)
	}
	if k := get("K"); k.Ret != DWord || len(k.Params) != 3 {
		t.Errorf("KEYBOARDPROC = %+v", k)
	}
	if e := get("E"); e.Ret != Bool || len(e.Params) != 2 {
		t.Errorf("ENUMCHILDPROC = %+v", e)
	}
	// A template ignores USING and does not consume it; END CALLBACK consumes it.
	if get("W").Conv != StdCall || get("C1").Conv != CDecl || get("C2").Conv != StdCall {
		t.Errorf("conventions: W=%v C1=%v C2=%v", get("W").Conv, get("C1").Conv, get("C2").Conv)
	}
}

func TestKinds(t *testing.T) {
	for name, want := range map[string]Kind{"lpstr": DWord, "CHAR": Byte, "Int16": Word, "int64": QWord, "bool": Bool, "double": Double, "float": Float, "void": Void} {
		if k, ok := KindOf(name); !ok || k != want {
			t.Errorf("KindOf(%q) = %v %v", name, k, ok)
		}
	}
	if _, ok := KindOf("STRING"); ok {
		t.Error("KindOf(STRING) accepted")
	}
	if QWord.Size() != 8 || Double.Size() != 8 || Float.Size() != 4 || Byte.Size() != 4 {
		t.Error("Size")
	}
	if !Byte.IsInt() || !DWord.IsInt() || Bool.IsInt() {
		t.Error("IsInt")
	}
	if DWord.String() != "DWORD" || Kind(9).String() != "Kind(9)" || CDecl.String() != "cdecl" || StdCall.String() != "stdcall" {
		t.Error("String")
	}
	if (Diag{5, "Bad Syntax"}).String() != "line: 5  error: Bad Syntax" {
		t.Error("Diag.String")
	}
}
