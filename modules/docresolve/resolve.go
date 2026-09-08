// Package docresolve is the a-posteriori step of the documentation database:
// once every source has been compiled, it verifies the references against the
// topics and records what is broken as issues. It is re-runnable: its own
// issues (code "resolve/...") are dropped and recomputed every time.
//
// Rules (spec Draft 4):
//   - A reference target is one of: <kind ident> looked up by (kind, key) with
//     the EXACT kind (no families: a function and a c-function of the same
//     name are two topics); <slug name> looked up among topic and group slugs,
//     case-insensitively; <tg name> looked up among the topic groups.
//   - Missing target -> issue "resolve/missing" on the segment that wrote it.
//   - include (transclusion) cycles are forbidden -> issue "resolve/cycle".
//     Repetitions are fine.
//   - Two pages (topics without a group, or groups) with the same slug,
//     case-insensitively -> issue "resolve/slug-duplicate" on the later one;
//     the generator will still write both, suffixing the later file.
//   - A topic may have any number of segments from any number of sources: that
//     is how scattered content and C++ overloads work. Never an issue.
package docresolve

import (
	"fmt"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/docconf"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/docgen"
)

// Report summarizes one resolve run.
type Report struct {
	Refs     int // references examined
	Missing  int // references whose target does not exist
	Cycles   int // include cycles found
	DupSlugs int // pages sharing a slug
	Books    int // book problems: unknown book named, or no book at all
	Pages    int // pages materialized, with their logical location
}

type topicKey struct{ kind, key string }

type ref struct {
	idseg, idsrc, from int64
	toKind, toIdent    string
	refType            string
}

// Resolve runs the step over an open database. conf is the documentation
// configuration (nil: the built-in one); it decides the books a topic may
// name and which kinds must name one.
func Resolve(db *docdb.DB, conf *docconf.Config) (Report, error) {
	var rep Report
	if conf == nil {
		conf = docconf.Default()
	}
	if _, err := db.Exec(`DELETE FROM issues WHERE code LIKE 'resolve/%'`); err != nil {
		return rep, err
	}
	// ---- collect everything first, write nothing while reading
	topics := map[topicKey]int64{}
	slugs := map[string][]string{} // lower slug -> pages ("topic kind ident" / "group name")
	rows, err := db.QueryAll(`SELECT idtopic, kind, key, ident, slug, idtg FROM topics ORDER BY idtopic`)
	if err != nil {
		return rep, err
	}
	topicIdent := map[int64]string{}
	for _, r := range rows {
		id := r[0].(int64)
		kind, key, ident, slug := r[1].(string), r[2].(string), r[3].(string), r[4].(string)
		topics[topicKey{kind, key}] = id
		topicIdent[id] = kind + " " + ident
		if r[5].(int64) == 0 && slug != "" {
			slugs[strings.ToLower(slug)] = append(slugs[strings.ToLower(slug)], kind+" "+ident)
		}
	}
	groups := map[string]bool{}
	rows, err = db.QueryAll(`SELECT key, slug FROM topic_groups ORDER BY idtg`)
	if err != nil {
		return rep, err
	}
	for _, r := range rows {
		groups[r[0].(string)] = true
		if s := r[1].(string); s != "" {
			slugs[strings.ToLower(s)] = append(slugs[strings.ToLower(s)], "group "+r[0].(string))
		}
	}
	rows, err = db.QueryAll(`SELECT idseg, idsrc, idtopic_in, reftokind, reftoident, reftype FROM refs ORDER BY idseg`)
	if err != nil {
		return rep, err
	}
	var refs []ref
	for _, r := range rows {
		refs = append(refs, ref{r[0].(int64), r[1].(int64), r[2].(int64), r[3].(string), r[4].(string), r[5].(string)})
	}
	segLine := map[int64]int64{}
	rows, err = db.QueryAll(`SELECT idseg, line FROM segments`)
	if err != nil {
		return rep, err
	}
	for _, r := range rows {
		segLine[r[0].(int64)] = r[1].(int64)
	}
	// ---- judge
	type issue struct {
		idsrc, idseg, line int64
		code, msg          string
	}
	var issues []issue
	includes := map[int64][]int64{} // topic -> included note topics
	for _, rf := range refs {
		rep.Refs++
		var found bool
		var target int64
		switch rf.toKind {
		case "slug":
			found = len(slugs[strings.ToLower(rf.toIdent)]) > 0
		case "tg":
			found = groups[rf.toIdent]
		default:
			target, found = topics[topicKey{rf.toKind, rf.toIdent}]
		}
		if !found {
			rep.Missing++
			issues = append(issues, issue{rf.idsrc, rf.idseg, segLine[rf.idseg], "resolve/missing",
				fmt.Sprintf("%s target <%s %s> does not exist", rf.refType, rf.toKind, rf.toIdent)})
			continue
		}
		if rf.refType == "include" {
			includes[rf.from] = append(includes[rf.from], target)
		}
	}
	// include cycles: a note that (transitively) includes itself
	for from := range includes {
		if path := findCycle(includes, from); path != nil {
			rep.Cycles++
			names := make([]string, 0, len(path))
			for _, id := range path {
				names = append(names, topicIdent[id])
			}
			// blame the segment(s) of `from` that include the next hop
			for _, rf := range refs {
				if rf.from == from && rf.refType == "include" && topics[topicKey{rf.toKind, rf.toIdent}] == path[1] {
					issues = append(issues, issue{rf.idsrc, rf.idseg, segLine[rf.idseg], "resolve/cycle",
						"include cycle: " + strings.Join(names, " -> ")})
				}
			}
		}
	}
	for s, pages := range slugs {
		if len(pages) > 1 {
			rep.DupSlugs++
			issues = append(issues, issue{0, 0, 0, "resolve/slug-duplicate",
				fmt.Sprintf("slug %q is shared by %s", s, strings.Join(pages, ", "))})
		}
	}
	// books: a named book must exist; a kind without a fixed book and without
	// a default needs the topic to name one (unless the kind is not indexed)
	named := map[int64][]string{}
	rows, err = db.QueryAll(`SELECT idtopic, idsrc, book FROM topic_book ORDER BY rowid`)
	if err != nil {
		return rep, err
	}
	for _, r := range rows {
		id, book := r[0].(int64), r[2].(string)
		named[id] = append(named[id], book)
		if conf.Book(book) == nil {
			rep.Books++
			issues = append(issues, issue{r[1].(int64), 0, 0, "resolve/unknown-book",
				fmt.Sprintf("%s names the book %q, which the configuration does not declare", topicIdent[id], book)})
		}
	}
	rows, err = db.QueryAll(`SELECT t.idtopic, t.kind, MIN(s.idsrc), MIN(s.line) FROM topics t JOIN segments s USING(idtopic) GROUP BY t.idtopic`)
	if err != nil {
		return rep, err
	}
	for _, r := range rows {
		id, kind := r[0].(int64), r[1].(string)
		k := conf.Kind(kind)
		if k == nil {
			continue
		}
		if len(conf.BooksOf(kind, named[id])) == 0 && k.IsIndexed() {
			rep.Books++
			issues = append(issues, issue{r[2].(int64), 0, r[3].(int64), "resolve/book-required",
				fmt.Sprintf("%s belongs to no book: a %s must say \"book: <name>\"", topicIdent[id], kind)})
		}
	}
	// ---- write
	for _, is := range issues {
		if err := db.AddIssue(is.idsrc, is.idseg, is.line, "error", is.code, is.msg); err != nil {
			return rep, err
		}
	}
	// ---- materialize the pages with their logical location
	if rep.Pages, err = docgen.Materialize(db, conf); err != nil {
		return rep, err
	}
	return rep, nil
}

// findCycle returns the path from start back to itself, or nil.
func findCycle(g map[int64][]int64, start int64) []int64 {
	var path []int64
	seen := map[int64]bool{}
	var walk func(n int64) bool
	walk = func(n int64) bool {
		path = append(path, n)
		for _, m := range g[n] {
			if m == start {
				path = append(path, m)
				return true
			}
			if seen[m] {
				continue
			}
			seen[m] = true
			if walk(m) {
				return true
			}
		}
		path = path[:len(path)-1]
		return false
	}
	if walk(start) {
		return path
	}
	return nil
}
