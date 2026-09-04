package artefacts

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree builds a small project-like tree and returns its root.
func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"ot4xb.sln":                "sln",
		"source/a.cpp":             "a",
		"source/b.cpp":             "b",
		"source/CRC32.HPP":         "crc",
		"source/notes.txt":         "n",
		"source/ot4xb.VersionInfo": "[x]  Version:{1,2,3,4}\r\n",
		"source/ch/one.ch":         "1",
		"source/ch/two.CH":         "2",
		"Release/ot4xb.dll":        "dll",
		"Release/ot4xb.lib":        "lib",
		"Release/ot4xb_cpp.lib":    "cpplib",
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGlob(t *testing.T) {
	dir := tree(t)
	j := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	cases := []struct {
		pattern string
		want    []string
	}{
		{"source/*.cpp", []string{"source/a.cpp", "source/b.cpp"}},
		{"source/*.hpp", []string{"source/CRC32.HPP"}}, // case-insensitive
		{"source/*.versioninfo", []string{"source/ot4xb.VersionInfo"}},
		{"source/ch/*.ch", []string{"source/ch/one.ch", "source/ch/two.CH"}},
		{"source/*.xyz", nil},
		{"missingdir/*.c", nil},
		{"ot4xb.sln", []string{"ot4xb.sln"}}, // no wildcard: literal, must exist
		{"nope.sln", nil},
	}
	for _, c := range cases {
		got, err := Glob(j(c.pattern))
		if err != nil {
			t.Fatalf("Glob(%s): %v", c.pattern, err)
		}
		var want []string
		for _, w := range c.want {
			want = append(want, j(w))
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("Glob(%s) = %v, want %v", c.pattern, got, want)
		}
	}
	if _, err := Glob(j("sour*/x.c")); err == nil {
		t.Errorf("wildcard in a folder element must fail")
	}
}

func TestZip(t *testing.T) {
	dir := tree(t)
	j := func(p string) string { return filepath.Join(dir, filepath.FromSlash(p)) }
	var warns []string
	zipPath := j("Release/builds/_x/pack.zip") // parent folders do not exist yet
	names, err := Zip(zipPath, []Content{
		{In: []string{j("Release/*.lib"), j("Release/ot4xb.dll"), j("source/*.txt")}, Out: "/"},
		{In: []string{j("source/ch/*.ch")}, Out: "/include"},
		{In: []string{j("source/*.nothing")}, Out: "/"},
	}, Options{Warn: func(m string) { warns = append(warns, m) }})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ot4xb.lib", "ot4xb_cpp.lib", "ot4xb.dll", "notes.txt", "include/one.ch", "include/two.CH"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Errorf("names = %v", names)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "no files match") {
		t.Errorf("warns = %v", warns)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	got := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b := make([]byte, 16)
		n, _ := rc.Read(b)
		rc.Close()
		got[f.Name] = string(b[:n])
	}
	if got["ot4xb.dll"] != "dll" || got["include/one.ch"] != "1" || got["notes.txt"] != "n" {
		t.Errorf("zip contents = %v", got)
	}
}

func TestZipDuplicates(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	os.Mkdir(a, 0o755)
	os.Mkdir(b, 0o755)
	os.WriteFile(filepath.Join(a, "same.txt"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(b, "SAME.TXT"), []byte("B"), 0o644)
	var warns []string
	names, err := Zip(filepath.Join(dir, "out.zip"), []Content{
		{In: []string{filepath.Join(a, "*.txt"), filepath.Join(b, "*.txt")}, Out: "/"},
	}, Options{Warn: func(m string) { warns = append(warns, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "same.txt" {
		t.Errorf("names = %v", names)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "duplicate zip entry") {
		t.Errorf("warns = %v", warns)
	}
}

func TestZipMissingLiteralFile(t *testing.T) {
	dir := t.TempDir()
	var warns []string
	names, err := Zip(filepath.Join(dir, "out.zip"), []Content{
		{In: []string{filepath.Join(dir, "missing.dll")}, Out: "/"},
	}, Options{Warn: func(m string) { warns = append(warns, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 || len(warns) != 1 {
		t.Errorf("names=%v warns=%v", names, warns)
	}
}

func TestZipCleanDocComments(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.ch")
	os.WriteFile(src, []byte("/*{{ topic: x | desc: y }}*/\r\n#define A 1\r\n/*{{begin-topic}}*/\r\n/*{{topic_: t }}*/\r\n/*{{end-topic}}*/\r\n#define B 2\r\n"), 0o644)
	bin := filepath.Join(dir, "b.txt")
	os.WriteFile(bin, []byte("no markers\n"), 0o644)
	zp := filepath.Join(dir, "out.zip")
	if _, err := Zip(zp, []Content{{In: []string{src, bin}, Out: "/", Clean: true}}, Options{}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(zp)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got := map[string]string{}
	for _, f := range r.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[f.Name] = string(b)
	}
	if got["a.ch"] != "#define A 1\r\n#define B 2\r\n" {
		t.Fatalf("a.ch: %q", got["a.ch"])
	}
	if got["b.txt"] != "no markers\n" {
		t.Fatalf("b.txt untouched: %q", got["b.txt"])
	}
}
