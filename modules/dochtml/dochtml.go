// Package dochtml writes the documentation as a static site: every page of
// docgen.Build rendered from its tree into minimal HTML - standard tags, no
// classes, one tag per line - and poured into a template. Only the pages are
// written; a style sheet on request (GenCSS) and an assets folder copied as
// it is (Assets). Everything is relative, so the site works from disk
// (file://) as well as from a server.
//
// The text is escaped here and only here: identifiers, placeholders and
// paths arrive as written and leave as HTML that shows them. goldmark runs
// on the {{begin-md}} blocks alone, never on a page.
//
// The templates are Go html/template files: page.html for topic and group
// pages, index.html for the composed index pages, plus style.css for GenCSS.
// The built-in ones are embedded; a file of the same name in the templates
// folder of the site replaces it.
package dochtml

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/docgen"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed templates/*
var builtin embed.FS

// Options of a site.
type Options struct {
	Title        string          // site title (the header of every page)
	TemplatesDir string          // overrides of the built-in templates, may be ""
	Assets       string          // folder copied verbatim into the site, may be ""
	GenCSS       bool            // write style.css (the override or the built-in one)
	Sitemap      *SitemapOptions // write a sitemap, nil = none
	Keywords     []string        // the site's keywords, added to every page's after its own
	Search       string          // JSON file with every page for a client-side search, "" = none
	Log          func(string)
}

// searchEntry is one page of the search JSON.
type searchEntry struct {
	File       string   `json:"file"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind"`
	Books      []string `json:"books,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
	Short      string   `json:"short,omitempty"`
	Text       string   `json:"text,omitempty"`
}

// writeSearch writes the pages as JSON for a client-side search engine: the
// index pages are left out, the text is the page body as plain text.
func writeSearch(pages []docgen.Page, outDir, name string, siteKw []string) error {
	var entries []searchEntry
	for _, p := range pages {
		if p.Kind == "index" {
			continue
		}
		entries = append(entries, searchEntry{File: htmlName(p.File), Title: p.Title, Kind: p.Kind, Books: p.Books,
			Categories: p.Categories, Keywords: keywords(p.Keywords, siteKw), Short: p.Short, Text: textOf(p.Body)})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, name), data, 0o644)
}

// textOf is the plain text of a page body: labels, paragraphs, items and
// code, one space between everything, the zone-2 blocks as written.
func textOf(entries []docgen.Entry) string {
	var b strings.Builder
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(s)
		}
	}
	for _, e := range entries {
		add(e.Label)
		for _, bl := range e.Blocks {
			switch v := bl.(type) {
			case *docgen.Heading:
				add(plain(v.Inlines))
			case *docgen.Paragraph:
				add(plain(v.Inlines))
			case *docgen.List:
				for _, it := range v.Items {
					add(plain(it))
				}
			case *docgen.CodeBlock:
				add(strings.Join(strings.Fields(v.Text), " "))
			case *docgen.RawMarkdown:
				add(strings.Join(strings.Fields(v.Text), " "))
			}
		}
	}
	return b.String()
}

// keywords joins a page's keywords and the site's, without repeats.
func keywords(page, site []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range append(append([]string(nil), page...), site...) {
		if key := strings.ToLower(strings.TrimSpace(k)); key != "" && !seen[key] {
			seen[key] = true
			out = append(out, strings.TrimSpace(k))
		}
	}
	return out
}

// View is what a template receives.
type View struct {
	Site       string
	Title      string
	Kind       string
	Books      []string
	Categories []string
	Keywords   []string
	Short      string
	Source     string
	SourceURL  string
	File       string        // this page's file name (.html)
	Index      string        // the general index file name (.html)
	Body       template.HTML // the rendered tree
	Trail      []Crumb       // the logical location: general index, book index, section, category
	Siblings   []Crumb       // the other pages under the same parent, by title
}

// A Crumb is one page of a trail or a sibling list: its title and its
// .html file, relative to the site root.
type Crumb struct {
	Title string
	File  string
}

func crumbs(in []docgen.Crumb) []Crumb {
	var out []Crumb
	for _, c := range in {
		out = append(out, Crumb{Title: c.Title, File: htmlTarget(c.File)})
	}
	return out
}

// htmlTarget is htmlName for a link target that may carry a fragment:
// "index-xbase.md#functions" -> "index-xbase.html#functions".
func htmlTarget(target string) string {
	if file, anchor, ok := strings.Cut(target, "#"); ok {
		return htmlName(file) + "#" + anchor
	}
	return htmlName(target)
}

// anchor is the id a heading gets: its text in lower case, every run of
// characters outside a-z 0-9 one '-' (the same rule as the category slugs).
func anchor(text string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// plain is the text of an inline sequence, for the heading anchors.
func plain(in []docgen.Inline) string {
	var b strings.Builder
	for _, i := range in {
		switch v := i.(type) {
		case *docgen.Text:
			b.WriteString(v.Text)
		case *docgen.Strong:
			b.WriteString(v.Text)
		case *docgen.Code:
			b.WriteString(v.Text)
		case *docgen.Link:
			b.WriteString(v.Text)
		}
	}
	return b.String()
}

// SitemapOptions asks for a sitemap: one <url> per page, its <loc> the base
// URL (absolute, the folder the site is served from, with its trailing
// slash) plus the page file. The optional fields are written when set.
type SitemapOptions struct {
	Base       string // "https://www.xbwin.com/ot4xb/doc/"
	File       string // output file name (default sitemap.xml)
	ChangeFreq string // "weekly", ... ("" = not written)
	Priority   string // "0.5", ... ("" = not written)
	Indexes    string // priority of the index pages ("" = the same as Priority)
}

// writeSitemap writes the sitemap of the pages into outDir.
func writeSitemap(pages []docgen.Page, outDir string, s *SitemapOptions) error {
	base := s.Base
	if base == "" {
		return fmt.Errorf("sitemap: no base URL")
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	name := s.File
	if name == "" {
		name = "sitemap.xml"
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, p := range pages {
		b.WriteString("<url>\n<loc>" + html.EscapeString(base+htmlName(p.File)) + "</loc>\n")
		if s.ChangeFreq != "" {
			b.WriteString("<changefreq>" + html.EscapeString(s.ChangeFreq) + "</changefreq>\n")
		}
		pr := s.Priority
		if p.Kind == "index" && s.Indexes != "" {
			pr = s.Indexes
		}
		if pr != "" {
			b.WriteString("<priority>" + html.EscapeString(pr) + "</priority>\n")
		}
		b.WriteString("</url>\n")
	}
	b.WriteString("</urlset>\n")
	return os.WriteFile(filepath.Join(outDir, name), []byte(b.String()), 0o644)
}

// Write renders the pages into outDir (created if needed). The first page
// whose Kind is "index" and whose file is index.md is the general index.
func Write(pages []docgen.Page, outDir string, o Options) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	tpl, err := loadTemplates(o.TemplatesDir)
	if err != nil {
		return err
	}
	r := &renderer{md: goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough),
		goldmark.WithRendererOptions(ghtml.WithUnsafe()),
	)}
	index := "index.html"
	for _, p := range pages {
		if p.Kind == "index" && strings.EqualFold(p.File, "index.md") {
			index = htmlName(p.File)
			break
		}
	}
	for _, p := range pages {
		body, err := r.body(p.Body)
		if err != nil {
			return fmt.Errorf("%s: %w", p.File, err)
		}
		v := View{Site: o.Title, Title: p.Title, Kind: p.Kind, Books: p.Books, Categories: p.Categories,
			Keywords: keywords(p.Keywords, o.Keywords), Short: p.Short, Source: p.Source, File: htmlName(p.File), Index: index,
			Body: template.HTML(body), Trail: crumbs(p.Trail), Siblings: crumbs(p.Siblings)}
		name := "page.html"
		if p.Kind == "index" {
			name = "index.html"
		}
		var out bytes.Buffer
		if err := tpl.ExecuteTemplate(&out, name, v); err != nil {
			return fmt.Errorf("%s: %w", p.File, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, v.File), out.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if o.Sitemap != nil {
		if err := writeSitemap(pages, outDir, o.Sitemap); err != nil {
			return err
		}
	}
	if o.Search != "" {
		if err := writeSearch(pages, outDir, o.Search, o.Keywords); err != nil {
			return err
		}
	}
	if o.GenCSS {
		data, err := readTemplate(o.TemplatesDir, "style.css")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "style.css"), data, 0o644); err != nil {
			return err
		}
	}
	n := 0
	if o.Assets != "" {
		if n, err = copyDir(o.Assets, outDir); err != nil {
			return err
		}
	}
	if o.Log != nil {
		msg := fmt.Sprintf("html: %d page(s) -> %s", len(pages), outDir)
		if o.Assets != "" {
			msg += fmt.Sprintf(", %d asset file(s)", n)
		}
		o.Log(msg)
	}
	return nil
}

func htmlName(md string) string { return strings.TrimSuffix(md, ".md") + ".html" }

// ---------------------------------------------------------------- the tree

// renderer turns a page tree into HTML, one tag per line.
type renderer struct {
	md goldmark.Markdown
}

// body renders the entries of a page. A label goes in front of the value's
// first paragraph; when the value starts with a list or a code block, on a
// line of its own.
func (r *renderer) body(entries []docgen.Entry) (string, error) {
	var b strings.Builder
	for _, e := range entries {
		blocks := e.Blocks
		if e.Label != "" {
			label := "<strong>" + html.EscapeString(e.Label+":") + "</strong>"
			if len(blocks) > 0 {
				if p, ok := blocks[0].(*docgen.Paragraph); ok {
					b.WriteString("<p>" + label + " " + r.inlines(p.Inlines) + "</p>\n")
					blocks = blocks[1:]
				} else {
					b.WriteString("<p>" + label + "</p>\n")
				}
			} else {
				b.WriteString("<p>" + label + "</p>\n")
			}
		}
		for _, bl := range blocks {
			if err := r.block(&b, bl); err != nil {
				return "", err
			}
		}
	}
	return b.String(), nil
}

func (r *renderer) block(b *strings.Builder, bl docgen.Block) error {
	switch v := bl.(type) {
	case *docgen.Heading:
		// every heading carries an id so that a trail can point at it
		fmt.Fprintf(b, "<h%d id=\"%s\">%s</h%d>\n", v.Level, html.EscapeString(anchor(plain(v.Inlines))), r.inlines(v.Inlines), v.Level)
	case *docgen.Paragraph:
		b.WriteString("<p>" + r.inlines(v.Inlines) + "</p>\n")
	case *docgen.List:
		r.list(b, v)
	case *docgen.CodeBlock:
		if v.Lang != "" {
			b.WriteString(`<pre><code class="language-` + html.EscapeString(v.Lang) + `">`)
		} else {
			b.WriteString("<pre><code>")
		}
		b.WriteString(html.EscapeString(v.Text) + "</code></pre>\n")
	case *docgen.RawMarkdown:
		// zone 2: goldmark over this block alone; its links to .md pages
		// point to the .html ones
		var buf bytes.Buffer
		if err := r.md.Convert([]byte(v.Text), &buf); err != nil {
			return err
		}
		b.WriteString(strings.ReplaceAll(buf.String(), `.md"`, `.html"`))
	default:
		return fmt.Errorf("unknown block %T", bl)
	}
	return nil
}

// list renders a list; an item with a nested list (the general index) holds
// it inside its <li>.
func (r *renderer) list(b *strings.Builder, l *docgen.List) {
	b.WriteString("<ul>\n")
	for i, it := range l.Items {
		b.WriteString("<li>" + r.inlines(it))
		if i < len(l.Subs) && l.Subs[i] != nil {
			b.WriteString("\n")
			r.list(b, l.Subs[i])
		}
		b.WriteString("</li>\n")
	}
	b.WriteString("</ul>\n")
}

func (r *renderer) inlines(in []docgen.Inline) string {
	var b strings.Builder
	for _, i := range in {
		switch v := i.(type) {
		case *docgen.Text:
			b.WriteString(html.EscapeString(v.Text))
		case *docgen.Strong:
			b.WriteString("<strong>" + html.EscapeString(v.Text) + "</strong>")
		case *docgen.Code:
			b.WriteString("<code>" + html.EscapeString(v.Text) + "</code>")
		case *docgen.Link:
			target := v.Target
			if file, anchor, ok := strings.Cut(target, "#"); ok {
				target = htmlName(file) + "#" + anchor
			} else {
				target = htmlName(target)
			}
			b.WriteString(`<a href="` + html.EscapeString(target) + `">` + html.EscapeString(v.Text) + "</a>")
		case *docgen.Call:
			// left unresolved by the builder: shown as written
			b.WriteString(html.EscapeString("{{" + v.Name + ": " + v.Arg + "}}"))
		}
	}
	return b.String()
}

// ---------------------------------------------------------------- files

// copyDir copies the files of src, subfolders included, into dst as they
// are, and returns how many.
func copyDir(src, dst string) (int, error) {
	n := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		n++
		return os.WriteFile(target, data, 0o644)
	})
	return n, err
}

// readTemplate returns the override from dir when it exists, the built-in
// file otherwise.
func readTemplate(dir, name string) ([]byte, error) {
	if dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			return data, nil
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return builtin.ReadFile("templates/" + name)
}

func loadTemplates(dir string) (*template.Template, error) {
	t := template.New("site").Funcs(template.FuncMap{
		"join":  strings.Join,
		"lower": strings.ToLower,
	})
	for _, name := range []string{"page.html", "index.html"} {
		data, err := readTemplate(dir, name)
		if err != nil {
			return nil, err
		}
		if _, err := t.New(name).Parse(string(data)); err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
	}
	return t, nil
}

// ExportTemplates writes the built-in templates into dir (created if
// needed), skipping the files that already exist, so they can be edited.
// It returns the names written.
func ExportTemplates(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var out []string
	for _, name := range []string{"page.html", "index.html", "style.css"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		data, err := builtin.ReadFile("templates/" + name)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, nil
}
