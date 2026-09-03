// Package doccompile is the ot4xb layer of the documentation database: it
// takes the model scandoc parses out of a source file and writes it into a
// docdb (spec 03-srcdoc-spec, "Draft 3 - the intermediate database"). This is
// the only place that knows the ot4xb kinds, the key rule per family and how
// each marker becomes a reference; the database core stays generic.
//
// One call per source file = one compile pass: register the source (it keeps
// its idsrc and pos across passes), delete what it contributed before, insert
// its topics, segments, references, categories and issues. Nothing is resolved
// here: resolution is a separate, later step.
package doccompile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/scandoc"
)

// Key normalizes an identity into the topic key of its family: Xbase++
// symbols upper-case (their canonical form in the xbmac and the export
// table), C/C++ symbols as written (case-sensitive), doc-internal ids
// lower-case. Blanks never count.
func Key(kind, ident string) string {
	id := stripBlanks(ident)
	switch kind {
	case "note", "topic", "markdown-free":
		return strings.ToLower(id)
	case "c-function", "cpp-function", "cpp-class", "debug-c-function":
		return id
	default: // function, internal-function, class, structure, command, components
		return strings.ToUpper(id)
	}
}

func stripBlanks(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// caseSensitiveOwner reports whether components of this owner kind keep case.
func caseSensitiveOwner(kind string) bool { return kind == "cpp-class" }

// componentKey builds Owner:Name in the owner's family rule.
func componentKey(ownerKind, owner, name string) string {
	if caseSensitiveOwner(ownerKind) {
		return stripBlanks(owner) + ":" + stripBlanks(name)
	}
	return strings.ToUpper(stripBlanks(owner)) + ":" + strings.ToUpper(stripBlanks(name))
}

// construct is one segment-to-be, in file order.
type construct struct {
	line int
	emit func(seq int64) error
}

// CompileFile writes one scanned file into the database. src is the path
// relative to the project root (already normalized by docdb.Source).
func CompileFile(db *docdb.DB, src string, f *scandoc.File) (idsrc int64, err error) {
	idsrc, srcPos, _, err := db.Source(src)
	if err != nil {
		return 0, err
	}
	if err := db.ReplaceSource(idsrc); err != nil {
		return 0, err
	}
	var cs []construct

	for i := range f.Entities {
		e := &f.Entities[i]
		if e.Ident == "" && e.Kind != scandoc.KindMarkdownFree {
			continue // no identity: the scanner already reported it
		}
		cs = append(cs, construct{e.StartLine, func(seq int64) error { return emitEntity(db, idsrc, srcPos, seq, f, e) }})
		for ci := range e.Components {
			c := &e.Components[ci]
			cs = append(cs, construct{c.Line, func(seq int64) error { return emitComponent(db, idsrc, srcPos, seq, e, c) }})
		}
	}
	for i := range f.Notes {
		n := &f.Notes[i]
		if n.ID == "" {
			continue
		}
		cs = append(cs, construct{n.StartLine, func(seq int64) error { return emitNote(db, idsrc, srcPos, seq, n) }})
	}
	// the parse order is the document order: number the segments by line
	sort.SliceStable(cs, func(a, b int) bool { return cs[a].line < cs[b].line })
	for i, c := range cs {
		if err := c.emit(int64(i + 1)); err != nil {
			return idsrc, fmt.Errorf("%s: %w", src, err)
		}
	}
	for _, d := range f.Diags {
		sev := "warning"
		if d.Severity == scandoc.Error {
			sev = "error"
		}
		if err := db.AddIssue(idsrc, 0, int64(d.Line), sev, "", d.Msg); err != nil {
			return idsrc, err
		}
	}
	return idsrc, nil
}

func emitEntity(db *docdb.DB, idsrc, srcPos, seq int64, f *scandoc.File, e *scandoc.Entity) error {
	kind := e.Kind.String()
	ident := e.Ident
	if e.Kind == scandoc.KindMarkdownFree {
		ident = fmt.Sprintf("%s#%d", filepath.Base(f.Path), e.StartLine) // a raw block has no name of its own
	}
	tp, err := db.Topic(kind, Key(kind, ident), ident)
	if err != nil {
		return err
	}
	seg, err := db.AddSegment(tp, idsrc, docdb.PackPos(srcPos, seq), int64(e.StartLine), rawOf(e.Raw))
	if err != nil {
		return err
	}
	ref := func(toKind, toIdent, refType string) error {
		return db.AddReference(seg, idsrc, tp, toKind, strings.TrimSpace(toIdent), refType)
	}
	// loose /*{{include-note-id: X}}*/ markers (and the retired field form)
	for _, id := range e.NoteRefs {
		if err := ref("note", id, "include"); err != nil {
			return err
		}
	}
	if err := emitFieldRefs(e.Fields, ref); err != nil {
		return err
	}
	// a command lives in a topic
	if e.Kind == scandoc.KindCommand {
		if v, ok := e.Field("topic"); ok && v != "" {
			if err := ref("topic", v, "in-topic"); err != nil {
				return err
			}
		}
	}
	for _, fd := range e.FieldAll("category") {
		for _, c := range splitList(fd.Value) {
			if err := db.AddCategory(idsrc, tp, c); err != nil {
				return err
			}
		}
	}
	return nil
}

func emitComponent(db *docdb.DB, idsrc, srcPos, seq int64, owner *scandoc.Entity, c *scandoc.Component) error {
	if c.Name == "" {
		return nil
	}
	ownerKind := owner.Kind.String()
	ident := owner.Ident + ":" + c.Name
	tp, err := db.Topic(c.Kind, componentKey(ownerKind, owner.Ident, c.Name), ident)
	if err != nil {
		return err
	}
	seg, err := db.AddSegment(tp, idsrc, docdb.PackPos(srcPos, seq), int64(c.Line), rawOf(c.Raw))
	if err != nil {
		return err
	}
	ref := func(toKind, toIdent, refType string) error {
		return db.AddReference(seg, idsrc, tp, toKind, strings.TrimSpace(toIdent), refType)
	}
	return emitFieldRefs(c.Fields, ref)
}

func emitNote(db *docdb.DB, idsrc, srcPos, seq int64, n *scandoc.Note) error {
	tp, err := db.Topic("note", Key("note", n.ID), n.ID)
	if err != nil {
		return err
	}
	seg, err := db.AddSegment(tp, idsrc, docdb.PackPos(srcPos, seq), int64(n.StartLine), rawOf(n.Raw))
	if err != nil {
		return err
	}
	for _, dep := range n.Includes {
		if err := db.AddReference(seg, idsrc, tp, "note", strings.TrimSpace(dep), "include"); err != nil {
			return err
		}
	}
	return nil
}

// emitFieldRefs turns the reference-bearing fields of an entity or component
// into rows: parent (inheritance), ilink (qualified link), see-also and calls
// (unqualified names, stored as such - not resolved for now).
func emitFieldRefs(fields []scandoc.Field, ref func(toKind, toIdent, refType string) error) error {
	for _, fd := range fields {
		switch strings.ToLower(fd.Name) {
		case "parent":
			// parent: NAME | A, B, C (normal, non-linear) | gwst,NAME (linear)
			v := strings.TrimSpace(fd.Value)
			if rest, ok := cutPrefixFold(v, "gwst,"); ok {
				if err := ref("class", rest, "gwst-parent"); err != nil {
					return err
				}
				continue
			}
			for _, p := range splitList(v) {
				if err := ref("class", p, "parent"); err != nil {
					return err
				}
			}
		case "ilink":
			kind, id, _, ok := scandoc.ParseILink(fd.Value)
			if !ok || kind == "" || id == "" {
				continue // malformed: the scanner reported it
			}
			if err := ref(strings.ToLower(kind), id, "ilink"); err != nil {
				return err
			}
		case "see-also", "calls":
			for _, name := range splitList(fd.Value) {
				if err := ref("", name, strings.ToLower(fd.Name)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return strings.TrimSpace(s[len(prefix):]), true
	}
	return s, false
}

// splitList splits a comma-separated list of names, trimming blanks and
// dropping empties.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// rawOf joins the captured marker lines with CRLF - the blob is source text.
func rawOf(lines []string) []byte { return []byte(strings.Join(lines, "\r\n")) }

// Compile scans and compiles every file of paths (in the given order - that
// order becomes the document order for new sources) into the database at
// dbPath, registering each under its path relative to root. It returns the
// number of files compiled.
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
			return n, fmt.Errorf("%s is not under the project root %s", p, root)
		}
		f, err := scandoc.Scan(abs)
		if err != nil {
			return n, err
		}
		idsrc, err := CompileFile(db, rel, f)
		if err != nil {
			return n, err
		}
		n++
		if log != nil {
			log(fmt.Sprintf("compile: %s -> source %d: %d entities, %d notes, %d issues",
				filepath.ToSlash(rel), idsrc, len(f.Entities), len(f.Notes), len(f.Diags)))
		}
	}
	if pruned, err := db.PruneOrphanTopics(); err != nil {
		return n, err
	} else if pruned > 0 && log != nil {
		log(fmt.Sprintf("compile: %d orphan topic(s) pruned", pruned))
	}
	return n, nil
}

// ExpandSources turns -src arguments (files, globs, or directories - a
// directory means its C/C++ sources, like scandoc) into the ordered list of
// files to compile. Within one argument the files are sorted by name so the
// order is reproducible; the arguments themselves keep the order given.
func ExpandSources(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		st, err := os.Stat(a)
		if err == nil && st.IsDir() {
			entries, err := os.ReadDir(a)
			if err != nil {
				return nil, err
			}
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				switch strings.ToLower(filepath.Ext(e.Name())) {
				case ".cpp", ".c", ".h", ".hpp":
					names = append(names, filepath.Join(a, e.Name()))
				}
			}
			sort.Strings(names)
			out = append(out, names...)
			continue
		}
		matches, err := filepath.Glob(a)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no source matches %q", a)
		}
		sort.Strings(matches)
		out = append(out, matches...)
	}
	return out, nil
}
