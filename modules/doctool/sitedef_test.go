package doctool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.site-def")
	os.WriteFile(p, []byte(`{"doc_tool": "../d.doc-tool", "db": "./x.db", "templates": "./tpl", "assets": "./a", "out": "./out", "title": "T", "gencss": true}`), 0o644)
	c, err := LoadSite(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "T" || c.OutDir() != filepath.Join(dir, "out") || c.TemplatesDir() != filepath.Join(dir, "tpl") || c.AssetsDir() != filepath.Join(dir, "a") || c.DBPath() != filepath.Join(dir, "x.db") || c.DocToolPath() != filepath.Join(filepath.Dir(dir), "d.doc-tool") || !c.GenCSS {
		t.Errorf("%+v %s %s", c, c.OutDir(), c.TemplatesDir())
	}
	os.WriteFile(p, []byte(`{"title": "T"}`), 0o644)
	if _, err := LoadSite(p); err == nil || !strings.Contains(err.Error(), "no out folder") {
		t.Errorf("missing out: %v", err)
	}
	os.WriteFile(p, []byte(`{"out": "./o"}`), 0o644)
	if c, err := LoadSite(p); err != nil || c.TemplatesDir() != "" {
		t.Errorf("no templates: %v %q", err, c.TemplatesDir())
	}
}

// TestLoadSiteUnknownKey: a key this build does not know has to be an error
// naming it. Dropped in silence it looks like the option had no effect, which
// is exactly what a typo and a stale binary both look like.
func TestLoadSiteUnknownKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.site-def")

	os.WriteFile(p, []byte(`{"out": "./o", "clean_url": true}`), 0o644)
	_, err := LoadSite(p)
	if err == nil || !strings.Contains(err.Error(), "clean_url") {
		t.Errorf("a typo must name the key, got %v", err)
	}
	// the right one still loads
	os.WriteFile(p, []byte(`{"out": "./o", "clean_urls": true}`), 0o644)
	c, err := LoadSite(p)
	if err != nil || !c.CleanURLs {
		t.Errorf("clean_urls: %v %+v", err, c)
	}
	// and so does an unknown key inside the sitemap object
	os.WriteFile(p, []byte(`{"out": "./o", "sitemap": {"base": "https://x.test/", "prioriti": "0.5"}}`), 0o644)
	if _, err := LoadSite(p); err == nil || !strings.Contains(err.Error(), "prioriti") {
		t.Errorf("nested typo: %v", err)
	}
	// two values in the file are still refused
	os.WriteFile(p, []byte(`{"out": "./o"}{"out": "./p"}`), 0o644)
	if _, err := LoadSite(p); err == nil {
		t.Error("a second JSON value must be an error")
	}
}
