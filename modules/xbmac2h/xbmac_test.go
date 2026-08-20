package xbmac2h

import (
	"strings"
	"testing"
)

const sampleList = "// header comment\r\n_XPP_REG_WAPI( MessageBoxA )\r\n_XPP_REG_WST_(getVersion) // trailing comment\r\n  _XPP_REG_WMAC ( CreateWindow )  \r\n_XPP_REG_FUN_(MyFunc)\r\n\r\nSOMETHING_ELSE(x)\r\nplain text line\r\n_xpp_reg_wapi(lowercase)\r\n\t_XPP_REG_FUN_(\tTabbed\t)\t// SRC: x.cpp\r\n_cdecl_export_( _conGetLong ) // SRC: Container.cpp\r\n"

func TestParse(t *testing.T) {
	f, err := Parse(strings.NewReader(sampleList))
	if err != nil {
		t.Fatal(err)
	}
	want := []Line{
		{N: 1, Kind: Comment, Comment: "header comment"},
		{N: 2, Kind: Wapi, Name: "MESSAGEBOXA"},
		{N: 3, Kind: Wst, Name: "GETVERSION", Comment: "trailing comment"},
		{N: 4, Kind: Wmac, Name: "CREATEWINDOW"},
		{N: 5, Kind: Fun, Name: "MYFUNC"},
		{N: 6, Kind: Blank},
		{N: 7, Kind: Unknown},
		{N: 8, Kind: Unknown},
		{N: 9, Kind: Wapi, Name: "LOWERCASE"},
		{N: 10, Kind: Fun, Name: "TABBED", Comment: "SRC: x.cpp"},
		{N: 11, Kind: CdeclExport, Name: "_conGetLong", Comment: "SRC: Container.cpp"},
	}
	if len(f.Lines) != len(want) {
		t.Fatalf("got %d lines: %+v", len(f.Lines), f.Lines)
	}
	for i, w := range want {
		g := f.Lines[i]
		if g.N != w.N || g.Kind != w.Kind || g.Name != w.Name || g.Comment != w.Comment {
			t.Errorf("line %d = %+v, want %+v", i+1, g, w)
		}
	}
	if f.Lines[6].Text != "SOMETHING_ELSE(x)" {
		t.Errorf("unknown text = %q", f.Lines[6].Text)
	}
	if got := f.Lines[2].Symbol(); got != "wapist_GETVERSION" {
		t.Errorf("Symbol = %q", got)
	}
	if got := f.Lines[4].Symbol(); got != "MYFUNC" {
		t.Errorf("Symbol = %q", got)
	}
	if len(f.Commands()) != 6 || len(f.Unknowns()) != 2 {
		t.Errorf("Commands=%d Unknowns=%d", len(f.Commands()), len(f.Unknowns()))
	}
	if got := f.Lines[10].Symbol(); got != "_conGetLong" || f.Lines[10].Kind.IsCommand() {
		t.Errorf("CdeclExport: Symbol=%q IsCommand=%v", got, f.Lines[10].Kind.IsCommand())
	}
}

func TestLineEndings(t *testing.T) {
	for _, eol := range []string{"\r\n", "\n", "\r"} {
		f, err := Parse(strings.NewReader("_XPP_REG_FUN_(A)" + eol + "_XPP_REG_FUN_(B)" + eol + "_XPP_REG_FUN_(C)"))
		if err != nil {
			t.Fatal(err)
		}
		if len(f.Lines) != 3 || f.Lines[2].Name != "C" || f.Lines[2].N != 3 {
			t.Errorf("%q: %+v", eol, f.Lines)
		}
	}
}

func TestKindStrings(t *testing.T) {
	if Fun.String() != "Fun" || Kind(99).String() != "Kind(99)" || !Wmac.IsCommand() || Comment.IsCommand() {
		t.Errorf("Kind helpers")
	}
	if CdeclExport.String() != "CdeclExport" || CdeclExport.IsCommand() || CdeclExport.Prefix() != "" {
		t.Errorf("CdeclExport helpers")
	}
}
