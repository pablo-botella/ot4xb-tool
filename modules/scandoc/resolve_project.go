package scandoc

import (
	"fmt"
	"strings"
)

// linkEntityKind maps an ilink kind token to the entity Kind it names. The
// component kinds (itemKinds) and note are resolved apart.
var linkEntityKind = map[string]Kind{
	"function":          KindFunction,
	"c-function":        KindCFunction,
	"cpp-function":      KindCppFunction,
	"internal-function": KindInternalFunction,
	"debug-c-function":  KindDebugCFunction,
	"class":             KindClass,
	"structure":         KindStructure,
	"cpp-class":         KindCppClass,
	"topic":             KindTopic,
	"command":           KindCommand,
}

// entRef and compRef point back to the file and location an identity was
// defined at, for cross-file duplicate diagnostics.
type entRef struct {
	file string
	line int
}

// ProjectIndex is the whole-project identity graph: every entity, class/
// structure component and shared note, keyed for O(1) reference resolution.
type ProjectIndex struct {
	entities map[string]entRef // key: kind|identKey
	comps    map[string]entRef // key: compkind|owner:name  (Xbase++-folded)
	notes    map[string]entRef // key: lowercased note-id
}

// reopenable reports whether a kind may be documented across several begin-*
// blocks with the same identity (partial-class style, merged by the tool).
func reopenable(k Kind) bool {
	return k == KindClass || k == KindStructure || k == KindCppClass
}

// entKey / compKey / noteKey build the map keys (all normalization in one place).
func entKey(k Kind, id string) string { return k.String() + "|" + identKey(k, id) }

// compRefKey builds a component reference key from an ilink id "Owner:member".
// Owner and member fold to lowercase (ot4xb components live on Xbase++ classes/
// structures); the "()" of a method is display, never part of the id.
func compRefKey(compKind, ownerMember string) string {
	om := strings.ToLower(strings.TrimSpace(ownerMember))
	if before, _, ok := strings.Cut(om, "("); ok {
		om = strings.TrimSpace(before)
	}
	return strings.ToLower(compKind) + "|" + om
}

// compDefKey builds a component definition key from its owner entity and the
// component itself (using the base Name, not the authored signature).
func compDefKey(owner *Entity, c *Component) string {
	return compRefKey(c.Kind, owner.Ident+":"+c.Name)
}

// BuildIndex indexes every referenceable identity across the files and reports
// cross-file duplicates (same kind+id, or same note-id, defined twice) onto the
// owning files' Diags.
func BuildIndex(files []*File) *ProjectIndex {
	ix := &ProjectIndex{
		entities: map[string]entRef{},
		comps:    map[string]entRef{},
		notes:    map[string]entRef{},
	}
	for _, f := range files {
		for i := range f.Entities {
			e := &f.Entities[i]
			if e.Ident != "" && e.Kind != KindMarkdownFree {
				k := entKey(e.Kind, e.Ident)
				if prev, dup := ix.entities[k]; dup {
					// class/structure/cpp-class may REOPEN (documented across
					// several begin-* blocks with the same identity, merged); the
					// others must be unique.
					if !reopenable(e.Kind) {
						f.Diags = append(f.Diags, Diag{Severity: Error, Line: e.StartLine,
							Msg: fmt.Sprintf("%s %q already defined in %s:%d", e.Kind, e.Ident, prev.file, prev.line)})
					}
				} else {
					ix.entities[k] = entRef{f.Path, e.StartLine}
				}
			}
			for ci := range e.Components {
				c := &e.Components[ci]
				if c.Name == "" {
					continue
				}
				k := compDefKey(e, c)
				if prev, dup := ix.comps[k]; dup {
					f.Diags = append(f.Diags, Diag{Severity: Error, Line: c.Line,
						Msg: fmt.Sprintf("%s %s:%s already defined in %s:%d", c.Kind, e.Ident, c.Name, prev.file, prev.line)})
				} else {
					ix.comps[k] = entRef{f.Path, c.Line}
				}
			}
		}
		for i := range f.Notes {
			n := &f.Notes[i]
			if n.ID == "" {
				continue
			}
			k := strings.ToLower(n.ID)
			if prev, dup := ix.notes[k]; dup {
				f.Diags = append(f.Diags, Diag{Severity: Error, Line: n.StartLine,
					Msg: fmt.Sprintf("note-id %q already defined in %s:%d", n.ID, prev.file, prev.line)})
			} else {
				ix.notes[k] = entRef{f.Path, n.StartLine}
			}
		}
	}
	return ix
}

// hasEntity / hasComp / hasNote answer reference lookups.
func (ix *ProjectIndex) hasEntity(k Kind, id string) bool {
	_, ok := ix.entities[entKey(k, id)]
	return ok
}
func (ix *ProjectIndex) hasNote(id string) bool { _, ok := ix.notes[strings.ToLower(id)]; return ok }

// resolveLink reports whether an ilink target <kind id> exists in the graph;
// ok is false only for an unknown kind token (structurally checked earlier).
func (ix *ProjectIndex) resolveLink(kind, id string) (found, ok bool) {
	kl := strings.ToLower(kind)
	if k, isEnt := linkEntityKind[kl]; isEnt {
		return ix.hasEntity(k, id), true
	}
	if itemKinds[kl] {
		_, found = ix.comps[compRefKey(kl, id)]
		return found, true
	}
	if kl == "note" {
		return ix.hasNote(id), true
	}
	return false, false
}

// ResolveProject is the whole-project resolver: it builds the index (reporting
// duplicates), then verifies every include-note-id and every ilink target
// against it, and detects cycles in note->note include chains. All findings are
// appended to the files' Diags. It supersedes the single-file Resolve for a
// multi-file scan.
func ResolveProject(files []*File) *ProjectIndex {
	ix := BuildIndex(files)
	used := map[string]bool{}
	for _, f := range files {
		for i := range f.Entities {
			e := &f.Entities[i]
			// include-note-id references (project-wide)
			for j, id := range e.NoteRefs {
				line := e.StartLine
				if j < len(e.noteRefLines) {
					line = e.noteRefLines[j]
				}
				if ix.hasNote(id) {
					used[strings.ToLower(id)] = true
				} else {
					f.Diags = append(f.Diags, Diag{Severity: Error, Line: line,
						Msg: fmt.Sprintf("include-note-id %q has no matching begin-note in the project", id)})
				}
			}
			// ilink targets on the entity and its components
			ix.resolveEntityLinks(f, e)
			for ci := range e.Components {
				ix.resolveFieldLinks(f, e.Components[ci].Fields)
			}
		}
	}
	ix.checkNoteCycles(files, used)
	return ix
}

// resolveEntityLinks / resolveFieldLinks verify the ilink fields of an entity
// or a component against the index (structure was checked at scan time).
func (ix *ProjectIndex) resolveEntityLinks(f *File, e *Entity) { ix.resolveFieldLinks(f, e.Fields) }

func (ix *ProjectIndex) resolveFieldLinks(f *File, fields []Field) {
	for _, fd := range fields {
		if !strings.EqualFold(fd.Name, "ilink") {
			continue
		}
		kind, id, _, ok := ParseILink(fd.Value)
		if !ok || kind == "" || id == "" {
			continue // malformed: reported at scan time
		}
		found, known := ix.resolveLink(kind, id)
		if known && !found {
			f.Diags = append(f.Diags, Diag{Severity: Error, Line: fd.Line,
				Msg: fmt.Sprintf("ilink target <%s %s> is not defined in the project", kind, id)})
		}
	}
}

// checkNoteCycles builds the note->note include graph (a note body may embed
// include-note-id markers) and reports any cycle, plus notes nothing includes.
func (ix *ProjectIndex) checkNoteCycles(files []*File, used map[string]bool) {
	type noteInfo struct {
		file string
		line int
		deps []string
	}
	notes := map[string]noteInfo{}
	for _, f := range files {
		for i := range f.Notes {
			n := &f.Notes[i]
			if n.ID == "" {
				continue
			}
			var deps []string
			// note->note dependencies: the compact form's include-note-id fields
			// (n.Includes) plus any {{include-note-id: X}} embedded in the body.
			rawDeps := append(append([]string{}, n.Includes...), extractIncludes(n.Body)...)
			for _, dep := range rawDeps {
				dl := strings.ToLower(dep)
				deps = append(deps, dl)
				used[dl] = true // a note included by another note is "used"
				if !ix.hasNote(dep) {
					f.Diags = append(f.Diags, Diag{Severity: Error, Line: n.StartLine,
						Msg: fmt.Sprintf("note %q includes note-id %q, which has no matching begin-note", n.ID, dep)})
				}
			}
			notes[strings.ToLower(n.ID)] = noteInfo{f.Path, n.StartLine, deps}
		}
	}
	// DFS cycle detection over the note dependency graph.
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var visit func(id string)
	visit = func(id string) {
		color[id] = gray
		stack = append(stack, id)
		for _, dep := range notes[id].deps {
			if _, ok := notes[dep]; !ok {
				continue // dangling: already reported
			}
			switch color[dep] {
			case white:
				visit(dep)
			case gray:
				ni := notes[id]
				files0(files, ni.file).Diags = append(files0(files, ni.file).Diags, Diag{
					Severity: Error, Line: ni.line,
					Msg: fmt.Sprintf("note include cycle: %s -> %s", strings.Join(stack, " -> "), dep)})
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
	}
	for id := range notes {
		if color[id] == white {
			visit(id)
		}
	}
	// notes nothing includes (project-wide): a lint warning.
	for id, ni := range notes {
		if !used[id] {
			files0(files, ni.file).Diags = append(files0(files, ni.file).Diags, Diag{
				Severity: Warning, Line: ni.line,
				Msg: fmt.Sprintf("shared note %q is never included in the project", id)})
		}
	}
}

// files0 returns the *File with that path (for appending a diagnostic).
func files0(files []*File, path string) *File {
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	return files[0]
}
