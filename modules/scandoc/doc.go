// Package scandoc scans C/C++ sources for the ot4xb in-source documentation
// markers (/*{{ ... }}*/ comments) and returns a structured, validated model
// of the documented entities. It is the front end that feeds the later doc
// generation legs (README / reference / changelog); the authoritative
// grammar is the living srcdoc specification (03-srcdoc-spec.md in the
// go-port notes).
//
// The scanner is a finite state machine that walks the file line by line,
// built on plain string operations only - no regular expressions anywhere.
// It reads only the comments: the C++ code between markers is opaque text
// and is never modified (the package is strictly read-only). Sources are
// Windows files - Windows-1252 encoding, CRLF line endings - so bytes are
// read, decoded from 1252 and split on line endings keeping the original
// line numbers for every diagnostic.
//
// A documented entity is a scope: an opening /*{{begin-<kind>}}*/ marker, a
// header marker /*{{<keyword>: ...}}*/, the real C/C++ definition, and the
// closing /*{{end-<kind>}}*/ marker. The scope kinds are c-function,
// cpp-function, function, class (header keyword class-name), structure,
// cpp-class, internal-function, debug-c-function and topic; a scope-less
// (compact) header is the blessed all-inline form. The header value is the
// entity's identity key - a plain name, or name(TYPES) for one C++ overload -
// and the calling forms live in syntax fields. Field values are raw markdown:
// CommonMark code spans and ``` / ~~~ fences pass through verbatim, delimiters
// included, and a '|' inside them is text, not a field separator.
//
// A topic is the amorphous catch-all: its body is free-form content, kept
// verbatim and not parsed structurally; a command authored inside a topic
// (/*{{command: id ...}}*/) becomes a referenceable entity carrying a "topic"
// field. A markdown-free block (begin-markdown-free / end-markdown-free) is a
// raw fence: nothing inside it is parsed.
//
// Shared notes are defined once as begin-note / note / end-note blocks and
// pulled into a scope with the loose /*{{include-note-id: <id>}}*/ marker
// between its begin and end; Resolve verifies the references. Scan parses
// one file, ScanDir a directory of sources; problems come back as Diags
// (errors and lint warnings) with their line numbers, never as a parse
// abort. Item markers that document class/structure auxiliaries (method,
// property, ivar, gwst-member, class-method, class-var, class-property) inside
// a scope are parsed into that entity's Components (qualified identity
// Owner:name); stand-alone fragment markers (a loose desc/param/note/flag)
// are still skipped.
package scandoc
