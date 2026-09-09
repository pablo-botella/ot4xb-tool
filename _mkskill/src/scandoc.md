---
title: scandoc
mkskill:
  pos: 70
---

## `scandoc` — the in-source documentation scanner

ot4xb documents itself in its sources: the line that registers a thing
documents it, in `/*{{ … }}*/` comment markers next to the code. `scandoc`
parses those markers (the Draft 4 authoring model of the spec) and reports
what is wrong, without ever touching a source:

```
ot4xb-tool [-q] doc scan [-doctool <file>] -src <file|dir> [-fields] [-issues]
```

| option | meaning |
|---|---|
| `-doctool` | the `.doc-tool` file whose kinds the scanner recognises (default: the built-in nine) |
| `-src` | one file, or a directory: its `.cpp/.c/.h/.hpp` files, then the `.prg/.ch` of it and its subfolders (`ch/`), sorted |
| `-fields` | print every field of every marker, not only the topics |
| `-issues` | print only the issues |

Output: one line per topic (`start-end  kind  identity  (compact|composed,
N markers)`), then every issue as `file:line: severity: code: message` so an
editor can jump to it. Exit code 1 when any file has an error. Sources are
read as bytes (Windows-1252) and any line ending is accepted; a file whose
lines are not all CRLF gets one warning (CRLF is the convention, required in
Xbase++ sources).

### The model, in short

Everything documented is a **topic** — a page, an identity, a link target —
of one of nine kinds:

| kind | identity | world |
|---|---|---|
| `function`, `internal-function` | the Xbase++ name | Xbase++ (case-insensitive) |
| `c-function`, `debug-c-function` | the C symbol | C (case-sensitive) |
| `cpp-function` | `name(param-types)`, one topic per overload | C++ (case-sensitive) |
| `class` (header label `class-name`) | the class name; a GWST structure is a class too | Xbase++ |
| `cpp-class` | the C++ class | C++ |
| `note-id` | a doc id; a note is a topic used by `include-note-id` | doc (lower-case) |
| `topic` | a doc id; an amorphous page (a chapter, a file header, a command set) | doc (lower-case) |

**Everything else is content** of the enclosing topic — members, methods,
properties, commands, parameters, notes — never a topic and never a link
target. A topic is either **compact** (one marker holds it all) or
**composed**: `/*{{begin-<kind>}}*/`, its header marker, the real code with
the content markers next to the lines they document, `/*{{end-<kind>}}*/`.
Content outside a topic is an error.

```
/*{{begin-class}}*/
/*{{class-name_: WAPIST_POINT
            | _slug_: wapist_point
            | class-function: WAPIST_POINT
            | parent: {{ilink: <class gwst> gwst}}
            | category: winapi/structures
            | desc: Wrapper over the WinApi POINT structure.
   }}*/
/*{{|:**BEGIN STRUCTURE  POINT** }}*/
XB_BEGIN_STRUCTURE ( POINT )
   /*{{|member_: - MEMBER LONG x | desc_: x coordinate. }}*/
   _XBST_LONG ( x )
XB_END_STRUCTURE
/*{{|:**END STRUCTURE** }}*/
/*{{include-note-id: wapist-map}}*/
/*{{end-class}}*/
```

- A marker is `label: value | label: value …`, one or more lines: a field
  starts at a `|` that begins a line or follows a blank, outside backticks,
  followed by an optional label and `:`. A `|` inside a code span or a fenced
  block is text. Continuation lines are kept verbatim.
- **Values are written in a small, strict subset of Markdown**, parsed by
  the tool itself: ATX headings `#` to `#####`, paragraphs, `- ` list items
  (never nested), fenced code blocks (three backticks, an optional language
  after the opening fence), `**strong**`, code spans (one backtick), and the
  `{{…}}` calls of this grammar. Nothing else means anything: `_`, `*`, `~`,
  `<`, `\` and `|` are plain text, so identifiers like `_LARGE_INTEGER_`,
  placeholders like `<memberName>` and paths like `\\server\share` survive
  as written. What falls outside - a sixth `#`, an unclosed `**` or code
  span, an unclosed fence, a nested `- `, a `#` at the start of a
  continuation line - is an error with its file and line, and nothing is
  generated: the tool never guesses. Every renderer escapes for its own
  output (HTML, Markdown), nothing is escaped in the sources.
- The header's first entry is the identity (`kind: ident`). A marker that
  starts with `|` is a **fragment** of the open topic; `|:` is text placed
  right there. `include-note-id: X` transcludes note X at that position.
- `{{begin-md}}` … `{{end-md}}` inside any value (inline, like `{{ilink}}`)
  is the one place where **full Markdown** is allowed - tables, emphasis,
  anything goldmark understands - and the one place where escaping is the
  author's business: the block is handed to goldmark as it is, on its own
  (a reference link or a footnote defined outside it does not exist for it).
  Between the two marks no `|` starts a field, so a table goes through; the
  block loses only the indentation common to its lines, the inline
  `{{ilink: …}}` still work there, and the two marks themselves are not
  rendered. `{{begin-md: raw}}` keeps the text byte for byte instead: no
  dedent, nothing replaced — the way to indent on purpose.
- **Visibility** is in the label's first and last underscore: `desc_` shows
  the value without its label, `_todo` hides the whole entry, `_slug_` is
  hidden both ways but still a field the tool reads. Only the first and the
  last underscore count; they are stripped before the label is recognized.
- The only labels the tool interprets: the identity, `slug` (the page's file
  name; computed from kind and key when absent), `tg` (topic group: every
  topic with the same `_tg_` renders into one page — the C++ overloads),
  `category` (comma list allowed) and `include-note-id`. Everything else
  renders as written, in written order.
- **Links**: `{{ilink: <kind ident> text}}`, `{{ilink: <slug name> text}}`,
  `{{ilink: <tg name> text}}` inside any value; the kind is exact (a
  `function` and a `c-function` of the same name are two topics). Markdown
  links stay for external URLs.
- **Scattered content**: a later block with the same identity — same file or
  another — adds its content to the same topic, in parse order. It carries
  the identity and the new content only; nothing already written is repeated.

### What it checks

Marker grammar (unterminated markers, unknown kinds, a head that is not a
topic kind, stray or mismatched `begin`/`end`, a scope without a header or
with two, content before the header or outside any topic), plus two
warnings: the CRLF one, and a backtick left open on a line (a code span
does not cross a line; an odd backtick would hide every field after it, so
a literal backtick is written as text, never as `` '`' ``). Cross-file checks — missing link targets, include cycles, slug
clashes — belong to `resolve`, over the compiled database.
