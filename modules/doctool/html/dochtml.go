// Package dochtml writes the documentation as a static site: every page of
// gen.Build rendered from its tree into minimal HTML - standard tags, no
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
package html

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

	"github.com/pablo-botella/ot4xb-tool/modules/doctool/gen"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed templates/*
var builtin embed.FS

// Options of a site.
type Options struct {
	Title        string // site title (the header of every page)
	TemplatesDir string // overrides of the built-in templates, may be ""
	Assets       string // folder copied verbatim into the site, may be ""
	GenCSS       bool   // write style.css (the override or the built-in one)
	// PageTemplate and IndexTemplate name the files of TemplatesDir to use
	// ('' = the built-in names). One folder can then hold the templates of
	// several sites, which a single fixed name would make impossible.
	PageTemplate  string
	IndexTemplate string
	Sitemap       *SitemapOptions // write a sitemap, nil = none
	CleanURLs     bool            // links, sitemap and search index drop the .html
	Keywords      []string        // the site's keywords, added to every page's after its own
	Search        string          // JSON file with every page for a client-side search, "" = none
	Log           func(string)
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
func writeSearch(pages []gen.Page, outDir, name string, siteKw []string, clean bool) error {
	var entries []searchEntry
	for _, p := range pages {
		if p.Kind == "index" {
			continue
		}
		entries = append(entries, searchEntry{File: href(htmlName(p.File), clean), Title: p.Title, Kind: p.Kind, Books: p.Books,
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
func textOf(entries []gen.Entry) string {
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
			case *gen.Heading:
				add(plain(v.Inlines))
			case *gen.Paragraph:
				add(plain(v.Inlines))
			case *gen.List:
				for _, it := range v.Items {
					add(plain(it))
				}
			case *gen.CodeBlock:
				add(strings.Join(strings.Fields(v.Text), " "))
			case *gen.RawMarkdown:
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
	File       string // this page's file name (.html)
	// Canonical is the address a search engine should file the page under:
	// the public URL when the site has one (the base of its sitemap), the
	// file otherwise; the general index is the folder itself, without
	// index.html.
	Canonical string
	Index     string        // the general index file name (.html)
	Body      template.HTML // the rendered tree
	Trail     []Crumb       // the logical location: general index, book index, section, category
	Siblings  []Crumb       // the other pages under the same parent, by title
}

// A Crumb is one page of a trail or a sibling list: its title and its
// .html file, relative to the site root.
type Crumb struct {
	Title string
	File  string
}

// canonical builds the canonical URL of a page: base + file, the base alone
// for the general index; with no base, the file as it is.
func canonical(sm *SitemapOptions, file, index string, clean bool) string {
	if sm == nil || sm.Base == "" {
		return href(file, clean)
	}
	// the same normalising writeSitemap does: without it a base written
	// without its final slash gives a good sitemap and a glued canonical
	base := sm.Base
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	if file == index {
		return base
	}
	return base + link(file, clean)
}

func crumbs(in []gen.Crumb, clean bool) []Crumb {
	var out []Crumb
	for _, c := range in {
		out = append(out, Crumb{Title: c.Title, File: htmlTarget(c.File, clean)})
	}
	return out
}

// htmlTarget is htmlName for a link target that may carry a fragment:
// "index-xbase.md#functions" -> "index-xbase.html#functions".
func htmlTarget(target string, clean bool) string {
	if file, anchor, ok := strings.Cut(target, "#"); ok {
		return href(htmlName(file), clean) + "#" + anchor
	}
	return href(htmlName(target), clean)
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
func plain(in []gen.Inline) string {
	var b strings.Builder
	for _, i := range in {
		switch v := i.(type) {
		case *gen.Text:
			b.WriteString(v.Text)
		case *gen.Strong:
			b.WriteString(v.Text)
		case *gen.Code:
			b.WriteString(v.Text)
		case *gen.Link:
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
func writeSitemap(pages []gen.Page, outDir string, s *SitemapOptions, clean bool) error {
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
		b.WriteString("<url>\n<loc>" + html.EscapeString(base+link(htmlName(p.File), clean)) + "</loc>\n")
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
func Write(pages []gen.Page, outDir string, o Options) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	tpl, err := loadTemplates(o.TemplatesDir, o.PageTemplate, o.IndexTemplate)
	if err != nil {
		return err
	}
	r := &renderer{clean: o.CleanURLs, md: goldmark.New(
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
			Keywords: keywords(p.Keywords, o.Keywords), Short: p.Short, Source: p.Source, File: htmlName(p.File), Index: href(index, o.CleanURLs), Canonical: canonical(o.Sitemap, htmlName(p.File), index, o.CleanURLs),
			Body: template.HTML(body), Trail: crumbs(p.Trail, o.CleanURLs), Siblings: crumbs(p.Siblings, o.CleanURLs)}
		name := "page"
		if p.Kind == "index" {
			name = "index"
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
		if err := writeSitemap(pages, outDir, o.Sitemap, o.CleanURLs); err != nil {
			return err
		}
	}
	if o.Search != "" {
		if err := writeSearch(pages, outDir, o.Search, o.Keywords, o.CleanURLs); err != nil {
			return err
		}
	}
	if o.GenCSS {
		data, err := readTemplate(o.TemplatesDir, "style.css", false)
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

// link is what points at a rendered file. With clean URLs the extension goes
// and the general index becomes the folder itself, because the hosts that want
// this map /foo to foo.html and /dir/ to /dir/index.html - and answer a
// redirect for the name with extension, which is the whole point of dropping
// it. The file on disk keeps the extension either way.
func link(file string, clean bool) string {
	if !clean {
		return file
	}
	if file == "index.html" {
		return "" // the folder itself
	}
	return strings.TrimSuffix(file, ".html")
}

// href is link for a link written inside a page, where the folder cannot be
// the empty string: an empty href means this same page.
func href(file string, clean bool) string {
	if s := link(file, clean); s != "" {
		return s
	}
	return "./"
}

// ---------------------------------------------------------------- the tree

// renderer turns a page tree into HTML, one tag per line.
type renderer struct {
	md    goldmark.Markdown
	clean bool // links drop the .html
}

// body renders the entries of a page. A label goes in front of the value's
// first paragraph; when the value starts with a list or a code block, on a
// line of its own.
func (r *renderer) body(entries []gen.Entry) (string, error) {
	var b strings.Builder
	for _, e := range entries {
		blocks := e.Blocks
		if e.Label != "" {
			label := "<strong>" + html.EscapeString(e.Label+":") + "</strong>"
			if len(blocks) > 0 {
				if p, ok := blocks[0].(*gen.Paragraph); ok {
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

func (r *renderer) block(b *strings.Builder, bl gen.Block) error {
	switch v := bl.(type) {
	case *gen.Heading:
		// every heading carries an id so that a trail can point at it
		fmt.Fprintf(b, "<h%d id=\"%s\">%s</h%d>\n", v.Level, html.EscapeString(anchor(plain(v.Inlines))), r.inlines(v.Inlines), v.Level)
	case *gen.Paragraph:
		b.WriteString("<p>" + r.inlines(v.Inlines) + "</p>\n")
	case *gen.List:
		r.list(b, v)
	case *gen.CodeBlock:
		if v.Lang != "" {
			b.WriteString(`<pre><code class="language-` + html.EscapeString(v.Lang) + `">`)
		} else {
			b.WriteString("<pre><code>")
		}
		b.WriteString(html.EscapeString(v.Text) + "</code></pre>\n")
	case *gen.RawMarkdown:
		// zone 2: goldmark over this block alone; its links to .md pages
		// point to the .html ones
		var buf bytes.Buffer
		if err := r.md.Convert([]byte(v.Text), &buf); err != nil {
			return err
		}
		ext := `.html"`
		if r.clean {
			ext = `"`
		}
		b.WriteString(strings.ReplaceAll(buf.String(), `.md"`, ext))
	default:
		return fmt.Errorf("unknown block %T", bl)
	}
	return nil
}

// list renders a list; an item with a nested list (the general index) holds
// it inside its <li>.
func (r *renderer) list(b *strings.Builder, l *gen.List) {
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

func (r *renderer) inlines(in []gen.Inline) string {
	var b strings.Builder
	for _, i := range in {
		switch v := i.(type) {
		case *gen.Text:
			b.WriteString(html.EscapeString(v.Text))
		case *gen.Strong:
			b.WriteString("<strong>" + html.EscapeString(v.Text) + "</strong>")
		case *gen.Code:
			b.WriteString("<code>" + html.EscapeString(v.Text) + "</code>")
		case *gen.Link:
			target := htmlTarget(v.Target, r.clean)
			b.WriteString(`<a href="` + html.EscapeString(target) + `">` + html.EscapeString(v.Text) + "</a>")
		case *gen.Call:
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
func readTemplate(dir, name string, mustExist bool) ([]byte, error) {
	if dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			return data, nil
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if mustExist {
		return nil, fmt.Errorf("template %q not found in %s", name, dir)
	}
	return builtin.ReadFile("templates/" + name)
}

// DefaultPageTemplate and DefaultIndexTemplate are the file names used when
// the site names none.
const (
	DefaultPageTemplate  = "page.html"
	DefaultIndexTemplate = "index.html"
)

// loadTemplates parses the two templates under fixed internal ids, so the name
// of the file and the name of the template stop being the same thing.
func loadTemplates(dir, pageFile, indexFile string) (*template.Template, error) {
	t := template.New("site").Funcs(template.FuncMap{
		"join":  strings.Join,
		"lower": strings.ToLower,
	})
	for _, e := range []struct {
		id, file, def string
	}{
		{"page", pageFile, DefaultPageTemplate},
		{"index", indexFile, DefaultIndexTemplate},
	} {
		name := e.file
		if name == "" {
			name = e.def
		}
		// a name the site asked for by hand must exist: falling back to the
		// built-in one silently is how a typo becomes a mystery
		data, err := readTemplate(dir, name, e.file != "" && e.file != e.def)
		if err != nil {
			return nil, err
		}
		if _, err := t.New(e.id).Parse(string(data)); err != nil {
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
