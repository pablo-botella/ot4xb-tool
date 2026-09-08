package dochtml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablo-botella/ot4xb-tool/modules/docgen"
)

// entries builds a page body from labelled values in the subset.
func entries(t *testing.T, labelled ...string) []docgen.Entry {
	var out []docgen.Entry
	for i := 0; i+1 < len(labelled); i += 2 {
		blocks, issues := docgen.ParseText(labelled[i+1])
		if len(issues) > 0 {
			t.Fatalf("%q: %v", labelled[i+1], issues)
		}
		out = append(out, docgen.Entry{Label: labelled[i], Blocks: blocks})
	}
	return out
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	pages := []docgen.Page{
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
