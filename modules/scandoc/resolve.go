package scandoc

import (
	"fmt"
	"strings"
)

// NoteByID returns the shared note with that note-id (case-insensitive),
// or nil.
func (f *File) NoteByID(id string) *Note {
	for i := range f.Notes {
		if strings.EqualFold(f.Notes[i].ID, id) {
			return &f.Notes[i]
		}
	}
	return nil
}

// Resolve is the resolver step: it verifies every include-note-id
// reference of every entity against the file's shared notes. A dangling
// include is an error; a defined note that nothing includes is a warning.
// Findings are appended to f.Diags.
func (f *File) Resolve() {
	used := make(map[string]bool)
	for i := range f.Entities {
		e := &f.Entities[i]
		for j, id := range e.NoteRefs {
			line := e.StartLine
			if j < len(e.noteRefLines) {
				line = e.noteRefLines[j]
			}
			if f.NoteByID(id) == nil {
				f.Diags = append(f.Diags, Diag{Severity: Error, Line: line,
					Msg: fmt.Sprintf("include-note-id %q has no matching begin-note", id)})
			} else {
				used[strings.ToLower(id)] = true
			}
		}
	}
	for i := range f.Notes {
		n := &f.Notes[i]
		if n.ID != "" && !used[strings.ToLower(n.ID)] {
			f.Diags = append(f.Diags, Diag{Severity: Warning, Line: n.StartLine,
				Msg: fmt.Sprintf("shared note %q is never included", n.ID)})
		}
	}
}

// ExpandNotes returns value with every resolvable {{include-note-id: <id>}}
// occurrence replaced by that note's body; unresolved includes are left as
// written (Resolve reports them). This serves the retired field form during
// the migration; composing the loose include markers is the generator's
// job.
func (f *File) ExpandNotes(value string) string {
	var sb strings.Builder
	i := 0
	for {
		id, start, end := findInclude(value, i)
		if start < 0 {
			sb.WriteString(value[i:])
			return sb.String()
		}
		sb.WriteString(value[i:start])
		if n := f.NoteByID(id); n != nil {
			sb.WriteString(n.Body)
		} else {
			sb.WriteString(value[start:end])
		}
		i = end
	}
}

// linkKinds is the vocabulary of kind tokens valid inside an ilink target
// <kind id>: every entity kind plus the class/structure component kinds and
// note. Matched case-insensitively.
var linkKinds = map[string]bool{
	"function": true, "c-function": true, "cpp-function": true,
	"internal-function": true, "debug-c-function": true,
	"class": true, "structure": true, "cpp-class": true,
	"topic": true, "command": true, "note": true,
	"method": true, "ivar": true, "property": true,
	"class-method": true, "class-var": true, "class-property": true,
	"gwst-member": true,
}

// ParseILink parses an ilink field value "<kind id> display text": kind is the
// first token inside the angle brackets, id is the rest (a name, name(types),
// Class:member, ...), and the trailing text is the display text - the id itself
// when none is given. ok is false when the value does not open with <...>.
func ParseILink(value string) (kind, id, text string, ok bool) {
	v := strings.TrimSpace(value)
	if !strings.HasPrefix(v, "<") {
		return "", "", "", false
	}
	j := strings.IndexByte(v, '>')
	if j < 0 {
		return "", "", "", false
	}
	inner := strings.TrimSpace(v[1:j])
	kind, id, _ = strings.Cut(inner, " ")
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	text = strings.TrimSpace(v[j+1:])
	if text == "" {
		text = id
	}
	return kind, id, text, true
}

// checkILink structurally validates one ilink field value and reports lint on
// it; target existence is the project resolver's job (a later pass over all
// files). Reported findings are appended to f.Diags.
func (f *File) checkILink(value string, line int) {
	kind, id, _, ok := ParseILink(value)
	if !ok {
		f.Diags = append(f.Diags, Diag{Severity: Error, Line: line,
			Msg: "ilink value must start with <kind id>"})
		return
	}
	if kind == "" || id == "" {
		f.Diags = append(f.Diags, Diag{Severity: Error, Line: line,
			Msg: fmt.Sprintf("ilink target <%s %s> is missing its kind or id", kind, id)})
		return
	}
	if !linkKinds[strings.ToLower(kind)] {
		f.Diags = append(f.Diags, Diag{Severity: Error, Line: line,
			Msg: fmt.Sprintf("ilink has unknown kind %q", kind)})
	}
}

// extractIncludes returns every include-note-id referenced with the
// {{include-note-id: <id>}} form inside a field value (the retired form,
// kept working during the migration). No regular expressions: a hand
// scan, like everything else in the package.
func extractIncludes(value string) []string {
	var ids []string
	for i := 0; ; {
		id, start, end := findInclude(value, i)
		if start < 0 {
			return ids
		}
		if id != "" {
			ids = append(ids, id)
		}
		i = end
	}
}

// findInclude locates the first {{include-note-id: <id>}} at or after
// position from; start is -1 when there is none.
func findInclude(v string, from int) (id string, start, end int) {
	const kw = "include-note-id"
	for {
		j := strings.Index(v[from:], "{{")
		if j < 0 {
			return "", -1, -1
		}
		j += from
		k := skipBlanks(v, j+2)
		if k+len(kw) <= len(v) && strings.EqualFold(v[k:k+len(kw)], kw) {
			k = skipBlanks(v, k+len(kw))
			if k < len(v) && v[k] == ':' {
				if m := strings.Index(v[k+1:], "}}"); m >= 0 {
					return strings.TrimSpace(v[k+1 : k+1+m]), j, k + 1 + m + 2
				}
			}
		}
		from = j + 2
	}
}

func skipBlanks(v string, i int) int {
	for i < len(v) && (v[i] == ' ' || v[i] == '\t') {
		i++
	}
	return i
}
