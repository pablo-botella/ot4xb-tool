package cbk2obj

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// diffText reports the first differing line of got and want.
func diffText(t *testing.T, what string, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	g := strings.Split(string(got), "\r\n")
	w := strings.Split(string(want), "\r\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			t.Errorf("%s: line %d differs:\n got: %q\nwant: %q\n(%d lines got, %d lines want)", what, i+1, gl, wl, len(g), len(w))
			return
		}
	}
	t.Errorf("%s: byte difference without line difference (line ends?)", what)
}

// TestLegacyGolden reproduces the .asm XPPCBK.EXE 1.0.17 wrote for
// testdata/sample.cbk (seed 60046 = the seconds-since-midnight it used),
// defects included: the templates and the frame layout are the legacy ones.
func TestLegacyGolden(t *testing.T) {
	s := parseSample(t)
	u := generate(s, genOptions{legacy: true, seed: 60046})
	var buf bytes.Buffer
	if err := writeAsm(&buf, u); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/sample_legacy.asm")
	if err != nil {
		t.Fatal(err)
	}
	diffText(t, "legacy asm", buf.Bytes(), want)
}

// TestGolden pins the real output (fixed code, ot4xb helper names, symbol
// numbers from 1). CBK2OBJ_UPDATE=1 rewrites the golden file.
func TestGolden(t *testing.T) {
	s := parseSample(t)
	_, asm, err := Build(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/sample.asm"
	if os.Getenv("CBK2OBJ_UPDATE") != "" {
		if err := os.WriteFile(path, asm, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	diffText(t, "asm", asm, want)
}

// TestExterns checks the declared externals are exactly the called ones.
func TestExterns(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"CALLBACK WNDPROC W\r\n", "__retnl __conCallPa __conRelease __conPutNL _conGetLong"},
		{"BEGIN CALLBACK V RETURNS VOID\r\nEND CALLBACK\r\n", "__retnl __conCall __conNew __conRelease"},
		{"BEGIN CALLBACK D RETURNS DOUBLE\r\nPARAM FLOAT\r\nEND CALLBACK\r\n", "__retnl __conCallPa __conRelease __conPutND __conGetND _conPutFloat"},
		{"BEGIN CALLBACK Q RETURNS QWORD\r\nPARAM BOOL\r\nEND CALLBACK\r\n", "__retnl __conCallPa __conRelease __conPutL _conPutQWord _conGetQWord"},
		{"BEGIN CALLBACK F RETURNS FLOAT\r\nPARAM QWORD\r\nPARAM BYTE\r\nEND CALLBACK\r\n", "__retnl __conCallPa __conRelease __conPutNL _conPutFloat _conGetFloat _conPutQWord"},
	}
	for _, c := range cases {
		s, diags, err := Parse(strings.NewReader(c.src))
		if err != nil || len(diags) != 0 {
			t.Fatalf("%v %v", err, diags)
		}
		u := generate(s, genOptions{})
		if got := strings.Join(u.externs, " "); got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
		for _, x := range u.externs {
			if x == "__fltused" {
				t.Errorf("%q: __fltused declared", c.src)
			}
		}
	}
}

func TestLayout(t *testing.T) {
	cb := &Callback{Name: "X", Ret: QWord, Params: []Kind{QWord, Float, Word, Byte}, Conv: StdCall}
	f := layout(cb)
	if f.size != 28 || f.retVar != 8 || f.retCon != 12 || f.conBase != 28 || f.stack != 20 {
		t.Errorf("frame = %+v", f)
	}
	if got := f.paramPos; got[0] != 8 || got[1] != 16 || got[2] != 20 || got[3] != 24 {
		t.Errorf("paramPos = %v", got)
	}
	if got := f.conPos; got[0] != 28 || got[1] != 24 || got[2] != 20 || got[3] != 16 {
		t.Errorf("conPos = %v", got)
	}
	v := layout(&Callback{Name: "V", Ret: Void})
	if v.size != 4 || v.retVar != 0 || v.retCon != 4 || v.stack != 0 || v.conBase != 0 {
		t.Errorf("void frame = %+v", v)
	}
}
