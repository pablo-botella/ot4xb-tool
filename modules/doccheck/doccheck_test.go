package doccheck

import (
	"testing"

	"github.com/pablo-botella/ot4xb-tool/modules/scandoc"
	"github.com/pablo-botella/ot4xb-tool/modules/xbmac2h"
)

func hasMiss(ms []Miss, name string) bool {
	for _, m := range ms {
		if m.Name == name {
			return true
		}
	}
	return false
}

func TestCheck(t *testing.T) {
	files := []*scandoc.File{{
		Path: "x.cpp",
		Entities: []scandoc.Entity{
			{Kind: scandoc.KindFunction, Ident: "Foo", StartLine: 10},         // registered as FOO -> matched
			{Kind: scandoc.KindClass, Ident: "_LARGE_INTEGER_", StartLine: 2}, // class name IS a FUN registration
			{Kind: scandoc.KindCFunction, Ident: "_bar", StartLine: 30},       // matched by CDECL (case-sensitive)
			{Kind: scandoc.KindFunction, Ident: "Orphan", StartLine: 40},      // documented, not registered
		},
	}}
	mac := &xbmac2h.File{Lines: []xbmac2h.Line{
		{Kind: xbmac2h.Fun, Name: "FOO", Comment: "SRC: x.cpp"},             // matched (case-insensitive)
		{Kind: xbmac2h.Fun, Name: "_LARGE_INTEGER_", Comment: "SRC: n.cpp"}, // matched by the class doc
		{Kind: xbmac2h.Fun, Name: "GHOST", Comment: "SRC: g.cpp"},           // registered, undocumented
		{Kind: xbmac2h.CdeclExport, Name: "_bar", Comment: "SRC: x.cpp"},    // matched
		{Kind: xbmac2h.CdeclExport, Name: "_baz", Comment: "SRC: x.cpp"},    // registered C export, undocumented
	}}

	rep := Check(files, mac)

	if rep.NRegFun != 3 || rep.NRegC != 2 {
		t.Fatalf("reg counts: fun=%d c=%d", rep.NRegFun, rep.NRegC)
	}
	// undocumented: GHOST (function) and _baz (c-function); NOT FOO/_LARGE_INTEGER_/_bar
	if !hasMiss(rep.RegisteredUndocumented, "GHOST") || !hasMiss(rep.RegisteredUndocumented, "_baz") {
		t.Fatalf("undocumented = %+v", rep.RegisteredUndocumented)
	}
	if hasMiss(rep.RegisteredUndocumented, "FOO") || hasMiss(rep.RegisteredUndocumented, "_LARGE_INTEGER_") ||
		hasMiss(rep.RegisteredUndocumented, "_bar") {
		t.Fatalf("false undocumented: %+v", rep.RegisteredUndocumented)
	}
	// reverse: Orphan is a documented function with no registration; only functions
	// are reverse-checked (structures/c-functions use other registries).
	if len(rep.DocumentedUnregistered) != 1 || rep.DocumentedUnregistered[0].Name != "orphan" {
		t.Fatalf("documented-unregistered = %+v", rep.DocumentedUnregistered)
	}
}
