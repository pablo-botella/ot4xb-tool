// Package gensite generates the static HTML site of a documentation database.
// It resolves what the site is - from a .site-def, from explicit options, or
// both, the options winning - loads the configuration, builds the pages with
// docgen and writes them with html.
//
// The configuration is the .doc-tool completed with the blocks the database
// carries (doctool.LoadDB), so a repository holding only the .db and its
// .site-def generates without a .doc-tool of its own.
package site

import (
	"errors"
	"fmt"

	"github.com/pablo-botella/ot4xb-tool/modules/doctool"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/gen"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/html"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/scan"
)

// ErrUsage is returned when, after reading the .site-def and the .doc-tool,
// there is still no database or no output folder. The caller prints its own
// usage: what is missing is an argument, not a fact about the documentation.
var ErrUsage = errors.New("gensite: a database and an output folder are required")

// Options is what the site is made of. Everything is optional: Site names a
// .site-def that supplies the ones left empty, and DocTool supplies the
// database when neither gives one. A value set here always wins over the
// .site-def.
type Options struct {
	DocTool       string // the .doc-tool (its .user applies); "" = only the database
	DB            string // the compiled database
	Site          string // a .site-def describing the site
	Out           string // where the site is written
	Title         string // the header of every page; "" = the title of the general index
	Templates     string // overrides of the built-in templates; "" = the built-in ones
	PageTemplate  string // the file of Templates used for a page; "" = page.html
	IndexTemplate string // the file of Templates used for an index; "" = index.html
	Assets        string // a folder copied into Out as it is
	Search        string // the JSON file written for a client-side search engine
	Sitemap       string // base URL of the sitemap; overrides the one of the .site-def
	GenCSS        bool   // write style.css into Out
	CleanURLs     bool   // links, sitemap and search index drop the .html
	Export        string // write the built-in templates here and, with no Out, stop

	Log  func(string) // progress, one line at a time
	Warn func(string) // what deserves attention but does not stop the run
}

// Run generates the site.
func Run(o Options) error {
	log, warn := o.Log, o.Warn
	if log == nil {
		log = func(string) {}
	}
	if warn == nil {
		warn = func(string) {}
	}
	if o.Export != "" {
		names, err := html.ExportTemplates(o.Export)
		if err != nil {
			return err
		}
		log(fmt.Sprintf("gensite: %d template(s) written to %s", len(names), o.Export))
		if o.Out == "" && o.Site == "" {
			return nil
		}
	}

	var sm *html.SitemapOptions
	var keywords []string
	if o.Site != "" {
		def, err := doctool.LoadSite(o.Site)
		if err != nil {
			return err
		}
		fill(&o.DocTool, def.DocToolPath())
		fill(&o.DB, def.DBPath())
		fill(&o.Out, def.OutDir())
		fill(&o.Templates, def.TemplatesDir())
		fill(&o.PageTemplate, def.TemplatePage)
		fill(&o.IndexTemplate, def.TemplateIndex)
		fill(&o.Assets, def.AssetsDir())
		fill(&o.Title, def.Title)
		fill(&o.Search, def.Search)
		o.GenCSS = o.GenCSS || def.GenCSS
		o.CleanURLs = o.CleanURLs || def.CleanURLs
		keywords = def.Keywords
		if def.Sitemap != nil {
			sm = &html.SitemapOptions{Base: def.Sitemap.Base, File: def.Sitemap.File,
				ChangeFreq: def.Sitemap.ChangeFreq, Priority: def.Sitemap.Priority, Indexes: def.Sitemap.Indexes}
		}
	}
	if o.Sitemap != "" {
		if sm == nil {
			sm = &html.SitemapOptions{}
		}
		sm.Base = o.Sitemap
	}

	// the database has to be known before the configuration can be completed:
	// the blocks the .doc-tool leaves out live in that very database
	if o.DB == "" && o.DocTool != "" {
		p, err := doctool.PeekDB(o.DocTool)
		if err != nil {
			return err
		}
		o.DB = p
	}
	if o.DB == "" || o.Out == "" {
		return ErrUsage
	}

	conf, err := loadConfig(o.DocTool, o.DB)
	if err != nil {
		return err
	}
	fill(&o.Title, conf.Index.Title)

	pages, err := gen.Build(o.DB, conf, warn)
	if err != nil {
		return err
	}
	return html.Write(pages, o.Out, html.Options{Title: o.Title, TemplatesDir: o.Templates,
		PageTemplate: o.PageTemplate, IndexTemplate: o.IndexTemplate,
		Assets: o.Assets, GenCSS: o.GenCSS, Sitemap: sm, Keywords: keywords, Search: o.Search,
		CleanURLs: o.CleanURLs, Log: log})
}

// loadConfig opens the database only to read its configuration blocks: docgen
// opens it again for the data.
func loadConfig(defPath, dbPath string) (*doctool.Config, error) {
	db, err := docdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	conf, err := doctool.LoadDB(defPath, db)
	if err != nil {
		return nil, err
	}
	scan.Use(conf.KindTable())
	return conf, nil
}

// fill sets dst to v when dst is still empty: the option wins, the .site-def
// completes.
func fill(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}
