package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/ot4xb-tool/modules/doctool/gen"
)

// entries builds a page body from labelled values in the subset.
func entries(t *testing.T, labelled ...string) []gen.Entry {
	var out []gen.Entry
	for i := 0; i+1 < len(labelled); i += 2 {
		blocks, issues := gen.ParseText(labelled[i+1])
		if len(issues) > 0 {
			t.Fatalf("%q: %v", labelled[i+1], issues)
		}
		out = append(out, gen.Entry{Label: labelled[i], Blocks: blocks})
	}
	return out
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	pages := []gen.Page{
		{File: "function-foo.md", Title: "Foo", Kind: "function", Books: []string{"xbase"}, Categories: []string{"string"},
			Short: "Does foo.", Keywords: []string{"foo", "bar"}, Source: "source/x.cpp:12",
			Body: entries(t,
				"", "# Foo",
				"desc", "Does foo with _OT4XB_X_ and <cMethod> & `char**`. See [Bar](function-bar.md).",
				"params", "- `a` first\n- `b` second",
				"example", "```xbase\n? \"<hi>\"\n```",
				"", "{{begin-md}}\n| a | b |\n|---|---|\n| 1 | [x](function-bar.md) |\n{{end-md}}")},
		{File: "index.md", Title: "Index", Kind: "index", Body: entries(t, "", "# Index\n\n- [Foo](function-foo.md)")},
	}
	if err := Write(pages, dir, Options{Title: "Test site"}); err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(dir, "function-foo.html"))
	for _, want := range []string{
		"<title>Foo - Test site</title>",
		"<h1 id=\"foo\">Foo</h1>\n",
		"<p><strong>desc:</strong> Does foo with _OT4XB_X_ and &lt;cMethod&gt; &amp; <code>char**</code>. See <a href=\"function-bar.html\">Bar</a>.</p>\n",
		"<p><strong>params:</strong></p>\n<ul>\n<li><code>a</code> first</li>\n<li><code>b</code> second</li>\n</ul>\n",
		"<p><strong>example:</strong></p>\n<pre><code class=\"language-xbase\">? &#34;&lt;hi&gt;&#34;</code></pre>\n",
		"<table>", `<a href="function-bar.html">x</a>`,
		"Categories: string", "Books: xbase", `name="keywords" content="foo, bar"`,
		`name="description" content="Does foo."`, `rel="canonical" href="function-foo.html"`, `href="index.html"`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(string(page), "<em>") {
		t.Errorf("an <em> in the page:\n%s", page)
	}
	idx, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if !strings.Contains(string(idx), `<li><a href="function-foo.html">Foo</a></li>`) {
		t.Errorf("index:\n%s", idx)
	}
	// only the pages are written
	got, _ := os.ReadDir(dir)
	if len(got) != 2 {
		t.Errorf("wrote %d files, want the 2 pages", len(got))
	}
	// GenCSS and an assets folder
	assets := filepath.Join(dir, "assets")
	os.MkdirAll(filepath.Join(assets, "img"), 0o755)
	os.WriteFile(filepath.Join(assets, "robots.txt"), []byte("User-agent: *\n"), 0o644)
	os.WriteFile(filepath.Join(assets, "img", "logo.svg"), []byte("<svg/>"), 0o644)
	site := filepath.Join(dir, "site")
	if err := Write(pages, site, Options{Title: "T", Assets: assets, GenCSS: true}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"style.css", "robots.txt", filepath.Join("img", "logo.svg")} {
		if _, err := os.Stat(filepath.Join(site, f)); err != nil {
			t.Errorf("%s missing", f)
		}
	}
	// an override, and a sitemap
	tdir := filepath.Join(dir, "tpl")
	os.MkdirAll(tdir, 0o755)
	os.WriteFile(filepath.Join(tdir, "page.html"), []byte("<p>{{ .Title }} custom</p>"), 0o644)
	if err := Write(pages, dir, Options{Title: "T", TemplatesDir: tdir,
		Sitemap: &SitemapOptions{Base: "https://x.test/doc", Priority: "0.5", Indexes: "1.0"}}); err != nil {
		t.Fatal(err)
	}
	page, _ = os.ReadFile(filepath.Join(dir, "function-foo.html"))
	if string(page) != "<p>Foo custom</p>" {
		t.Errorf("override: %q", page)
	}
	sm, _ := os.ReadFile(filepath.Join(dir, "sitemap.xml"))
	for _, want := range []string{`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`,
		"<loc>https://x.test/doc/function-foo.html</loc>\n<priority>0.5</priority>",
		"<loc>https://x.test/doc/index.html</loc>\n<priority>1.0</priority>"} {
		if !strings.Contains(string(sm), want) {
			t.Errorf("sitemap lacks %q:\n%s", want, sm)
		}
	}
	names, err := ExportTemplates(tdir)
	if err != nil || len(names) != 2 {
		t.Errorf("export: %v %v", names, err)
	}
}

// TestTemplateNames: one templates folder can serve several sites, so the file
// names are the site's business - and a name asked for by hand that is not
// there is an error, not a silent fallback to the built-in one.
func TestTemplateNames(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	tpl := filepath.Join(dir, "tpl")
	if err := os.MkdirAll(tpl, 0o755); err != nil {
		t.Fatal(err)
	}
	mark := `<html><body>MINE {{ .Title }}</body></html>`
	for _, n := range []string{"mine-page.html", "mine-index.html"} {
		if err := os.WriteFile(filepath.Join(tpl, n), []byte(mark), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pages := []gen.Page{{File: "a.md", Title: "A", Kind: "function"}}

	// the named templates are the ones used
	if err := Write(pages, out, Options{TemplatesDir: tpl,
		PageTemplate: "mine-page.html", IndexTemplate: "mine-index.html"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "a.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "MINE A") {
		t.Fatalf("the named template was not used: %s", got)
	}

	// a typo must say so instead of falling back
	err = Write(pages, filepath.Join(dir, "out2"), Options{TemplatesDir: tpl, PageTemplate: "mine-pag.html"})
	if err == nil || !strings.Contains(err.Error(), "mine-pag.html") {
		t.Fatalf("a missing named template must be an error, got %v", err)
	}

	// no names given: the built-in ones, as always
	if err := Write(pages, filepath.Join(dir, "out3"), Options{}); err != nil {
		t.Fatalf("the built-in templates must still work: %v", err)
	}
}

// TestCanonical: the public URL when the site has a base, the folder itself
// for the general index, the bare file when there is no base at all.
func TestCanonical(t *testing.T) {
	sm := &SitemapOptions{Base: "https://www.xbwin.com/ot4xb/doc/"}
	if got := canonical(sm, "tbinfile.html", "index.html"); got != "https://www.xbwin.com/ot4xb/doc/tbinfile.html" {
		t.Fatal(got)
	}
	if got := canonical(sm, "index.html", "index.html"); got != "https://www.xbwin.com/ot4xb/doc/" {
		t.Fatal(got)
	}
	if got := canonical(nil, "index.html", "index.html"); got != "index.html" {
		t.Fatal(got)
	}
}
