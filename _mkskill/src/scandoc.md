---
title: scandoc
mkskill:
  pos: 70
---

## `scandoc` — the in-source documentation scanner

ot4xb documents itself in its C/C++ sources: the line that registers a thing
documents it, in `/*{{ … }}*/` comment markers next to the code. `scandoc`
parses those markers into a model and reports what is wrong, without ever
touching a source:

```
ot4xb-tool [-q] scandoc -src <file|dir> [-fields] [-tags [file]]
```

| option | meaning |
|---|---|
| `-src` | one file, or a directory: its `.cpp/.c/.h/.hpp` files (not recursive), in name order |
| `-fields` | print every field of every entity, not only the identities |
| `-tags` | tag inventory (debug): alone it prints the vocabulary in use; with a file it writes every occurrence there, one per line |

Output: one line per documented entity (`start-end  kind  identity`, with its
components below), then the diagnostics as `file:line: severity: message` so
an editor can jump to them. Sources are read as bytes (Windows-1252) and any
line ending is accepted; a file whose lines are not all CRLF gets one warning
(CRLF is the convention, required in Xbase++ sources).

### The grammar, in short

A marker is `/*{{ label: value | field: value | … }}*/`, one or more lines; a
`|` starts a field, and a `|` inside a backtick code span is text. The first
label names the **entity kind** and its identity:

| kind | identity | world |
|---|---|---|
| `function`, `internal-function` | the Xbase++ name | Xbase++ (case-insensitive) |
| `c-function`, `debug-c-function` | the C symbol | C (case-sensitive) |
| `cpp-function` | `[ns::]name(param-types)`, one per overload | C++ (case-sensitive) |
| `class` (label `class-name`), `structure` | the class name; a structure is a class plus its binary `gwst-member`s | Xbase++ |
| `cpp-class` | the C++ class | C++ |
| `topic` | a doc-internal id; its body is free content | doc (lower-case) |
| `command` | an id; lives inside a topic | Xbase++ |

An entity is either **compact** (everything inline in one marker) or a
**scope**: `/*{{begin-<kind>}}*/`, the header marker, the real code, and
`/*{{end-<kind>}}*/`. Inside a class or structure scope the auxiliaries are
their own markers — `method`, `ivar`, `property`, `class-method`, `class-var`,
`class-property`, `gwst-member` — identified as `Class:member` (a method
keeps its authored signature for display; the name before `(` is the
identity).

Fields are an open vocabulary (`syntax`, `desc`, `param x`, `return`,
`example`, `see-also`, `category`, `since`, `deprecated`, `parent`, `ilink`,
…): unknown names are kept and transported, never dropped. Values are
Markdown; a ``` fence keeps its content verbatim.

**Shared notes** are written once and pulled in by id: the compact form
`/*{{note-id: X |: body | note: caveat | include-note-id: dep}}*/`, or a
composed block `/*{{begin-note | note-id: X}}*/` … `/*{{end-note}}*/` whose
`/*{{note: …}}*/` fragments, scattered through the code they annotate, make
up the body in order. An entity includes a note with a loose
`/*{{include-note-id: X}}*/` inside its scope. `| ilink: <kind id> text` is
an internal link to any entity by kind and identity. A
`/*{{begin-markdown-free}}*/` … `/*{{end-markdown-free}}*/` block is raw
content nothing inside is parsed.

### What it checks

Grammar errors (unclosed markers, mismatched scopes, missing identities,
duplicate identities per kind, duplicate `mangled-name`), lint (line length,
non-ASCII, non-CRLF), migration debt (`todo` fields, retired forms), and —
across the whole directory — that every `include-note-id` and `ilink` target
exists, that note inclusion has no cycles, and that no non-reopenable identity
is defined twice.
