package compile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandSourcesOrder(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"b.cpp", "a.cpp", "x.h", "ch/z.ch", "ch/a.ch", "ch/notes.txt"} {
		p := filepath.Join(dir, n)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	var warns []string
	got, err := ExpandSources([]string{
		filepath.Join(dir, "missing/*.*"),
		filepath.Join(dir, "b.cpp"),
		filepath.Join(dir, "*.cpp"),
		filepath.Join(dir, "ch/z.ch"),
		filepath.Join(dir, "ch/*.*"),
		filepath.Join(dir, "*.h"),
	}, func(m string) { warns = append(warns, m) })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"b.cpp", "a.cpp", "ch/z.ch", "ch/a.ch", "ch/notes.txt", "x.h"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if filepath.ToSlash(got[i]) != filepath.ToSlash(filepath.Join(dir, want[i])) {
			t.Fatalf("at %d: got %s, want %s (all: %v)", i, got[i], want[i], got)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("warnings: %v", warns)
	}
	if _, err := ExpandSources([]string{filepath.Join(dir, "missing/*.*")}, nil); err == nil {
		t.Fatal("nil warn must make an empty pattern an error")
	}
}
