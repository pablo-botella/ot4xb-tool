// Package doccompile is the ot4xb layer of the documentation database: it
// scans sources with srcdoc (Draft 4) and writes what it finds into a db.
//
// One call per source file = one compile pass: register the source (it keeps
// its pos across passes), drop what it contributed before, insert its topics
// again. A topic is created the first time its (kind, key) is seen and every
// later block with the same identity - same file or another - only appends
// segments to it, in parse order. Every marker of a topic is one segment
// (raw text kept), its fields go to the fields table in written order, its
// references (include-note-id, inline {{ilink: <target> text}}) to refs, its
// categories (comma lists allowed) to topic_category. _slug_ and _tg_ are
// applied with the rules of the spec (explicit wins, first explicit stays,
// conflicts are issues). Resolution is a separate step (docresolve).
//
// Source order: the .ch headers are compiled after every C/C++ source, so the
// annotations they add to existing topics land after the main content.
package compile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/doctool/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/doctool/scan"
)

// Key is scan.Key: the topic key of an identity.
func Key(kind, ident string) string { return scan.Key(kind, ident) }

// ilinkRe matches an inline link: {{ilink: <target-kind target-ident> text}}.
// The target is "<kind ident>", "<slug name>" or "<tg name>"; text is
// optional.
var ilinkRe = regexp.MustCompile(`\{\{\s*ilink\s*:\s*<\s*([A-Za-z_-]+)\s+([^>]+?)\s*>\s*([^}]*)\}\}`)

// Ilink is one inline link found in a value.
type Ilink struct {
	Kind, Ident, Text string
}

// Ilinks extracts every inline link of a value.
func Ilinks(v string) []Ilink {
	var out []Ilink
	for _, m := range ilinkRe.FindAllStringSubmatch(v, -1) {
		out = append(out, Ilink{Kind: m[1], Ident: strings.TrimSpace(m[2]), Text: strings.TrimSpace(m[3])})
	}
	return out
}

// ComputedSlug is the fallback slug of a topic: kind-key lower-cased, ':'
// becomes '.', anything outside a-z 0-9 _ - . becomes '-'.
func ComputedSlug(kind, key string) string {
	return slugify(kind + "-" + key)
}

func slugify(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
			b.WriteRune(c)
		case c == ':':
			b.WriteByte('.')
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// ValidSlug reports whether an explicit slug uses only a-z 0-9 _ - . (after
// lower-casing).
func ValidSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// CompileFile writes one scanned file into the database. src is the path
// relative to the project root (the database's source name).
func CompileFile(db *docdb.DB, src string, f *scan.File) (idsrc int64, err error) {
	idsrc, srcPos, _, err := db.Source(src)
	if err != nil {
		return 0, err
	}
	if _, err := db.Exec("BEGIN"); err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			db.Exec("ROLLBACK")
		}
	}()
	if err = db.ReplaceSource(idsrc); err != nil {
		return 0, err
	}
	for _, is := range f.Issues {
		if err := db.AddIssue(idsrc, 0, int64(is.Line), is.Severity, "scan/"+is.Code, is.Message); err != nil {
			return 0, err
		}
	}
	var seq int64
	for _, t := range f.Topics {
		idtopic, err := db.Topic(t.Kind, t.Key, t.Ident)
		if err != nil {
			return 0, err
		}
		if _, err := db.SetTopicSlug(idtopic, ComputedSlug(t.Kind, t.Key), false); err != nil {
			return 0, err
		}
		for _, mk := range t.Markers {
			seq++
			pos := docdb.PackPos(srcPos, seq)
			idseg, err := db.AddSegment(idtopic, idsrc, pos, int64(mk.Line), []byte(mk.Raw))
			if err != nil {
				return 0, err
			}
			for i, fd := range mk.Fields {
				if err := db.AddField(idsrc, idseg, int64(i), fd.Label, fd.Value, fd.HideEntry, fd.HideLabel); err != nil {
					return 0, err
				}
				if err := emitField(db, idsrc, idseg, idtopic, pos, t, mk, fd); err != nil {
					return 0, err
				}
			}
		}
	}
	if _, err = db.Exec("COMMIT"); err != nil {
		return 0, err
	}
	return idsrc, nil
}

// emitField applies the fields the tool interprets (slug, tg, category,
// include-note-id) and records the inline links of every value.
func emitField(db *docdb.DB, idsrc, idseg, idtopic, pos int64, t *scan.Topic, mk *scan.Marker, fd scan.Field) error {
	switch fd.Label {
	case "slug":
		if mk.Kind != scan.MkHeader {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "warning", "compile/slug-in-fragment", "slug only counts in the topic header; ignored")
		}
		slug := strings.ToLower(strings.TrimSpace(fd.Value))
		if !ValidSlug(slug) {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "error", "compile/bad-slug", fmt.Sprintf("slug %q: only a-z 0-9 _ - . allowed", fd.Value))
		}
		conflict, err := db.SetTopicSlug(idtopic, slug, true)
		if err != nil {
			return err
		}
		if conflict != "" {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "warning", "compile/slug-conflict",
				fmt.Sprintf("%s %s already has the explicit slug %q; %q ignored (the first one stays)", t.Kind, t.Ident, conflict, slug))
		}
	case "tg":
		if mk.Kind != scan.MkHeader {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "warning", "compile/tg-in-fragment", "tg only counts in the topic header; ignored")
		}
		name := strings.TrimSpace(fd.Value)
		if name == "" {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "error", "compile/bad-tg", "empty topic group name")
		}
		idtg, err := db.Group(name, name, idsrc, pos)
		if err != nil {
			return err
		}
		if _, err := db.SetGroupSlug(idtg, slugify(name), false); err != nil {
			return err
		}
		if s := t.Field("slug"); s != nil && ValidSlug(s.Value) {
			conflict, err := db.SetGroupSlug(idtg, strings.ToLower(strings.TrimSpace(s.Value)), true)
			if err != nil {
				return err
			}
			if conflict != "" {
				if err := db.AddIssue(idsrc, idseg, int64(fd.Line), "warning", "compile/tg-slug-conflict",
					fmt.Sprintf("topic group %s already has the explicit slug %q; %q ignored (all blocks of a group must agree)", name, conflict, s.Value)); err != nil {
					return err
				}
			}
		}
		other, err := db.SetTopicGroup(idtopic, idtg)
		if err != nil {
			return err
		}
		if other != 0 {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "warning", "compile/tg-conflict",
				fmt.Sprintf("%s %s is already in another topic group; tg %s ignored", t.Kind, t.Ident, name))
		}
	case "category":
		if mk.Kind == scan.MkHeader {
			for _, c := range scan.Categories(fd.Value) {
				if err := db.AddCategory(idsrc, idtopic, c); err != nil {
					return err
				}
			}
		}
	case "book":
		if mk.Kind != scan.MkHeader {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "warning", "compile/book-in-fragment", "book only counts in the topic header; ignored")
		}
		for _, b := range scan.Categories(fd.Value) {
			if err := db.AddBook(idsrc, idtopic, b); err != nil {
				return err
			}
		}
	case "include-note-id":
		id := strings.TrimSpace(fd.Value)
		if id == "" {
			return db.AddIssue(idsrc, idseg, int64(fd.Line), "error", "compile/bad-include", "include-note-id without a note id")
		}
		if err := db.AddReference(idseg, idsrc, idtopic, scan.KindNote, scan.Key(scan.KindNote, id), "include"); err != nil {
			return err
		}
	}
	for _, l := range Ilinks(fd.Value) {
		kind, ident := l.Kind, l.Ident
		switch kind {
		case "slug":
			ident = strings.ToLower(ident)
		case "tg":
		default:
			if scan.HeaderKind(kind) == "" || kind == "class-name" {
				if err := db.AddIssue(idsrc, idseg, int64(fd.Line), "error", "compile/bad-ilink",
					fmt.Sprintf("ilink target <%s %s>: %q is not a topic kind, slug or tg", kind, ident, kind)); err != nil {
					return err
				}
				continue
			}
			ident = scan.Key(kind, ident)
		}
		if err := db.AddReference(idseg, idsrc, idtopic, kind, ident, "ilink"); err != nil {
			return err
		}
	}
	return nil
}

// Compile scans and compiles every file of paths (in the given order - that
// order is the document order and is kept in the database), reporting each
// one through log. root is the project directory: source names are stored
// relative to it. It returns the number of files compiled.
func Compile(dbPath, root string, paths []string, log func(string)) (int, error) {
	db, err := docdb.Open(dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return n, err
		}
		rel, err := filepath.Rel(absRoot, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return n, fmt.Errorf("%s is outside the project root %s", p, root)
		}
		f, err := scan.ScanFile(abs)
		if err != nil {
			return n, err
		}
		f.Name = rel
		if _, err := CompileFile(db, rel, f); err != nil {
			return n, fmt.Errorf("%s: %w", rel, err)
		}
		n++
		if log != nil {
			log(fmt.Sprintf("%s: %d topic(s), %d issue(s)", rel, len(f.Topics), len(f.Issues)))
		}
	}
	if _, err := db.PruneOrphanTopics(); err != nil {
		return n, err
	}
	if _, err := db.PruneOrphanGroups(); err != nil {
		return n, err
	}
	return n, nil
}

// ExpandSources turns source arguments into the ordered list of files to
// compile. Each argument is a file, a glob (wildcards in the last element,
// "*.*" = every file of the folder) or a directory (its documented sources:
// .cpp .c .h .hpp at the top level, .prg .ch anywhere below, C first and .ch
// last - the CLI convenience). The order of the arguments is the document
// order: nothing is re-sorted across them, and a file already taken by an
// earlier argument is discarded. A pattern matching nothing is reported
// through warn (when nil, it is an error).
func ExpandSources(args []string, warn func(string)) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		key := strings.ToLower(filepath.Clean(p))
		if abs, err := filepath.Abs(p); err == nil {
			key = strings.ToLower(abs)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, p)
	}
	for _, a := range args {
		st, err := os.Stat(a)
		if err == nil && st.IsDir() {
			names, err := dirSources(a)
			if err != nil {
				return nil, err
			}
			for _, n := range names {
				add(n)
			}
			continue
		}
		matches, err := filepath.Glob(a)
		if err != nil {
			return nil, err
		}
		var files []string
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && !st.IsDir() {
				files = append(files, m)
			}
		}
		if len(files) == 0 {
			if warn == nil {
				return nil, fmt.Errorf("no source matches %q", a)
			}
			warn(fmt.Sprintf("no source matches %q", a))
			continue
		}
		sort.SliceStable(files, func(i, j int) bool {
			return strings.ToLower(files[i]) < strings.ToLower(files[j])
		})
		for _, f := range files {
			add(f)
		}
	}
	return out, nil
}

func isCh(p string) bool { return strings.EqualFold(filepath.Ext(p), ".ch") }

func dirSources(dir string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		top := filepath.Dir(p) == filepath.Clean(dir)
		switch strings.ToLower(filepath.Ext(p)) {
		case ".cpp", ".c", ".h", ".hpp":
			if top { // C sources only at the top level: subfolders hold other things
				names = append(names, p)
			}
		case ".prg", ".ch": // Xbase++ sources may live in a subfolder (ch/)
			names = append(names, p)
		}
		return nil
	})
	sort.SliceStable(names, func(i, j int) bool {
		ci, cj := isCh(names[i]), isCh(names[j])
		if ci != cj {
			return !ci
		}
		return names[i] < names[j]
	})
	return names, err
}
