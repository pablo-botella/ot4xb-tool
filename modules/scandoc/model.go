package scandoc

import "strings"

// Kind identifies what a documented entity is.
type Kind int

const (
	// KindCFunction is an extern "C" / cdecl export of the DLL
	// (begin-c-function, header keyword c-function).
	KindCFunction Kind = iota
	// KindCppFunction is a C++-linkage export - OT4XB_API without
	// extern "C", one scope per overload (begin-cpp-function).
	KindCppFunction
	// KindFunction is an Xbase++ function, _XPP_REG_FUN_
	// (begin-function, header keyword function).
	KindFunction
	// KindClass is an Xbase++ class, possibly a GWST structure class
	// (begin-class, header keyword class-name).
	KindClass
	// KindNote is a shared note definition (begin-note); shared notes are
	// delivered as Note values, the constant exists for completeness.
	KindNote
	// KindStructure is a GWST structure class (structure:), a class plus its
	// binary gwst-members; it inherits from GWST. Draft 2 replaces the old
	// class-name + gwst-class flag for these.
	KindStructure
	// KindCppClass is a C++ class of the C/C++ manual (begin-cpp-class), the
	// class-level twin of cpp-function.
	KindCppClass
	// KindInternalFunction is an internal helper listed apart from the public
	// API - same grammar as function.
	KindInternalFunction
	// KindDebugCFunction is a c-function present only in debug builds, listed
	// apart - same grammar as c-function.
	KindDebugCFunction
	// KindMarkdownFree is a raw free-Markdown block (begin-markdown-free /
	// end-markdown-free): its body is verbatim, not parsed for markers/fields.
	KindMarkdownFree
	// KindTopic is the amorphous catch-all (begin-topic / end-topic, header
	// keyword topic): curated content that fits no structured kind. Its id is
	// normalized to lowercase; its body is free-form content (prose/lists/links),
	// kept verbatim and NOT parsed structurally. Commands live inside a topic.
	KindTopic
	// KindCommand is an Xbase++ command (multi-word names, clause variants):
	// referenced by an id, authored as content inside a topic - not a rigid
	// entity. The command carries a "topic" field naming its container.
	KindCommand
)

func (k Kind) String() string {
	switch k {
	case KindCFunction:
		return "c-function"
	case KindCppFunction:
		return "cpp-function"
	case KindFunction:
		return "function"
	case KindClass:
		return "class"
	case KindNote:
		return "note"
	case KindStructure:
		return "structure"
	case KindCppClass:
		return "cpp-class"
	case KindInternalFunction:
		return "internal-function"
	case KindDebugCFunction:
		return "debug-c-function"
	case KindMarkdownFree:
		return "markdown-free"
	case KindTopic:
		return "topic"
	case KindCommand:
		return "command"
	}
	return "unknown"
}

// Field is one "name: value" field of a header marker, with its
// continuation lines folded in (joined with one blank). Values are raw
// markdown: code spans and fences keep their delimiters, escapes stay as
// written; rendering is the generator's job.
type Field struct {
	Name    string // as written: "desc", "param nFlags", "flag 0x01", ... (matching is case-insensitive)
	Value   string
	Line    int  // line the field starts on
	Unknown bool // name is not in the known vocabulary; the field is kept and transported all the same
}

// Entity is one documented scope: begin marker, header, definition, end
// marker (or a tolerated stand-alone header, reported with a warning).
type Entity struct {
	Kind       Kind
	Ident      string      // header value: the identity key - a name, or name(TYPES) for a C++ overload
	Fields     []Field     // ordered, repeated names kept; calling forms live in the syntax field(s)
	NoteRefs   []string    // include-note-id values referenced by the scope's loose include markers
	Components []Component // class/structure auxiliaries (method, ivar, ...), in source order
	Raw        []string    // the entity's marker lines verbatim, in order (never the code between them): the segment blob

	StartLine int // begin-* marker line (stand-alone header: its first line)
	EndLine   int // end-* marker line (stand-alone header: its closing line)

	noteRefLines []int // parallel to NoteRefs: marker lines, for diagnostics
}

// Component is a class or structure auxiliary parsed from an item marker
// inside the scope: method, ivar, property, class-method, class-var,
// class-property, gwst-member. Its qualified identity is Owner:Ident (a
// method reads as Owner:Ident() by its parentheses); the owner comes from the
// enclosing scope. Fields are the descriptive fields of the item marker.
type Component struct {
	Kind   string // the item kind label: "method", "ivar", "gwst-member", ...
	Ident  string // the item value AS AUTHORED: a name, a method signature Name( ... ), or "name type: ... " for a gwst-member (the syntax/display form)
	Name   string // the base identifier - the identity anchor: Ident without the ( ... ) of a method or the trailing text of a gwst-member
	Fields []Field
	Line   int
	Raw    []string // the item marker's lines verbatim: the component's own segment blob
}

// Field returns the value of the first field with that name (case-insensitive)
// and whether it exists.
func (c *Component) Field(name string) (string, bool) {
	for i := range c.Fields {
		if strings.EqualFold(c.Fields[i].Name, name) {
			return c.Fields[i].Value, true
		}
	}
	return "", false
}

// Field returns the value of the first field with that name
// (case-insensitive) and whether it exists.
func (e *Entity) Field(name string) (string, bool) {
	for i := range e.Fields {
		if strings.EqualFold(e.Fields[i].Name, name) {
			return e.Fields[i].Value, true
		}
	}
	return "", false
}

// FieldAll returns every field with that name (case-insensitive), in order.
func (e *Entity) FieldAll(name string) []Field {
	var out []Field
	for i := range e.Fields {
		if strings.EqualFold(e.Fields[i].Name, name) {
			out = append(out, e.Fields[i])
		}
	}
	return out
}

// Note is a shared, include-by-id note: defined once in a begin-note /
// note / end-note block and pulled into scopes with the loose
// /*{{include-note-id: <id>}}*/ marker.
type Note struct {
	ID        string   // note-id, the reference key (matched case-insensitively)
	Title     string   // optional human-readable heading
	Body      string   // verbatim body lines joined with \n
	Includes  []string // note-ids this note pulls in (note->note composition, compact form)
	Slug      string   // explicit file-name stem from a `| slug:` field, "" when none
	Raw       []string // the note's marker lines verbatim (begin-note, its note: fragments, end-note; or the compact marker), never the code between: the segment blob
	StartLine int      // begin-note / note-id marker line
	EndLine   int      // end-note marker line (== StartLine for the compact form)
}

// File is the parsed model of one source file.
type File struct {
	Path     string
	Entities []Entity // in source order
	Notes    []Note   // shared note definitions, in source order
	Diags    []Diag   // warnings and errors, with line numbers
}
