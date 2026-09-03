# Prompt: build the `scandoc` FSM parser inside ot4xb-tool

## Goal

Add a **`scandoc`** module to the **ot4xb-tool** Go project. It scans C/C++
source files, extracts the in-source documentation markers written as
`/*{{ ... }}*/` comments, and returns a structured, validated model of the
documented entities. The parser is a **finite state machine (FSM)** that walks
the file line by line. `scandoc` is **read-only**: it never modifies sources.

The authoritative grammar is the living spec
`dev-tools/go-port/03-srcdoc-spec.md`. This prompt summarizes it so the parser
can be built without re-reading everything, but on any conflict the spec wins.
Real files already carrying markers (good fixtures/tests): `fpCall.cpp`,
`DrTool.cpp`, `FileTime.cpp` (under the ot4xb `source/` tree).

## Input / encoding

- Files are Windows source: **Windows-1252** encoding, **CRLF** line endings.
  Read bytes, decode 1252, split on CRLF; keep original line numbers.
- Markers live **inside** C comments `/* ... */`. A key invariant: **every line
  of documentation text must be inside a comment** (a balanced `/*`..`*/` count
  is NOT proof of that — see the shared-note body rule below).
- Marker/field lines should be <= 120 chars (140 tolerable inside code
  fences). This is a lint rule to report, not a parse error.
- The whole parser is a pure FSM over plain string operations: NO regular
  expressions anywhere (decided 2026-08-24), including the include-note-id
  extraction (it is a marker of its own, recognized by the FSM like any
  other).

## Marker grammar (summary)

### Scope entities

A documented entity is a **scope**: an opening `begin-*` marker, a header
marker with fields, the real C/C++ definition, and a closing `end-*` marker.

```
/*{{begin-c-function}}*/
/*{{c-function: <export name>
            | syntax: `<C signature>`
            | category: ...
            | header: ot4xb_c_exported.h
            | mangled-name: <plain/decorated export name>
            | desc: ...
            | param <name>: <type> - <text>
            | return: <type> - <text>
   }}*/
<the real C definition>
/*{{end-c-function}}*/
```

Scope kinds and their header keyword:

| begin / end            | header keyword   | entity                                  |
|------------------------|------------------|-----------------------------------------|
| `begin-c-function` / `end-c-function`     | `c-function:`   | `extern "C"` / cdecl export |
| `begin-cpp-function` / `end-cpp-function` | `cpp-function:` | C++-linkage export (OT4XB_API, no extern "C") |
| `begin-function` / `end-function`         | `function:`     | Xbase++ function (`_XPP_REG_FUN_`) |
| `begin-class` / `end-class`               | `class-name:`   | Xbase++ class (may be a GWST structure class) |

- The header marker is `/*{{<keyword>: <first line>` ... fields ... `}}*/`.
  It is ONE C comment (opens `/*{{`, closes `}}*/`).
- The `begin-*`/`end-*` markers are each their own self-closed comment
  `/*{{begin-x}}*/` on a line by themselves.

### Header value = the identity key (decided 2026-08-24)

The header keyword's value is NOT the signature: it is the entity's
**identity key** - the exact string every other place uses to reference it
(spec §4: `see-also`, `calls`, external documents, note includes):

| kind | value = identity | example |
|---|---|---|
| `function:` | the Xbase++ name (Xbase++ has no overloads) | `cPrintf` |
| `class-name:` | the class name | `FILETIME64` |
| `c-function:` | the export name (C has no overloads) | `bWriteLogLine` |
| `cpp-function:` | name + parameter type list = the overload id | `_conCallCon(LPSTR,LONG)` |

- The calling forms move into a **`syntax:` field** (accumulative; added to
  the known vocabulary): a markdown code span holding the full form. Inside
  the span a `|` separates **alternative calling forms of the SAME entity**:

  ```
  /*{{begin-function}}*/
  /*{{function: cPrintf
              | syntax: `cPrintf( cFormat, ... ) -> cText | cPrintf( NIL, cFormatWithEscapes, ... ) -> cText`
              | category: strings
              | desc: ...
     }}*/
  XPPRET XPPENTRY CPRINTF( XppParamList pl ) { ... }
  /*{{include-note-id: fp-np-parameter-inference}}*/
  /*{{end-function}}*/
  ```

  Alternative calling forms (ONE registration/export with several call
  patterns, like cPrintf with NIL first) are NOT C++ overloads. The dividing
  line: **distinct exports = distinct scopes; same export with several call
  patterns = one scope, one `syntax` with `|`**.
- The cpp-function overload id is `name(TYPE,TYPE)`: parameter types only,
  no return type (C++ forbids overloads differing only in return, so
  name+types is unique by construction), no parameter names, types spelled
  exactly as the prototype spells them (no typedef resolution). The name
  before the `(` **groups the family**: a reference WITH parentheses targets
  one concrete overload, WITHOUT parentheses the family (or the single
  function - same degenerate case). This makes the spec's deferred
  "grouping the overloads of a family" fall out for free.
- Comparison rules: blanks collapse (`f(LPSTR,LONG)` == `f( LPSTR , LONG )`);
  Xbase++ names (`function:`, `class-name:`) compare case-INsensitively (the
  language is), C/C++ names (`c-function:`, `cpp-function:`)
  case-SENSITIVELY.
- `mangled-name` is the **physical twin** of the id: the MSVC decoration
  encodes the whole signature, so it is unique per overload by construction
  and must map 1:1 to the id, anchoring the documentation to the real export
  table. In-file diagnostics for scandoc: two scopes with the same
  `mangled-name` = error; two scopes with the same identity = error.
  (Verifying against the DLL export table - missing exports, undocumented
  exports - is the later `check` leg, which needs the DLL.)
- An entity may be documented in **two or more places, complementary never
  competing**: the normal mechanism is shared notes plus loose
  `/*{{include-note-id: X}}*/` markers inside the scope; a block living
  elsewhere (or an external document) names its owner by this identity key.
  Proximity fragment markers inside scopes stay OUT of scandoc v1.
- **Migration**: the ~84 sources marked so far carry the OLD form - the full
  signature as the header value. The old form is mechanically detectable (a
  `(` in a `function:` value; a blank in a `c-function:`/`cpp-function:`
  value). The sources must be migrated to name + `syntax:`; until then the
  parser tolerates the old form and reports it as a warning.

### Fields

Inside a header, fields are `| <name>: <value>`. Every `|` outside inline
code spans / fences starts a new field, so one line may legally carry several
fields; writing one field per line with the `|` aligned at column 12 and
continuations indented 14 is the house *style*, not grammar (decided
2026-08-24: "un campo por linea lo veo bien esteticamente pero no lo veo
necesario como convencion estricta"). The parser is indentation-blind. A
continuation is any line that does not start with `|` (after trimming); its
text joins the current field's value with one blank. A field may repeat
(e.g. several `param`, `flag`, `note`, `see-also`).

Known field names (open set — unknown fields should be kept, not rejected):
`syntax`, `category`, `header`, `mangled-name`, `desc`, `param`, `return`,
`flag`, `note`, `see-also`, `example`, `todo`, `introduced`, `calls`,
`ivar`, `method`, `class-function`, `gwst-member`, `access` (rare), plus
class auxiliaries.

Unknown names get an IIS-style known/unknown treatment WITHOUT altering the
flat model (decided 2026-08-24): the parser carries a vocabulary of known
field names (matched by the name's first word, case-insensitive, so
`param nFlags` matches `param`) and sets a single extra flag
`Field.Unknown: true` on the rest. Flagged fields are kept, ordered and
transported exactly like known ones — no slots, no diagnostic; what to do
with them (generic rendering, a typo lint in the later `check` leg) is the
consumer's business.

Looking ahead to rendering (noted 2026-08-24): format rules will come in
three tiers — **known** (built into the generator: `param` → parameter
table, `flag` → bit list, `syntax` → the Syntax section split on the `|`
inside the span, ...), **re-known** (tags born unknown and promoted by
project configuration, given a format without touching parser or generator
code), and **the rest** rendered generically as `name: value`. The parser's
vocabulary / `Unknown` flag is informative (debug, `-tags`); the generator
keys on its own rule set by tag name, so no two vocabularies need to stay in
sync.

- `param <name>: <type> - <text>` — the name follows `param `, then `:`.
- `flag 0xNN: <text>` — a bit flag.
- `introduced: X.Y.Z[ - <what>]` — the ot4xb version a feature/entity appeared
  in. Removals are NOT a field: a removed function has no doc block, so its
  removal is a `note` (with the version) in the successor entity.
- `todo: <text>` — provisional marker; greppable, means "needs review".

### Field values are markdown (inline code and fences)

Decided 2026-08-24 with Pablo: field values are markdown-flavored text and
are stored RAW - the parser never unescapes and never strips delimiters;
rendering (and resolving escapes) is the generator's job.

- Inline code spans follow CommonMark: a run of N backticks opens a span,
  the next run of exactly N on the same line closes it; everything inside is
  literal (a `|` there is text, not a field separator) and the span is stored
  INCLUDING its backticks. A backtick with no closing partner on the same
  line is a plain literal character - no span opens. Spans do not cross
  lines (multi-line code is a fence).
- Backslash escapes work outside code spans: `\|`, `` \` `` and `\\` remove
  the structural meaning of the escaped character but are stored as written.
- Fenced code blocks may appear in any field (typically `example`): a field
  line ending in an opening fence - 3 or more backticks OR 3 or more tildes
  (`~~~`, standard CommonMark alternative), optionally followed by an info
  string (` ```prg `) - switches to verbatim copying until a line holding at
  least as many of the same fence character. The opening fence must be the
  last thing on its line. Fenced lines are code and may exceed 120 (up to
  ~140). The whole block is stored in the value INCLUDING the fence lines.
- `}}` and `*/` remain forbidden inside a marker (they close the marker /
  the C comment); no escape can protect them.

The classic `example` shape:

```
            | example: ```
   QOut( cPrintf( , "%s", cValue ) )
```
```

### Shared notes (reusable, include-by-id)

A note shared by several entities is defined once and pulled in by id.

**Definition** (usually near the relevant helper, or at end of file):

```
/*{{begin-note | note-id: <kebab-or-symbol-id>
             | title: <Human Readable Title>}}*/
/*{{note:
<free-text body; markdown-style "- " bullets allowed>
}}*/
/*{{end-note}}*/
```

CRITICAL C-syntax rule: `begin-note` and `end-note` are self-closed comment
markers, and the **body lives inside its OWN comment** `/*{{note: ... }}*/`.
The body must never be left as bare text between `}}*/` and `/*{{end-note}}*/`
(that would be outside any comment and break C compilation).

**Inclusion** is a **stand-alone marker**, placed loose inside a scope between
the entity's `begin` and its `end` (typically right after the header, before
the code):

```
/*{{include-note-id: <id>}}*/
```

It is its OWN self-closed comment, NOT a header field - do NOT nest `{{ }}`
inside a field. It counts as an include ONLY when it appears loose between a
`begin` and its `end`. `note-id` on the definition matches `include-note-id`
on the use. The composer inserts the referenced note's body at that spot.
Every `include-note-id` must resolve to a `begin-note` with that `note-id`;
a dangling include is an error. A defined note with no includes is allowed
(a warning at most).

Real examples in `fpCall.cpp`: `note-id: fp-np-parameter-inference` (the
Xbase++ value -> C type conversion table) and `note-id: _dwGetFpParam_` (how to
provide the `fp` parameter), each included by several functions.

## Authoring preferences (house style)

The writing conventions, in one place — both for writing new blocks and for
REVIEWING the docs already written (the ~84 marked sources predate several
of these decisions and must be checked against them):

1. Header value = the identity key, nothing else: `function: cPrintf`,
   `class-name: FILETIME64`, `c-function: bWriteLogLine`,
   `cpp-function: _conCallCon(LPSTR,LONG)`. Never a full signature there.
2. Calling forms in `| syntax:` as a markdown code span; alternative call
   patterns of the same entity separated by `|` INSIDE the span. Long forms:
   one `syntax:` field per form.
3. Note inclusion is the loose marker `/*{{include-note-id: X}}*/` between
   the header and the code — never `{{ }}` nested inside a header field.
4. One field per line, `|` aligned at column 12, continuations at 14,
   closing `}}*/` indented 3: preferred style (the parser does not care).
5. Code, operators and any literal `|` go between backticks; multi-line code
   in a ``` / `~~~` fence (typically `example`); type alternatives written
   with `/` (`Date/Character/FILETIME64 object`).
6. Lines <= 120 (fence lines <= 140), ASCII only, CRLF. The moment a
   trailing one-line marker does not fit, it becomes a block marker before
   the registration line.
7. References (`see-also`, `calls`) use identity keys: with parentheses = a
   concrete cpp overload, without = the function/family/class.
8. `introduced: X.Y.Z[ - <what>]` near the end of the header (before
   `see-also`); removals are a `note` (with the version) in the successor
   entity, never a field.
9. `todo:` marks provisional/skeleton blocks; greppable; the generator
   treats such a block as unfinished.
10. Scopes framed by the file's `// ----` separator lines; old informal
    comments superseded by markers are removed.
11. Signature conventions inside `syntax`: `[x]` optional, `@x` by
    reference, descriptive parameter names (the count must match the code).
12. Shared-note definitions live near the relevant helper or at the end of
    the file; the body ALWAYS inside its own `/*{{note: ... }}*/` comment.

Known review debt in the existing sources (detectable mechanically, see the
old-form warning and `-tags`): old-form header values everywhere (full
signatures as values); `fpCall.cpp` still includes notes through the retired
field form `| note: {{include-note-id: ...}}` (5 uses) instead of the loose
marker.

## The FSM

Walk lines; maintain a small state and a scope stack. Suggested states:

- **Idle** — normal source; look for `/*{{`. On `/*{{begin-<kind>}}*/` push a
  scope and go to **AwaitHeader**. On `/*{{begin-note ...` go to **NoteDef**.
  On `/*{{<keyword>:` with no matching open begin, it may be a stand-alone
  header (tolerate/emit per spec). Everything else stays Idle.
- **AwaitHeader** — expect the header marker `/*{{<keyword>: ...`. Its first
  line holds the identity key (name, or `name(TYPES)` for a cpp overload);
  then read fields until the closing `}}*/`.
- **InHeaderFields** — split each line on `|` outside code spans and escapes
  (several fields per line are legal; one per line is only style); a line not
  starting with `|` continues the current field (or the signature), joined
  with one blank. On a field line ending in an opening fence (``` or `~~~`,
  info string allowed) enter **Fence**. On the closing `}}*/` line, go to
  **InScopeBody**.
- **InScopeBody** — between the header and `end-<kind>`: source code plus any
  loose `/*{{include-note-id: X}}*/` markers. Each such marker records a note
  reference on the current entity (an include ONLY here, loose between begin
  and end - never as a header field). On `/*{{end-<kind>}}*/` pop scope.
- **Fence** — copy lines verbatim (the stored value keeps the fence lines
  themselves) until a closing line of at least as many of the same fence
  character, then back to InHeaderFields. A `}}*/` or EOF before the closing
  fence is an error.
- **NoteDef** — read `note-id` and `title` from the `begin-note` marker, then
  expect `/*{{note:`; copy body lines until `}}*/`; expect `/*{{end-note}}*/`;
  register the shared note; back to Idle.
- Scope close: `/*{{end-<kind>}}*/` pops the matching scope; mismatched or
  unbalanced begin/end is an error with file:line.

Track, per entity: kind, identity, fields (ordered, repeats kept), note
references, and source span (start/end line). Track per file: entities in
order, shared-note definitions, and diagnostics.

## Output model (Go, sketch)

```go
type Kind int // CFunction, CppFunction, Function, Class, Note

type Field struct { Name, Value string; Line int; Unknown bool } // continuations folded in; Value is raw markdown (spans/fences kept verbatim, delimiters included); Unknown = name not in the known vocabulary, field kept all the same

type Entity struct {
    Kind      Kind
    Ident     string      // header value: the identity key (name, or name(TYPES) for a cpp overload)
    Fields    []Field     // syntax/desc/param/...; the calling forms live in the syntax field(s)
    NoteRefs  []string    // include-note-id values
    StartLine, EndLine int
}

type Note struct { ID, Title, Body string; StartLine, EndLine int }

type File struct {
    Path     string
    Entities []Entity
    Notes    []Note
    Diags    []Diag      // warnings/errors with Line + message
}
```

Provide a resolver step: expand `include-note-id` against `Notes`, and report
unresolved includes.

## Validation / diagnostics (report with file:line)

- begin/end balance per scope kind; mismatched nesting.
- Malformed header (`/*{{` with nothing after, missing `}}*/`).
- A fence (``` / `~~~`) not closed before the header's `}}*/` or EOF.
- `include-note-id` with no matching `note-id`.
- Shared-note body not enclosed in its `/*{{note: ... }}*/` comment.
- Duplicate identity, or duplicate `mangled-name`, between scopes of the
  same file (the id <-> mangled-name bijection, in-file half).
- Old-form header value (a full signature where the identity key belongs) —
  a warning until the sources are migrated.
- Comment-depth never negative; ends at 0.
- Lint (warnings): marker/field line > 120 (fence lines > 140); leftover
  `todo:` fields; non-ASCII bytes; CR/LF imbalance.

## Deliverables

- Package `scandoc` in the ot4xb-tool module: `scanner.go` (FSM), `model.go`
  (types), `resolve.go` (note expansion), `diag.go`.
- A `Scan(path) (*File, error)` entry point and a `ScanDir` helper.
- Table-driven unit tests using `fpCall.cpp` / `DrTool.cpp` / `FileTime.cpp`
  fixtures (or trimmed copies) covering: each scope kind, repeated fields,
  several fields on one line, `|` and backticks inside code spans, backslash
  escapes, backtick and tilde fences, the identity forms (name vs
  `name(TYPES)`, `syntax` with alternative call forms, duplicate id /
  mangled-name), a shared note + its includes, the versioning field, and the
  bad-body/dangling-include error cases.
- Keep it read-only and encoding-safe (1252 in, positions preserved).

## Wiring into ot4xb-tool

`scandoc` is the front end that feeds later doc generation (README / AGENTS /
SKILL-style outputs, or a changelog from `introduced:` / removal notes). For
now, expose it as a library plus a `ot4xb-tool scandoc <path>` subcommand that
prints the parsed model and diagnostics, so the markers already written in the
ot4xb sources can be validated end to end.

CLI convention (decided 2026-08-24): every parameter has a name -
positionals end up being a pain (there are none). One syntax only:
`-name value` (no '=' / ':' forms). `-src path` names the input;
hand-parsed like -tool/-bs, which is also what lets -tags take an optional
value.

Debug option `-tags [file]` (decided 2026-08-24) - ONE flag with an
optional value: alone it prints the compact inventory, with a file it
writes the full detail there, because extracting the tags is normally for
analyzing them separately. The model already carries name + line + the
Unknown flag, so this is a CLI-side loop, no model change.

Console summary (`-tags` alone), aggregated over the scanned file or
directory:

```
known tags:   category(84) desc(85) param(212) return(85) note(31) ...
unknown tags: patata(1, DrTool.cpp:123) ...
```

Known names with a count; unknown names with a count AND the first
occurrence as file:line — that is what hunting a typo (or spotting a tag
worth promoting into the vocabulary) needs.

The detail file (`-tags file`, or `-tags:file`) carries EVERY occurrence,
one per line, tab-separated, unknown first, ASCII + CRLF - ready for grep /
sort / UltraEdit:

```
unknown	nDrive	C:\...\source\DrTool.cpp:1046
known	category	C:\...\source\Bitwise.cpp:12
```
