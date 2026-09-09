package site

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUsage: with nothing to work from, the caller is the one that has to
// complain - Run says so with ErrUsage instead of inventing a database.
func TestUsage(t *testing.T) {
	if err := Run(Options{}); !errors.Is(err, ErrUsage) {
		t.Fatalf("got %v", err)
	}
	if err := Run(Options{DB: "x.db"}); !errors.Is(err, ErrUsage) {
		t.Fatalf("an output folder is required too: %v", err)
	}
	if err := Run(Options{Out: "out"}); !errors.Is(err, ErrUsage) {
		t.Fatalf("a database is required too: %v", err)
	}
}

// TestSiteDefFills checks the resolution order: the .site-def completes what
// the options leave empty, and an option always wins.
func TestSiteDefFills(t *testing.T) {
	dir := t.TempDir()
	site := filepath.Join(dir, "x.site-def")
	os.WriteFile(site, []byte(`{"db": "./from-def.db", "out": "./from-def", "title": "T"}`), 0o644)

	// nothing else given: both come from the .site-def. Run gets past the usage
	// check and reaches the database - which docdb creates, so the file naming
	// it is the proof that the .site-def was read.
	err := Run(Options{Site: site})
	if err == nil || errors.Is(err, ErrUsage) {
		t.Fatalf("the .site-def must fill db and out: %v", err)
	}
	if _, e := os.Stat(filepath.Join(dir, "from-def.db")); e != nil {
		t.Fatalf("the database of the .site-def was not used: %v", e)
	}

	// an explicit DB wins over the one of the .site-def
	mine := filepath.Join(dir, "mine.db")
	if err = Run(Options{Site: site, DB: mine}); err == nil || errors.Is(err, ErrUsage) {
		t.Fatalf("got %v", err)
	}
	if _, e := os.Stat(mine); e != nil {
		t.Fatalf("the option did not win: %v", e)
	}
}

// TestExportTemplates writes the built-in templates and, with no output asked
// for, stops there.
func TestExportTemplates(t *testing.T) {
	dir := t.TempDir()
	var logged []string
	if err := Run(Options{Export: dir, Log: func(m string) { logged = append(logged, m) }}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"page.html", "index.html", "style.css"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s not exported: %v", n, err)
		}
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "template(s) written") {
		t.Fatalf("log: %v", logged)
	}
	// asked to export and to generate: the usage check still applies
	if err := Run(Options{Export: dir, Out: filepath.Join(dir, "o")}); !errors.Is(err, ErrUsage) {
		t.Fatalf("got %v", err)
	}
}
