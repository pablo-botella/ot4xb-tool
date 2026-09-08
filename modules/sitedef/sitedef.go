// Package sitedef reads a .site-def file: the description of one static site
// built from a documentation database. One database can feed any number of
// sites, so what belongs to a site - its templates folder, its output folder
// and its title - lives here and not in the .doc-tool file.
//
// The file is JSON:
//
//	{
//	  "doc_tool": "../ot4xb.doc-tool",
//	  "db": "../out/ot4xb.db",
//	  "templates": "./templates",
//	  "assets": "./assets",
//	  "gencss": false,
//	  "out": "./out",
//	  "title": "ot4xb Reference"
//	}
//
// Paths are relative to the file. doc_tool is the .doc-tool of the
// documentation and db its database (both optional: the -doctool and -db
// flags of gensite override or replace them), templates is optional (the built-in
// templates apply without it), assets is optional (a folder copied into out
// as it is, files and subfolders - style sheets, favicon, robots.txt, the
// static folder of other generators), out is required, title defaults to the
// title of the general index, gencss (default false) writes style.css.
package sitedef

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is a .site-def file.
type Config struct {
	Path string `json:"-"` // absolute path of the file
	Dir  string `json:"-"` // its folder, the base of relative paths

	// DocTool is the .doc-tool file of the documentation ("" = none).
	DocTool string `json:"doc_tool"`
	// DB is the documentation database ("" = none).
	DB string `json:"db"`
	// Templates is the folder whose files override the built-in templates
	// of the site ("" = none).
	Templates string `json:"templates"`
	// Assets is a folder copied verbatim into Out ("" = none).
	Assets string `json:"assets"`
	// Out is the folder the site is written into.
	Out string `json:"out"`
	// Title names the site (the header of every page).
	Title string `json:"title"`
	// GenCSS writes style.css into Out: the override of the templates folder
	// when there is one, the built-in sheet otherwise.
	GenCSS bool `json:"gencss"`
	// Keywords are added to the keywords of every page, after its own.
	Keywords []string `json:"keywords"`
	// Search names the JSON file written with every page for a client-side
	// search engine ("" = none): file, title, kind, books, categories,
	// keywords, short description and the page text.
	Search string `json:"search"`
	// Sitemap, when present, asks for a sitemap: base is the absolute URL the
	// site is served from (required), file the output name (default
	// sitemap.xml), changefreq and priority optional <url> fields, indexes the
	// priority of the index pages when it differs.
	Sitemap *Sitemap `json:"sitemap"`
}

// Sitemap is the "sitemap" object of a .site-def.
type Sitemap struct {
	Base       string `json:"base"`
	File       string `json:"file"`
	ChangeFreq string `json:"changefreq"`
	Priority   string `json:"priority"`
	Indexes    string `json:"indexes"`
}

// Load reads and checks a .site-def file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	c.Path = abs
	c.Dir = filepath.Dir(abs)
	if c.Out == "" {
		return nil, fmt.Errorf("%s: no out folder", path)
	}
	return c, nil
}

// OutDir is the output folder, absolute.
func (c *Config) OutDir() string { return c.abs(c.Out) }

// DocToolPath is the .doc-tool file, absolute ("" when none).
func (c *Config) DocToolPath() string {
	if c.DocTool == "" {
		return ""
	}
	return c.abs(c.DocTool)
}

// DBPath is the database, absolute ("" when none).
func (c *Config) DBPath() string {
	if c.DB == "" {
		return ""
	}
	return c.abs(c.DB)
}

// AssetsDir is the assets folder, absolute ("" when none).
func (c *Config) AssetsDir() string {
	if c.Assets == "" {
		return ""
	}
	return c.abs(c.Assets)
}

// TemplatesDir is the templates folder, absolute ("" when none).
func (c *Config) TemplatesDir() string {
	if c.Templates == "" {
		return ""
	}
	return c.abs(c.Templates)
}

func (c *Config) abs(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(c.Dir, p)
}
