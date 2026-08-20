package xbmac2h

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = "// header comment\r\n_XPP_REG_WAPI( MessageBoxA )\r\n_XPP_REG_WST_(getVersion) // trailing comment\r\n  _XPP_REG_WMAC ( CreateWindow )  \r\n_XPP_REG_FUN_(MyFunc)\r\n\r\nSOMETHING_ELSE(x)\r\nplain text line\r\n_xpp_reg_wapi(lowercase)\r\n_CDECL_EXPORT_( _conGetLong )  // SRC: Container.cpp\r\n"

// Expected outputs: the Harbour xbmac2h output for the same input
// (01-analysis-xbmac2h.md) minus the comment line it copied verbatim, plus
// the _CDECL_EXPORT_ line, which the legacy tool did not know.
var (
	wantExports = toCRLF(`// ---------------------------------------------------------------------------
#ifdef __cplusplus
extern "C" {
#endif
// ---------------------------------------------------------------------------
XPPRET XPPENTRY wapi_MESSAGEBOXA(XppParamList );
XPPRET XPPENTRY wapist_GETVERSION(XppParamList );
XPPRET XPPENTRY wapimc_CREATEWINDOW(XppParamList );
XPPRET XPPENTRY MYFUNC(XppParamList );

//////////  UNKNOW LINE #7>>>SOMETHING_ELSE(x)<<<
//////////  UNKNOW LINE #8>>>plain text line<<<
XPPRET XPPENTRY wapi_LOWERCASE(XppParamList );
// _CDECL_EXPORT_( _conGetLong )
// ---------------------------------------------------------------------------
#ifdef __cplusplus
}
#endif
// ---------------------------------------------------------------------------
`)
	wantFuncList = toCRLF(`        {"MESSAGEBOXA",wapi_MESSAGEBOXA}
   ,    {"GETVERSION",wapist_GETVERSION}
   ,    {"CREATEWINDOW",wapimc_CREATEWINDOW}
   ,    {"MYFUNC",MYFUNC}

//////////  UNKNOW LINE #7>>>SOMETHING_ELSE(x)<<<
//////////  UNKNOW LINE #8>>>plain text line<<<
   ,    {"LOWERCASE",wapi_LOWERCASE}
// _CDECL_EXPORT_( _conGetLong )
`)
	wantCppDef = toCRLF(`LIBRARY mylib
EXPORTS
     MESSAGEBOXA =  wapi_MESSAGEBOXA  PRIVATE
     GETVERSION =  wapist_GETVERSION  PRIVATE
     CREATEWINDOW =  wapimc_CREATEWINDOW  PRIVATE
     MYFUNC  PRIVATE

;;;;;;;;;;  UNKNOW LINE #7>>>SOMETHING_ELSE(x)<<<
;;;;;;;;;;  UNKNOW LINE #8>>>plain text line<<<
     LOWERCASE =  wapi_LOWERCASE  PRIVATE
`)
	wantDef = toCRLF(`LIBRARY mylib
EXPORTS
     MESSAGEBOXA =  _wapi_MESSAGEBOXA
     GETVERSION =  _wapist_GETVERSION
     CREATEWINDOW =  _wapimc_CREATEWINDOW
     MYFUNC =  _MYFUNC

;;;;;;;;;;  UNKNOW LINE #7>>>SOMETHING_ELSE(x)<<<
;;;;;;;;;;  UNKNOW LINE #8>>>plain text line<<<
     LOWERCASE =  _wapi_LOWERCASE
     _conGetLong =  __conGetLong
`)
)

// toCRLF converts the LF raw literal (Go raw strings never contain CR) to CRLF.
func toCRLF(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

func TestWriters(t *testing.T) {
	f, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		write func(w *bytes.Buffer) error
		want  string
	}{
		{"exports", func(w *bytes.Buffer) error { return WriteExports(w, f) }, wantExports},
		{"funclist", func(w *bytes.Buffer) error { return WriteFuncList(w, f) }, wantFuncList},
		{"cppdef", func(w *bytes.Buffer) error { return WriteCppDef(w, f, "mylib") }, wantCppDef},
		{"def", func(w *bytes.Buffer) error { return WriteDef(w, f, "mylib") }, wantDef},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := c.write(&buf); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if buf.String() != c.want {
			t.Errorf("%s mismatch:\n--- got ---\n%s\n--- want ---\n%s", c.name, buf.String(), c.want)
		}
	}
}

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "proj", "mylib.xbm")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	paths, err := Generate(src, Options{Warn: func(s string) { warnings = append(warnings, s) }})
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{
		filepath.Join(dir, "proj", "mylib_xbexports.hpp"),
		filepath.Join(dir, "proj", "mylib_xbfunclist.hpp"),
		filepath.Join(dir, "proj", "mylibCpp.def"),
		filepath.Join(dir, "proj", "mylib.def"),
	}
	if strings.Join(paths, "|") != strings.Join(wantPaths, "|") {
		t.Errorf("paths = %q", paths)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "mylib.xbm:7") {
		t.Errorf("warnings = %q", warnings)
	}
	got, err := os.ReadFile(wantPaths[3])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != wantDef {
		t.Errorf(".def content mismatch:\n%s", got)
	}
	got, _ = os.ReadFile(wantPaths[0])
	if string(got) != wantExports {
		t.Errorf("exports content mismatch")
	}
}

func TestOutputPathsAndLibName(t *testing.T) {
	e, fl, c, d := OutputPaths(`C:\x\ot4xb.xbmac`)
	if e != `C:\x\ot4xb_xbexports.hpp` || fl != `C:\x\ot4xb_xbfunclist.hpp` || c != `C:\x\ot4xbCpp.def` || d != `C:\x\ot4xb.def` {
		t.Errorf("OutputPaths = %q %q %q %q", e, fl, c, d)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "ot4xb.xbmac")
	os.WriteFile(src, []byte("_XPP_REG_FUN_(A)\r\n"), 0o644)
	if _, err := Generate(src, Options{LibName: "ot4xb"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "ot4xb.def"))
	if !strings.HasPrefix(string(got), "LIBRARY ot4xb\r\nEXPORTS\r\n     A =  _A\r\n") {
		t.Errorf("def = %q", got)
	}
}
