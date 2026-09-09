// The materialized pages: what resolve writes into the database for the
// renderers to read - one row per final page with its logical location
// (the trail: general index, book index, section, category), so that a
// page knows where it lives without every renderer working it out again.
package gen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/doctool"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/docdb"
)

// Materialize rebuilds the pages and page_trail tables of the database from
// its topics and the configuration conf (nil: the built-in one): one row
// per page - topics, groups, index and category pages - with its title,
// kind, first book, short description, keywords, categories, and its trail
// (the general index, the book index, the section and, when the section is
// by category, the category page), the parent being the last crumb. It
// returns the number of pages written. Re-runnable: the tables are emptied
// first.
func Materialize(db *docdb.DB, conf *doctool.Config) (int, error) {
	if conf == nil {
		conf = doctool.Default()
	}
	m, err := load(db)
	if err != nil {
		return 0, err
	}
	indexes := m.renderIndexes(conf) // records the trails as a side effect
	if _, err := db.Exec(`DELETE FROM page_trail`); err != nil {
		return 0, err
	}
	if _, err := db.Exec(`DELETE FROM pages`); err != nil {
		return 0, err
	}
	n := 0
	write := func(file, title, kind, book, short, kw, cats string) error {
		trail := m.trails[file]
		parent := ""
		if len(trail) > 0 {
			parent = trail[len(trail)-1].File
		}
		if _, err := db.Exec(`INSERT INTO pages(file, title, kind, book, short, keywords, categories, parent) VALUES (?,?,?,?,?,?,?,?)`,
			file, title, kind, book, short, kw, cats, parent); err != nil {
			return err
		}
		for i, c := range trail {
			if _, err := db.Exec(`INSERT INTO page_trail(file, pos, title, target) VALUES (?,?,?,?)`, file, i, c.Title, c.File); err != nil {
				return err
			}
		}
		n++
		return nil
	}
	for _, p := range m.pages {
		book := ""
		if bs := p.books(conf); len(bs) > 0 {
			book = bs[0]
		}
		if err := write(p.file, p.title, p.kind, book, p.short(), p.keywords(), strings.Join(p.cats(), ", ")); err != nil {
			return n, err
		}
	}
	var names []string
	for name := range indexes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		title := strings.TrimSuffix(name, ".md")
		if first, _, ok := strings.Cut(indexes[name], "\n"); ok && strings.HasPrefix(first, "# ") {
			title = strings.TrimSpace(first[2:])
		}
		if err := write(name, title, "index", "", "", "", ""); err != nil {
			return n, err
		}
	}
	return n, nil
}

// attachTrails reads the materialized location of every page and fills its
// Trail, Parent and Siblings. It fails when resolve has not run on this
// database: nothing is guessed.
func attachTrails(db *docdb.DB, pages []Page) error {
	rows, err := db.QueryAll(`SELECT file, parent FROM pages`)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("the database has no materialized pages: run resolve first")
	}
	parent := map[string]string{}
	for _, r := range rows {
		parent[r[0].(string)] = r[1].(string)
	}
	rows, err = db.QueryAll(`SELECT file, title, target FROM page_trail ORDER BY file, pos`)
	if err != nil {
		return err
	}
	trail := map[string][]Crumb{}
	for _, r := range rows {
		f := r[0].(string)
		trail[f] = append(trail[f], Crumb{Title: r[1].(string), File: r[2].(string)})
	}
	// siblings: the pages under the same parent, by title
	byParent := map[string][]Crumb{}
	for i := range pages {
		p := &pages[i]
		byParent[parent[p.File]] = append(byParent[parent[p.File]], Crumb{Title: p.Title, File: p.File})
	}
	for _, sib := range byParent {
		sort.SliceStable(sib, func(i, j int) bool { return strings.ToLower(sib[i].Title) < strings.ToLower(sib[j].Title) })
	}
	for i := range pages {
		p := &pages[i]
		p.Trail = trail[p.File]
		p.Parent = parent[p.File]
		if p.Parent == "" {
			continue // the general index, and a page listed nowhere: no siblings
		}
		for _, s := range byParent[p.Parent] {
			if s.File != p.File {
				p.Siblings = append(p.Siblings, s)
			}
		}
	}
	return nil
}
