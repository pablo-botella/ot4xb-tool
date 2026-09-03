package srcsplit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// crlf joins lines with CRLF and a trailing CRLF, the on-disk shape.
func crlf(lines ...string) []byte {
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\r\n")
	}
	return b.Bytes()
}

func TestStripRoundTrip(t *testing.T) {
	// A source with no doc blocks strips to itself, byte for byte.
	src := crlf("#xcommand FOO => bar", "#define X 1", "// note")
	got, doc := Strip(src)
	if !bytes.Equal(got, src) {
		t.Fatalf("strip changed a doc-less source:\n got %q\nwant %q", got, src)
	}
	if doc != 0 {
		t.Fatalf("docLines = %d, want 0", doc)
	}
}

func TestStripAndExtract(t *testing.T) {
	src := crlf(
		"#xcommand FOO => bar",
		"/*{{command: FOO | desc: does foo}}*/",
		"#define X 1",
	)
	// Strip removes the doc block, keeps the rest.
	got, doc := Strip(src)
	want := crlf("#xcommand FOO => bar", "#define X 1")
	if !bytes.Equal(got, want) {
		t.Fatalf("strip:\n got %q\nwant %q", got, want)
	}
	if doc != 1 {
		t.Fatalf("docLines = %d, want 1", doc)
	}
	// Extract keeps only the doc block.
	ex := Extract(src)
	if !bytes.Contains(ex, []byte("{{command: FOO")) || bytes.Contains(ex, []byte("#xcommand")) {
		t.Fatalf("extract = %q", ex)
	}
}

func TestEncode1252(t *testing.T) {
	// em-dash U+2014 -> 0x97, e-acute U+00E9 -> 0xE9, ASCII unchanged.
	got, err := encode1252("a—éb")
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{'a', 0x97, 0xE9, 'b'}; !bytes.Equal(got, want) {
		t.Fatalf("encode1252 = %v, want %v", got, want)
	}
	// A codepoint outside 1252 (CJK) is a hard error.
	if _, err := encode1252("中"); err == nil {
		t.Fatal("expected error for a non-1252 rune")
	}
}

// TestMixedEol is the data-loss bug that motivated moving to linereader: with a
// "\r\n"-only split, a bare-LF line glued onto the following doc block and was
// thrown away with it, and a CR-only line reached the .ch verbatim. Now every
// line end is a line end, no code is lost, and the outputs are pure CRLF.
func TestMixedEol(t *testing.T) {
	src := []byte("#define A 1\r\n" +
		"#define B 2\n" + // bare LF right before a doc block
		"/*{{command: FOO | desc: x}}*/\r\n" +
		"#define C 3\r" + // CR only
		"#define D 4\r\n")
	got, doc := Strip(src)
	want := []byte("#define A 1\r\n#define B 2\r\n#define C 3\r\n#define D 4\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("Strip =\n%q\nwant\n%q", got, want)
	}
	if doc != 1 {
		t.Fatalf("docLines = %d, want 1", doc)
	}
	if ex := Extract(src); !bytes.Equal(ex, []byte("/*{{command: FOO | desc: x}}*/\r\n")) {
		t.Fatalf("Extract = %q", ex)
	}
	// the non-CRLF lines are counted for the caller's warning
	if _, ends, bad := splitLines(src); !ends || bad != 2 {
		t.Fatalf("splitLines: endsWithEol=%v nonCRLF=%d, want true/2", ends, bad)
	}
	// a source with no trailing terminator keeps none (round-trip fidelity)
	if got, _ := Strip([]byte("A\r\nB")); !bytes.Equal(got, []byte("A\r\nB")) {
		t.Fatalf("no-trailing-eol Strip = %q", got)
	}
	// a doc-only source strips to nothing, exactly as before
	if got, _ := Strip([]byte("/*{{d}}*/\r\n")); len(got) != 0 {
		t.Fatalf("doc-only Strip = %q, want empty", got)
	}
}

// TestDstPolicy covers the destinations and the overwrite policy: '*' expands
// to the source base name, identical content is a no-op, a different existing
// file is refused without -bak/-force, kept as .bak with -bak, replaced with
// -force, and several sources need a '*' in every destination.
func TestDstPolicy(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0666); err != nil {
			t.Fatal(err)
		}
		return p
	}
	src := write("ot4xb.chsrc", []byte("#define A 1\r\n/*{{command: FOO | desc: x}}*/\r\n#define B 2\r\n"))
	code := filepath.Join(dir, "*.ch")
	doc := filepath.Join(dir, "doc", "*.chdoc")
	os.MkdirAll(filepath.Join(dir, "doc"), 0777)

	// first run: destinations expanded from the pattern, both written
	res, err := Run(src, Options{Code: code, Doc: doc})
	if err != nil || len(res) != 1 || !res[0].CodeWrote || !res[0].DocWrote {
		t.Fatalf("first run: %+v err=%v", res, err)
	}
	if filepath.Base(res[0].CodeDst) != "ot4xb.ch" || filepath.Base(res[0].DocDst) != "ot4xb.chdoc" {
		t.Fatalf("dst = %q / %q", res[0].CodeDst, res[0].DocDst)
	}
	// identical content: idempotent, nothing written, no policy involved
	res, err = Run(src, Options{Code: code, Doc: doc})
	if err != nil || res[0].CodeWrote || res[0].DocWrote {
		t.Fatalf("second run must be a no-op: %+v err=%v", res, err)
	}
	// the source changes: the destination now differs -> refused by default
	write("ot4xb.chsrc", []byte("#define A 1\r\n#define B 3\r\n"))
	if _, err = Run(src, Options{Code: code}); err == nil || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("overwrite must be refused without -bak/-force: %v", err)
	}
	if b, _ := os.ReadFile(res[0].CodeDst); !bytes.Contains(b, []byte("B 2")) {
		t.Fatal("refused overwrite must leave the file untouched")
	}
	// -check reports the drift and writes nothing
	res, err = Run(src, Options{Code: code, Check: true})
	if err != nil || !res[0].CodeDrift || res[0].CodeWrote {
		t.Fatalf("check: %+v err=%v", res, err)
	}
	// -bak keeps the previous bytes and writes the new ones
	res, err = Run(src, Options{Code: code, Bak: true})
	if err != nil || !res[0].CodeWrote || !res[0].CodeBacked {
		t.Fatalf("bak: %+v err=%v", res, err)
	}
	if b, _ := os.ReadFile(res[0].CodeDst + ".bak"); !bytes.Contains(b, []byte("B 2")) {
		t.Fatal(".bak must hold the previous content")
	}
	if b, _ := os.ReadFile(res[0].CodeDst); !bytes.Contains(b, []byte("B 3")) {
		t.Fatal("destination must hold the new content")
	}
	// -force overwrites with no copy
	write("ot4xb.chsrc", []byte("#define B 4\r\n"))
	if res, err = Run(src, Options{Code: code, Force: true}); err != nil || !res[0].CodeWrote || res[0].CodeBacked {
		t.Fatalf("force: %+v err=%v", res, err)
	}
	// several sources and a destination without '*' is an error
	write("other.chsrc", []byte("x\r\n"))
	if _, err = Run(filepath.Join(dir, "*.chsrc"), Options{Code: filepath.Join(dir, "single.ch")}); err == nil || !strings.Contains(err.Error(), "no '*'") {
		t.Fatalf("multi-source without '*' must fail: %v", err)
	}
	// a projection whose destination is not named is not produced: -code alone
	// yields clean sources and creates no doc file at all
	only := write("only.chsrc", []byte("#define Z 1\r\n/*{{command: Z}}*/\r\n"))
	res, err = Run(only, Options{Code: filepath.Join(dir, "only-*.ch")})
	if err != nil || res[0].DocDst != "" {
		t.Fatalf("code-only run: %+v err=%v", res, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc", "only.chdoc")); !os.IsNotExist(err) {
		t.Fatal("no doc destination was named, yet a doc file appeared")
	}
}

func TestEncodingDirective(t *testing.T) {
	if got := detectEncoding(crlf("/*{{encoding: UTF-8}}*/", "x")); got != "utf-8" {
		t.Fatalf("detectEncoding = %q, want utf-8", got)
	}
	if got := detectEncoding(crlf("no directive", "x")); got != "" {
		t.Fatalf("detectEncoding = %q, want empty", got)
	}
	// A UTF-8 source with the directive: the encoding marker is not doc (Extract
	// skips it), and Strip removes it like any doc block.
	src := crlf("/*{{encoding: utf-8}}*/", "#xcommand FOO => —", "/*{{command: FOO | desc: x}}*/")
	if got, _ := Strip(src); bytes.Contains(got, []byte("encoding")) {
		t.Fatalf("strip kept the encoding marker: %q", got)
	}
	if ex := Extract(src); bytes.Contains(ex, []byte("encoding")) {
		t.Fatalf("extract kept the encoding marker: %q", ex)
	}
}
