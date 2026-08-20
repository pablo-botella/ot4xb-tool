package vbuild

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseHeader(t *testing.T) {
	ok := []struct {
		in      string
		product string
		v       Version
		noinc   bool
	}{
		{"[ot4xb.dll]  Version:{1,7,14,0}", "ot4xb.dll", Version{1, 7, 14, 0}, false},
		{"[P] Version:{1,2,3,4}", "P", Version{1, 2, 3, 4}, false},
		{"[P]Version:{1,2,3,4}", "P", Version{1, 2, 3, 4}, false},
		{"  [P]   version:{1,2,3,4}", "P", Version{1, 2, 3, 4}, false},
		{"[Two Words]  VERSION:{0,0,0,255}", "Two Words", Version{0, 0, 0, 255}, false},
		{"[a[b]c]  Version:{001,2,3,4}", "a[b]c", Version{1, 2, 3, 4}, false},
		{"[Prod:noinc]  Version:{1,2,3,4}", "Prod", Version{1, 2, 3, 4}, true},
		{"[Prod:NOINC]  Version:{1,2,3,4}", "Prod", Version{1, 2, 3, 4}, true},
	}
	for _, c := range ok {
		h, err := ParseHeader([]byte(c.in))
		if err != nil {
			t.Errorf("ParseHeader(%q): %v", c.in, err)
			continue
		}
		if h.Product != c.product || h.Version != c.v || h.NoInc != c.noinc {
			t.Errorf("ParseHeader(%q) = %+v", c.in, h)
		}
	}
	bad := []struct {
		in  string
		err error
	}{
		{"", ErrNoHeader},
		{"garbage", ErrNoHeader},
		{"[P]  Version:{1,2,3,4} ", ErrNoHeader},
		{"[P]  Version:{1, 2,3,4}", ErrNoHeader},
		{"[P]  Version:{1,2,3}", ErrNoHeader},
		{"[P]  Version:{1,2,3,4,5}", ErrNoHeader},
		{"P]  Version:{1,2,3,4}", ErrNoHeader},
		{"[]  Version:{1,2,3,4}", ErrNoHeader},
		{"[P]  Version:{1,2,3,300}", ErrRange},
		{"[P]  Version:{256,2,3,4}", ErrRange},
	}
	for _, c := range bad {
		_, err := ParseHeader([]byte(c.in))
		if !errors.Is(err, c.err) {
			t.Errorf("ParseHeader(%q) err = %v, want %v", c.in, err, c.err)
		}
	}
}

func TestHeaderFormat(t *testing.T) {
	h := Header{Product: "My Prod", Version: Version{1, 2, 3, 5}}
	if h.Format() != "[My Prod]  Version:{1,2,3,5}" {
		t.Errorf("Format = %q", h.Format())
	}
	h.NoInc = true
	if h.Format() != "[My Prod:noinc]  Version:{1,2,3,5}" {
		t.Errorf("Format noinc = %q", h.Format())
	}
}

func TestInc(t *testing.T) {
	cases := []struct {
		in   Version
		c    Component
		want Version
		err  error
	}{
		{Version{1, 2, 3, 4}, NoInc, Version{1, 2, 3, 4}, nil},
		{Version{1, 2, 3, 4}, LBuild, Version{1, 2, 3, 5}, nil},
		{Version{1, 2, 3, 4}, HBuild, Version{1, 2, 4, 0}, nil},
		{Version{1, 2, 3, 4}, Minor, Version{1, 3, 0, 0}, nil},
		{Version{1, 2, 3, 4}, Major, Version{2, 0, 0, 0}, nil},
		{Version{1, 2, 3, 255}, LBuild, Version{1, 2, 4, 0}, nil},
		{Version{1, 2, 255, 255}, LBuild, Version{1, 3, 0, 0}, nil},
		{Version{1, 255, 255, 255}, LBuild, Version{2, 0, 0, 0}, nil},
		{Version{255, 255, 255, 255}, LBuild, Version{255, 255, 255, 255}, ErrOverflow},
		{Version{255, 0, 0, 0}, Major, Version{255, 0, 0, 0}, ErrOverflow},
	}
	for _, c := range cases {
		got, err := c.in.Inc(c.c)
		if !errors.Is(err, c.err) || (err == nil && got != c.want) {
			t.Errorf("%v.Inc(%d) = %v, %v; want %v, %v", c.in, c.c, got, err, c.want, c.err)
		}
	}
	if (Version{1, 7, 14, 0}).Build() != 14*256 {
		t.Errorf("Build()")
	}
}

func TestParseComponent(t *testing.T) {
	for name, want := range map[string]Component{"major": Major, "MINOR": Minor, "hbuild": HBuild, "lbuild": LBuild, "Build": LBuild} {
		got, err := ParseComponent(name)
		if err != nil || got != want {
			t.Errorf("ParseComponent(%q) = %v, %v", name, got, err)
		}
	}
	if _, err := ParseComponent("patch"); err == nil {
		t.Errorf("unknown component must fail")
	}
}

func ctx(v Version) *Context {
	loc := time.FixedZone("CEST", 2*3600)
	return &Context{
		Version: v,
		Now:     time.Date(2026, 8, 19, 16, 25, 37, 0, loc),
		UUID:    "7e95207205154948ae833f3086c4e5e0",
		Folder:  "sub",
	}
}

// The expansions verified against the original tool (dev-tools go-port
// analysis §3.3), SYSTEMTIME(19) fixed.
func TestExpand(t *testing.T) {
	c := ctx(Version{1, 2, 3, 5})
	cases := []struct{ in, want string }{
		{"ver={$<FILEVERSION(,,,)>$}", "ver=1,2,3,5"},
		{"dot={$<FILEVERSION(...)>$}", "dot=1.2.3.5"},
		{"z3={$<FILEVERSION(000,000,000,000)>$}", "z3=001,002,003,005"},
		{"u={$<FILEVERSION(_._._._)>$}", "u=001_002_003_005"},
		{"d3={$<FILEVERSION(000.000.000.000)>$}", "d3=001.002.003.005"},
		{"lt={$<LOCALTIME(YYYY)>$}", "lt=2026"},
		{"{$<LOCALTIME(YYYYMMDD)>$}", "20260819"},
		{"{$<LOCALTIME(14)>$}", "20260819162537"},
		{"{$<SYSTEMTIME(14)>$}", "20260819142537"},
		{"[{$<LOCALTIME(19)>$}]", "[2026-08-19 16:25:37]"},
		{"[{$<LOCALTIME(19+z)>$}]", "[2026-08-19 16:25:37 +0200]"},
		{"[{$<SYSTEMTIME(19)>$}]", "[2026-08-19 14:25:37]"},
		{"uuid={$<UUID>$} again={$<UUID>$}", "uuid=7e95207205154948ae833f3086c4e5e0 again=7e95207205154948ae833f3086c4e5e0"},
		{"fld={$<fld>$}", "fld=sub"},
		{"lower={$<fileversion(...)>$}", "lower=1.2.3.5"},
		{"spaced={$<FILEVERSION( ,,,)>$}", "spaced={$<FILEVERSION( ,,,)>$}"}, // exact spelling
		{"unknown={$<NOPE>$}", "unknown={$<NOPE>$}"},
		{"open={$<FILEVERSION(,,,", "open={$<FILEVERSION(,,,"},
		{"none here", "none here"},
	}
	for _, cse := range cases {
		got := string(Expand([]byte(cse.in), c))
		if got != cse.want {
			t.Errorf("Expand(%q) = %q, want %q", cse.in, got, cse.want)
		}
	}
}

func TestExpandMask(t *testing.T) {
	c := ctx(Version{1, 7, 14, 0}) // build = 3584
	cases := []struct{ in, want string }{
		{"{$<FILEVERSION(:<maj>.<min>.<build>:)>$}", "1.7.3584"},
		{"{$<FILEVERSION(:version: <maj>.<min> build: <build(05)>:)>$}", "version: 1.7 build: 03584"},
		{"{$<FILEVERSION(:<maj(02)>_<min(02)>_<hbuild(02)>_<lbuild(02)>:)>$}", "01_07_14_00"},
		{"{$<FILEVERSION(:v<maj>.<min> (<build>):)>$}", "v1.7 (3584)"},
		{"{$<FILEVERSION(:<maj(3)>|<min(3)>:)>$}", "  1|  7"},
		{"{$<FILEVERSION(:<word> stays, <maj> not:)>$}", "<word> stays, 1 not"},
		{"{$<FILEVERSION(:<build:)>$}", "<build"},    // unclosed component: literal
		{"{$<fileversion(:B=<BUILD>:)>$}", "B=3584"}, // names case-insensitive
		{"x{$<FILEVERSION(:<maj>:)>$}y", "x1y"},
	}
	for _, cse := range cases {
		got := string(Expand([]byte(cse.in), c))
		if got != cse.want {
			t.Errorf("Expand(%q) = %q, want %q", cse.in, got, cse.want)
		}
	}
}

const sampleScript = "[MyProd]  Version:{1,2,3,4}\r\n" +
	"free notes, no file yet {$<FILEVERSION(...)>$}\r\n" +
	"{$<File:>$[out/a.txt]$}\r\n" +
	"A ver={$<FILEVERSION(,,,)>$} fld={$<fld>$}\r\n" +
	"  {$<File:>$[  b.txt  ]$}  \r\n" +
	"B1\r\n" +
	"\r\n" +
	"B3 last"

func writeScript(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "sub", "test.VersionInfo")
	if err := os.MkdirAll(filepath.Join(dir, "sub", "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunNoInc(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, dir, sampleScript)
	res, err := Run(p, Options{UUID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.New != (Version{1, 2, 3, 4}) {
		t.Errorf("res = %+v", res)
	}
	// script untouched
	b, _ := os.ReadFile(p)
	if string(b) != sampleScript {
		t.Errorf("script rewritten without change")
	}
	// generated files
	a, _ := os.ReadFile(filepath.Join(dir, "sub", "out", "a.txt"))
	if string(a) != "A ver=1,2,3,4 fld=sub\r\n" {
		t.Errorf("a.txt = %q", a)
	}
	// the marker path "  b.txt  " is trimmed (vbuild.md: blanks around the
	// path are removed)
	bt, _ := os.ReadFile(filepath.Join(dir, "sub", "b.txt"))
	if string(bt) != "B1\r\n\r\nB3 last\r\n" {
		t.Errorf("b.txt = %q", bt)
	}
	if len(res.Files) != 2 {
		t.Errorf("files = %v", res.Files)
	}
}

func TestRunInc(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, dir, sampleScript)
	var printed []string
	res, err := Run(p, Options{Inc: LBuild, UUID: "u", Print: func(s string) { printed = append(printed, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.New != (Version{1, 2, 3, 5}) || res.Header != "[MyProd]  Version:{1,2,3,5}" {
		t.Errorf("res = %+v", res)
	}
	if len(printed) != 1 || printed[0] != res.Header {
		t.Errorf("printed = %v", printed)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "[MyProd]  Version:{1,2,3,5}\r\n") {
		t.Errorf("header not rewritten: %q", b[:40])
	}
	if !strings.HasSuffix(string(b), "B3 last") {
		t.Errorf("body altered")
	}
	a, _ := os.ReadFile(filepath.Join(dir, "sub", "out", "a.txt"))
	if string(a) != "A ver=1,2,3,5 fld=sub\r\n" {
		t.Errorf("a.txt = %q", a)
	}
}

func TestRunNoIncHeader(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, dir, "[P:noinc]  Version:{1,2,3,4}\r\n{$<File:>$[o.txt]$}\r\nv={$<FILEVERSION(,,,)>$}\r\n")
	res, err := Run(p, Options{Inc: LBuild})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || res.New != (Version{1, 2, 3, 4}) {
		t.Errorf("noinc header must freeze the version: %+v", res)
	}
	o, _ := os.ReadFile(filepath.Join(dir, "sub", "o.txt"))
	if string(o) != "v=1,2,3,4\r\n" {
		t.Errorf("o.txt = %q", o)
	}
}

func TestRunOverflow(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, dir, "[P]  Version:{255,255,255,255}\r\nbody\r\n")
	if _, err := Run(p, Options{Inc: LBuild}); !errors.Is(err, ErrOverflow) {
		t.Errorf("err = %v", err)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "[P]  Version:{255,255,255,255}") {
		t.Errorf("script must stay untouched on overflow")
	}
}

func TestRunEols(t *testing.T) {
	dir := t.TempDir()
	mixed := "[P]  Version:{1,2,3,4}\r\n{$<File:>$[o.txt]$}\nl1\rl2\r\nl3"
	cases := []struct {
		eol  OutEol
		want string
	}{
		{OutCrLf, "l1\r\nl2\r\nl3\r\n"},
		{OutLf, "l1\nl2\nl3\n"},
		{OutCr, "l1\rl2\rl3\r"},
		{OutSave, "l1\rl2\r\nl3"}, // save: each line keeps its own; last had none
	}
	for _, c := range cases {
		p := writeScript(t, dir, mixed)
		if _, err := Run(p, Options{Eol: c.eol}); err != nil {
			t.Fatal(err)
		}
		o, _ := os.ReadFile(filepath.Join(dir, "sub", "o.txt"))
		if string(o) != c.want {
			t.Errorf("eol %v: %q, want %q", c.eol, o, c.want)
		}
	}
}

func TestRunFirstBodyLineProcessed(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, dir, "[P]  Version:{1,2,3,4}\r\n{$<File:>$[o.txt]$}\r\nx\r\n")
	if _, err := Run(p, Options{}); err != nil {
		t.Fatal(err)
	}
	o, err := os.ReadFile(filepath.Join(dir, "sub", "o.txt"))
	if err != nil || string(o) != "x\r\n" {
		t.Errorf("marker on body line 1 must work: %q, %v", o, err)
	}
}

func TestRunBadHeader(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, dir, "garbage\r\nbody\r\n")
	if _, err := Run(p, Options{}); !errors.Is(err, ErrNoHeader) {
		t.Errorf("err = %v", err)
	}
}

func TestRunMacroInMarker(t *testing.T) {
	dir := t.TempDir()
	p := writeScript(t, dir, "[P]  Version:{1,2,3,4}\r\n{$<File:>$[out/{$<fld>$}_{$<FILEVERSION(_._._._)>$}.txt]$}\r\ncontent\r\n")
	res, err := Run(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "sub", "out", "sub_001_002_003_004.txt")
	if len(res.Files) != 1 || res.Files[0] != want {
		t.Errorf("files = %v, want %s", res.Files, want)
	}
}
