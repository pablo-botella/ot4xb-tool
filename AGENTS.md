# ot4xb-tool

`ot4xb-tool` is the tool set of the [ot4xb](https://github.com/pablo-botella)
Xbase++/C++ library in a single Go binary. Every tool is a subcommand backed by
a package under `modules/`, so it can also be used as a library:

```
ot4xb-tool [-q] <command> [options]
```

`-q` (before the command) prints nothing but errors. Exit code 0 on success,
1 on any error, 2 on a usage error; errors go to stderr.

```sh
go install github.com/pablo-botella/ot4xb-tool@latest    # the binary
```

```sh
go get github.com/pablo-botella/ot4xb-tool    # library
```

## Commands

| command | what it does |
|---|---|
| `-bs entry` | run one entry of the project tool file (`<project>.ot4xb-tool`): a sequence of build steps |
| `vbuild` | version scripts: increment the version and generate the files that carry it |
| `xbmac2h` | from a `.xbmac` registration list, the export/function-list headers and the two `.def` files |
| `def2lib20` | an x86 COFF import library (`.lib`, long format, ALINK compatible) from a `.def` |
| `cbk2obj` | an Xbase++ callback script (`.cbk`) compiled into a linkable x86 COFF object (`.obj`) |
| `doc scan` | scan sources for `/*{{ }}*/` documentation markers (Draft 4): list the topics and the issues |
| `doc split` | split an authoring source into its code projection (no doc blocks) and its doc projection |
| `doc check` | cross-check the documented surface against the `.xbmac` registration list |
| `doc compile` | compile documented sources into the intermediate SQLite database (any number of passes) |
| `doc resolve` | the a-posteriori step over that database: broken references, include cycles, slug clashes; then the pages with their logical location, materialized for the generators |
| `doc gen` | the reference Markdown from that database: one file per topic or topic group (slugs) plus an index |
| `doc site` | a static HTML site from that database, as a `.site-def` describes it (templates, assets); not a build step |

The build-side commands (`-bs`, `vbuild`, `xbmac2h`, `def2lib20`, `cbk2obj`)
replace the legacy Harbour/xppcbk tools of the ot4xb build, byte-compatible
where the old outputs are consumed by other tools. The documentation subcommands
(`doc scan`, `doc split`, `doc check`, `doc compile`, `doc resolve`) form a pipeline: the
sources are the only truth, the database is an intermediate build artifact
like an `.obj`, and every product is generated from it afterwards.

## Conventions shared by every command

- Inputs are read as bytes (ANSI / Windows-1252, no transcoding) and lines
  may end in CR, LF or CRLF in any mix (every reader goes through the same
  line reader). Generated outputs are CRLF unless a command says otherwise.
- No command modifies a source it reads; generated files are written whole.
- Nothing is done in parallel: one process, one thread. The tools are fast
  enough and their outputs must be reproducible.

## `-bs` — build steps of a project tool file

A project keeps its build automation in one JSON file, `<project>.ot4xb-tool`,
next to its sources. The file holds a `versioninfo` pointer (the vbuild script)
and any number of *entries* — `pre.release`, `post.debug`, … — each a sequence
of *steps*. Visual Studio (or any build system) runs an entry as a pre- or
post-build event:

```
ot4xb-tool [-q] [-tool file.ot4xb-tool] -bs <entry>
```

`-tool` names the tool file; without it the single `*.ot4xb-tool` of the
current folder is used (more than one, or none, is an error). Paths inside the
file are relative to the folder of the tool file, never to the current
directory.

```json
{
  "versioninfo": "./source/ot4xb.VersionInfo",
  "pre.release": {
    "folders": { "src": "./source" },
    "steps": [ "vbuild", "codegen" ],
    "codegen": { "xbmac2h": "<src>/ot4xb.xbmac" }
  }
}
```

### Steps

A step whose name is one of the tool modules runs that tool with the config
of the same name; any other name is a composite step executed by its config
keys, in this fixed order:

| step | runs |
|---|---|
| `vbuild` | the version script named by `versioninfo` (after it, the `<v.*>` macros see the new version) |
| `xbmac2h` | `xbmac2h` on the given `.xbmac` |
| `def2lib20` | `def2lib20` on the given `.def` |
| `artefacts` | the artefacts module (release file layout); a content item with `"clean_doc_comments": true` packs the clean projection of every source carrying `/*{{ }}*/` blocks (what `srcsplit -code` would write), so a release zip ships sources without the documentation; a content item with `"tree": "<folder>"` packs that folder's whole subtree under `out`, every file under its relative path, where `in` lands files flat |
| `srcsplit` | `srcsplit` over `in` (file, glob or directory): the code projection to `code` and/or the doc projection to `doc` (`*` = the source base name); `force` overwrites, `bak` keeps a `.bak` |
| `docs` | the documentation: with `doc_tool` (a `.doc-tool` file, see that section) the sources, database and kinds come from the file; otherwise `src` (a pattern or an ordered list of patterns - the list order is the document order, a file matched twice counts once, `*.*` is every file of a folder, a pattern matching nothing warns) and `db` are required and `root` (default: the tool dir) is the project root. The sources are compiled, resolved and rendered into `out`; `clean` (default true) removes the database and the output folder first; broken references are warnings, `strict` makes them an error |
| *(composite)* | `cleanup.before` (delete files), then `xbmac2h`, then `copy` |

The documentation is built when and how the JSON says. An entry of its own,
run by hand or from a build event:

```json
"docs": {
  "folders": { "src": "./source", "out": "./out" },
  "steps": [ "docs" ],
  "docs": { "doc_tool": "./ot4xb.doc-tool", "out": "<out>/md", "clean": true }
}
```

### Macros in values

Every string value may use `<name>` macros, resolved case-insensitively
against the entry's `folders` and `vars`, plus the version components of the
vbuild script: `<v.maj>`, `<v.min>`, `<v.hbuild>`, `<v.lbuild>`, `<v.build>`,
optionally with a width (`<v.build:5>`, `<v.maj:03>`). An unknown name is an
error — loud beats silent. Paths are normalised the agreed way: `/` and `\`
both accepted, doubled separators at macro junctions collapsed, `\` handed to
the disk.

## vbuild — version scripts

`vbuild` keeps the version number of a project in one text file (the *version
script*, by convention `<project>.VersionInfo`) and generates from it, at every
build, the files that must carry that number: the `.rc` VERSIONINFO block, a
`version.h`/`.ch`, batch files, a LICENSE with the current year… One script per
project, run as a pre-build step:

```
ot4xb-tool [-q] vbuild [-inc major|minor|hbuild|lbuild|build] [-eolrn|-eolr|-eoln|-eols] <script>
```

It reads the script, optionally increments the version (only when `-inc` says
so), rewrites the first line of the script when the version changed, expands the
macros of the body and splits the body into the output files named by the
`{$<File:>$[…]$}` markers.

### The script

```
[Product]  Version:{major,minor,hbuild,lbuild}
free text, macros and file markers …
```

#### Line 1: the version header

```
[ot4xb.dll]  Version:{1,7,14,0}
```

- `[Product]` — any text (blanks allowed) between the first `[` and the last `]`
  before `Version:`. The product name is written back verbatim and is not used
  anywhere else.
- `Version:{a,b,c,d}` — four decimal components `major,minor,hbuild,lbuild`, each
  in `0..255`, separated by commas, **no blanks** inside the braces and nothing
  after `}`. The keyword `Version:` is case-insensitive; blanks are allowed before
  `[` and between `]` and `Version:`.
- `[Product:noinc]` — the suffix `:noinc` (case-insensitive) freezes the version:
  the script is never incremented whatever `-inc` says.
- When vbuild increments, the line is rewritten normalised: `[Product]  Version:{a,b,c,d}`
  (two blanks, capital V, no zero padding); the rest of the file is not touched.
  When nothing changes the file is left alone.

A first line that is not a valid header is an error (nothing is generated).

#### The body: macros

The body is free text. Every occurrence of these macros is replaced (case-insensitive
in the macro name, exact spelling otherwise; values come from the version **after**
the increment, the clock and the folder of the script; all occurrences of a macro
get the same value within one run):

| macro | value | example |
|---|---|---|
| `{$<FILEVERSION(,,,)>$}` | `a,b,c,d` | `1,7,14,0` |
| `{$<FILEVERSION(...)>$}` | `a.b.c.d` | `1.7.14.0` |
| `{$<FILEVERSION(000,000,000,000)>$}` | zero-padded to 3 digits, commas | `001,007,014,000` |
| `{$<FILEVERSION(000.000.000.000)>$}` | zero-padded, dots | `001.007.014.000` |
| `{$<FILEVERSION(_._._._)>$}` | zero-padded, underscores | `001_007_014_000` |
| `{$<LOCALTIME(YYYY)>$}` | local year | `2026` |
| `{$<LOCALTIME(YYYYMMDD)>$}` | local date | `20260819` |
| `{$<LOCALTIME(14)>$}` | local `YYYYMMDDhhmmss` | `20260819162537` |
| `{$<LOCALTIME(19)>$}` | local `YYYY-MM-DD hh:mm:ss` | `2026-08-19 16:25:37` |
| `{$<LOCALTIME(19+z)>$}` | local, with UTC offset | `2026-08-19 16:25:37 +0200` |
| `{$<SYSTEMTIME(14)>$}` | UTC `YYYYMMDDhhmmss` | `20260819142537` |
| `{$<SYSTEMTIME(19)>$}` | UTC `YYYY-MM-DD hh:mm:ss` | `2026-08-19 14:25:37` |
| `{$<UUID>$}` | a fresh UUID, 32 lowercase hex digits, no dashes (one per run) | `7e95207205154948ae833f3086c4e5e0` |
| `{$<fld>$}` | name of the folder that contains the script | `source` |

##### `FILEVERSION` with a mask

```
{$<FILEVERSION(:mask:)>$}
```

The mask is free text in which the version components, written between angle
brackets, are replaced by their values; everything else is copied as is (a
`<word>` that is not a component stays literal). The tag closes at the exact
sequence `:)>$}`.

| component | value |
|---|---|
| `<maj>` | major (`a`) |
| `<min>` | minor (`b`) |
| `<hbuild>` | high byte of the build (`c`) |
| `<lbuild>` | low byte of the build (`d`) |
| `<build>` | the build as a 16-bit number, `c*256 + d` (0..65535) |

Width and padding go in parentheses, like a C format: `<maj>` renders as `%i`
(no padding), `<maj(03)>` as `%03i` (zero-padded to 3 digits), `<maj(3)>` as
`%3i` (space-padded to 3). Same for every component.

Examples with `1,7,14,0`:

| mask | result |
|---|---|
| `<maj>.<min>.<build>` | `1.7.3584` |
| `version: <maj>.<min> build: <build(05)>` | `version: 1.7 build: 03584` |
| `<maj(02)>_<min(02)>_<hbuild(02)>_<lbuild(02)>` | `01_07_14_00` |
| `v<maj>.<min> (<build>)` | `v1.7 (3584)` |

Macros are expanded before the body is split, so they also work inside the file
markers (`{$<File:>$[{$<fld>$}_{$<FILEVERSION(_._._._)>$}.txt]$}`).

#### The body: file markers

```
{$<File:>$[path]$}
```

A line holding only a marker (blanks around it allowed, nothing else) starts a new
output file; every following line, up to the next marker or the end of the script,
is written to it. Rules:

- `path` is taken literally (leading/trailing blanks trimmed); a relative path is
  resolved against the **folder of the script**, not the current directory; an
  absolute path is used as is. The target folder must exist.
- The file is created or truncated; naming the same file twice keeps the last
  block.
- Lines between the header and the first marker belong to no file: that area is
  free for notes.
- A marker with no lines after it creates an empty file.
- The marker keyword (`File:`) is case-insensitive.

Example — the script of ot4xb:

```
[ot4xb.dll]  Version:{1,7,14,0}

{$<File:>$[LICENSE.TXT]$}

Copyright (c) 2004 - {$<LOCALTIME(YYYY)>$} Pablo Botella Navarro
…

{$<File:>$[ot4xb_version.h]$}
#define OT4XB_VERSION_STRING   "{$<FILEVERSION(000.000.000.000)>$}"
{$<File:>$[ot4xb.rc]$}
#include <windows.h>

1 VERSIONINFO
FILEVERSION {$<FILEVERSION(,,,)>$}
PRODUCTVERSION {$<FILEVERSION(,,,)>$}
…
```

#### Encoding and line endings

The script and the generated files are plain bytes (ANSI / Windows-1252), no
transcoding. Input lines may end in CR, LF or CRLF, in any mix; lines are never
wrapped. The line terminator of the **output** is chosen on the command line
(default CRLF, see below).

### The command

```
ot4xb-tool [-q] vbuild [-inc <component>] [-eolrn|-eolr|-eoln|-eols] <script>
```

| option | meaning |
|---|---|
| `-inc major` | `a+1`, and `b,c,d` reset to 0 |
| `-inc minor` | `b+1`, `c,d` reset to 0 |
| `-inc hbuild` | `c+1`, `d` reset to 0 |
| `-inc lbuild` | `d+1`; carries into `c` (and `b`, `a`) when a component passes 255 |
| `-inc build` | same as `lbuild` |
| *(no `-inc`)* | no increment: the files are regenerated with the current version |
| `-eolrn` | output lines end in CRLF (**default**) |
| `-eolr` | output lines end in CR |
| `-eoln` | output lines end in LF |
| `-eols` | *save*: every output line keeps the terminator it had in the script (a last line without terminator stays without it) |
| `-q` | (global option of ot4xb-tool, before the command) print nothing but errors |

Behaviour:

- The version is incremented only with `-inc`, and never when the header carries
  `:noinc`. An increment that would overflow `major` past 255 is an error and
  nothing is written.
- When the version changed, the header line of the script is rewritten in place
  and the new header is printed (unless `-q`).
- All the output files are then generated from the expanded body, whether or not
  the version changed.
- Exit code 0 on success, 1 on any error (invalid header, overflow, unknown
  option, unreadable script, output file that cannot be created); errors go to
  stderr.

## `xbmac2h` — code generation from the registration list

The `.xbmac` file is the registration list of an ot4xb-style DLL: one line per
function the DLL offers to Xbase++. `xbmac2h` generates from it the four files
the build needs, next to the list:

```
ot4xb-tool [-q] xbmac2h [-lib NAME] file.xbmac
```

| output | content |
|---|---|
| `<base>_xbexports.hpp` | `XPPRET XPPENTRY <sym>(XppParamList );` prototypes inside `extern "C"` |
| `<base>_xbfunclist.hpp` | `{"NAME",<sym>}` initialiser rows (the function table) |
| `<base>Cpp.def` | `LIBRARY`/`EXPORTS` with `NAME =  <sym>  PRIVATE` — the MSVC side |
| `<base>.def` | `LIBRARY`/`EXPORTS` with `NAME =  _<sym>` — the Xbase++ side, input of `def2lib20` |

`-lib NAME` sets the `LIBRARY` name of the `.def` files (default: the file name
without extension).

### The list

```
_XPP_REG_FUN_( name )      // plain Xbase++ function, C symbol NAME
_XPP_REG_WMAC( name )      // macro-style wrapper, C symbol wapimc_NAME
_XPP_REG_WST_( name )      // structure wrapper, C symbol wapist_NAME
_XPP_REG_WAPI( name )      // Win32 API wrapper, C symbol wapi_NAME
_CDECL_EXPORT_( name )     // a plain C function the DLL exports under its own name
```

Blanks are free; `//` starts a comment to the end of the line. The names of
the four registration commands are upper-cased (as the legacy tool did); a
`_CDECL_EXPORT_` name keeps its case, it is a C symbol. A `_CDECL_EXPORT_`
function is not an Xbase++ function: the two `.hpp` files get only a comment,
`<base>Cpp.def` gets nothing (a `PRIVATE` entry would drop it from the MSVC
import library) and `<base>.def` gets `name =  _name` so that Xbase++ clients
reach it through the `def2lib20` import library as well.

The formats are byte-compatible with the Harbour `xbmac2h` (CRLF, same
blanks), except that the comments of the `.xbmac` are discarded instead of
copied. Blank lines are kept; unrecognised lines are reported and marked in the
outputs exactly like the legacy tool did.

## `def2lib20` — import libraries for ALINK

`def2lib20` builds an x86 COFF import library (`.lib`, long format) from a
module definition file, so that Xbase++ programs can link against the DLL
with Alaska's ALINK:

```
ot4xb-tool [-q] def2lib20 [-monkey] [-o out.lib] [-dll name.dll] [-prefix _] [-ts seconds] file.def
```

| option | meaning |
|---|---|
| `-o out.lib` | output library (default: the `.def` name with `.lib`) |
| `-dll name.dll` | DLL file name recorded in the library (default: the `LIBRARY` name + `.dll`) |
| `-prefix _` | prefix added to the linker symbols (e.g. `_` for cdecl C exports) |
| `-monkey` | imitate Alaska's `aimplib`: `<dll>_IMPORT_DESCRIPTOR` / `NULL_IMPORT_DESCRIPTOR` / `<dll>_NULL_THUNK_DATA` names and hint `0xFFFF`, sharing the terminator with the Xbase++ runtime libraries |
| `-ts seconds` | COFF timestamp, seconds since the Unix epoch, for reproducible outputs |

### The `.def`

Read line by line (CR, LF or CRLF), as bytes. `;` starts a comment. The
`LIBRARY` statement and the `EXPORTS` section are understood:

```
entryname[=internalname] [@ordinal [NONAME]] [DATA|CONSTANT] [PRIVATE]
```

Other statements (`NAME`, `DESCRIPTION`, `STACKSIZE`, `HEAPSIZE`, `VERSION`,
`SECTIONS`, …) are recognised and skipped. A line the parser does not
understand is an error with its line number.

### Two import libraries for one DLL

A DLL exported to Xbase++ usually needs two libraries: the `def2lib20` one for
ALINK and the MSVC one for C/C++ clients. When both are linked into the same
program the import descriptors must not collide: without `-monkey` the second
library works whatever the link order; with `-monkey` (Alaska-shared
descriptor names) the order matters. Choose per project.

## `cbk2obj` — callback objects

An Xbase++ program that hands a callback to a Win32 API needs a small stub
per callback: native code with the right calling convention that jumps into
the Xbase++ side. `cbk2obj` compiles a callback script (`.cbk`, the format of
the legacy `xppcbk` tool) straight into a linkable x86 COFF object:

```
ot4xb-tool [-q] cbk2obj [-asm] [-o out.obj] [-ts seconds] file.cbk
```

| option | meaning |
|---|---|
| `-o out.obj` | output object (default: the `.cbk` name with `.obj`) |
| `-asm` | also write the equivalent FASM source (`.asm`) next to the object, for inspection |
| `-ts seconds` | COFF timestamp, seconds since the Unix epoch, for reproducible outputs |

The script is read line by line (CR, LF or CRLF), as bytes. Parsing is a
hand-written scan; errors do not stop it: every faulty line yields a
diagnostic with its line number and the legacy message, and parsing goes on,
as `xppcbk` did. A `XPPCBK VERSION v` line above the version this tool accepts
is an error.

The generated stubs call their runtime helpers through the ot4xb import
library (`ot4xb.lib`), never by runtime injection; the object links like any
other. See the ot4xb sources for the scripts in use.

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
- **Code from the source**: `/*{{begin-code: xbase}}*/` ... `/*{{end-code}}*/`
  inside a composed topic make the source lines between them a code block of
  that topic, at that position, in the language named after the colon
  (optional). Those lines are code, not documentation: `split` keeps them in
  the clean source, and in the doc projection too, between the two markers.
  No marker may appear inside, the pair does not nest, a second `end-code`
  is an error, and a compact topic cannot hold one.
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
  `function` and a `c-function` of the same name are two topics). An `ilink`
  is a reference: `resolve` finds the page and every output writes it its own
  way - `.md` in a Markdown manual, `.html`, no extension with clean URLs. A
  Markdown link `[text](url)` is a link: it comes out exactly as written, in
  every output, and nothing rewrites it - so it is for URLs, never for a page
  of this documentation.
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

## `srcsplit` — the two projections of an authoring source

The sources are **authored** with their `/*{{ }}*/` documentation in place —
the Xbase++ headers (`.ch`) included — and what ships is a projection of
them: `srcsplit` produces the *clean* one (the source with every doc block
removed: the `.ch` a release installs, the sources a release zips — the
documented ones are always in the repository) and the *doc* one (only the
doc blocks, verbatim, in order):

```
ot4xb-tool [-q] doc split -src <file|glob|dir> [-code <dst>] [-doc <dst>] [-bak | -force] [-check]
```

| option | meaning |
|---|---|
| `-src` | one source, a glob (`ch/*.ch`), or a directory: its `.ch .prg .chsrc .c .cpp .h .hpp` |
| `-code <dst>` | write the clean projection to `dst` |
| `-doc <dst>` | write the doc projection to `dst` |
| `-bak` | an existing, different destination is copied to `<dst>.bak` before being overwritten |
| `-force` | an existing, different destination is overwritten, no copy |
| `-check` | compare against the existing destinations and report drift, write nothing (the CI leg): exit 1 when any would change |

Give one or both: **a projection whose destination is not named is not
produced** — `-code` alone just yields clean sources, nothing else is written.
Extensions are not implied: you name every destination. A destination may
carry a `*`, replaced by the source's base name (`-src ch/*.ch -code
out/include/*.ch` turns `ch/ot4xb.ch` into `out/include/ot4xb.ch`), or
`*.*`, replaced by the full file name — the way to send sources of mixed
extensions into one folder (`-src source -code out/clean/*.*`); a
destination without `*` is a single file, which is only allowed when `-src`
matches a single source. Missing destination folders are created.

### Overwriting

A destination that already holds exactly the bytes about to be written is
left alone (idempotent; not an overwrite). One that exists **with different
content** is never silently replaced: without `-bak` or `-force` the run
fails for that file and nothing is written; `-bak` keeps the previous bytes in
`<dst>.bak` (replacing an older `.bak`) and writes; `-force` just writes.

### The split, line endings and encoding

A doc block occupies whole lines, from its `/*{{` line to its `}}*/` line;
ordinary `/* */` comments are kept in the clean projection. The source is
read line by line and any line ending is accepted; the outputs are **always
written CRLF** (a source that was not CRLF-code gets a warning, `N line(s)
not CRLF-terminated; outputs normalized to CRLF`), and a clean CRLF source
round-trips byte for byte.

Bytes pass through untouched (Windows-1252) unless the source declares
`/*{{encoding: utf-8}}*/`: then both projections are transcoded to
Windows-1252, and a character with no 1252 mapping is an error. The
directive itself is doc, so it never reaches the clean projection, and it is
not copied into the doc one.

## The `.doc-tool` file - kinds, books and indexes

The documentation of a project is configured in one JSON file, `<project>.doc-tool`,
next to the sources it describes. It declares what the sources may write
(the **kinds** of topic), where the pages are filed (the **books** and their
**indexes**), the general index, and what the `docs` build step compiles.
Every doc command takes it with `-doctool <file>`; the `docs` step with
`"doc_tool": "./x.doc-tool"`. Without one, the built-in configuration applies:
the nine kinds of Draft 4 and four books (c, cpp, xbase, other), each with an
index by category and an alphabetic one.

```json
{
  "books": [
    { "name": "xbase", "title": "ot4xb Xbase++ Manual", "prefix": "",
      "indexes": [ { "slug": "index-xbase", "title": "ot4xb Xbase++ Manual", "sections": [
        { "title": "Functions by name", "include": [ { "kind": "function", "category": "*" } ], "by": "name" },
        { "title": "Classes by name",   "include": [ { "kind": "class",    "category": "*" } ], "by": "name" },
        { "title": "By category",       "include": [ { "kind": "*",        "category": "*" } ], "by": "category" },
        { "title": "Alphabetic",        "include": [ { "kind": "*",        "category": "*" } ], "by": "name", "slug": "index-xbase-alpha" }
      ] } ] },
    { "name": "cpp", "title": "ot4xb C/C++ Manual", "prefix": "cpp-", "includes": [ "c" ], "indexes": [ ... ] },
    { "name": "c",   "title": "C API", "prefix": "c-", "indexes": [ ... ] }
  ],
  "index": { "slug": "index", "title": "ot4xb Reference", "sections": [
    { "title": "Manuals",   "books": [ "xbase", "cpp" ] },
    { "title": "Reference", "books": [ "c" ] } ] },
  "kinds": [
    { "name": "function",   "book": "xbase", "case_sensitive": false, "case_index": "upper", "tag": "function" },
    { "name": "class",      "book": "xbase", "case_sensitive": false, "case_index": "upper", "tag": "class", "aliases": [ "class-name" ] },
    { "name": "c-function", "book": "c",     "case_sensitive": true,  "tag": "function", "multi_definition": true },
    { "name": "topic",      "book": null,    "book_default": "xbase", "case_sensitive": false, "case_index": "lower", "tag": "topic" },
    { "name": "note-id",    "book": null,    "case_sensitive": false, "case_index": "lower", "tag": "note", "indexed": false }
  ],
  "folders": { "src": "./source", "out": "./out" },
  "src": [ "<src>/ot4xb.cpp", "<src>/*.cpp", "<src>/*.h", "<src>/ch/ot4xb.ch", "<src>/ch/*.*" ],
  "db": "<out>/ot4xb.db"
}
```

### Kinds

A kind is a header label (`/*{{c-function: name ...}}*/`, `begin-c-function`)
and the rules of its identity:

| property | meaning |
|---|---|
| `name` | the label; letters, digits, `-` and `_` |
| `book` | the book every topic of the kind belongs to; `null` when the topic says it itself with a `book:` field (a comma list: a topic may be in several books) |
| `book_default` | with `book: null`, the book of a topic that names none; without it, such a topic is an error (`resolve/book-required`) unless the kind is not indexed |
| `case_sensitive` | the identity is compared as written (C, C++); otherwise `case_index` says how it is keyed |
| `case_index` | `upper` (Xbase++ names) or `lower` (topics, notes) |
| `tag` | shown after the link in a list that mixes kinds (`(class)`, `(internal)`) - the kind with most entries in the list carries none |
| `aliases` | other header labels for the kind (`class-name` for `class`) |
| `indexed` | `false`: never listed (note-ids only live inside the pages that include them) |
| `multi_definition`, `prototype` | informational: the symbol may be documented in several places; the identity carries the parameter types |

### Books and indexes

A book is a set of pages: the topics of the kinds that name it, the topics
that name it in their `book:` field, and (`includes`) everything of the
books it includes - the C/C++ manual includes the C API. `prefix` names its
category page files (`cpp-c-api-tlist.md`; the Xbase++ book uses none:
`bitwise.md`; a name already taken by a page gets `-category` appended and
the log says so).

Each book has any number of `indexes`, each a page (`slug`, `title`) made of
`sections`. A section takes the pages of the book (or of another one, with
`book`) that pass its `include` filters and none of its `exclude` filters -
`{ "kind": <kind or *>, "category": <comma list of masks, * for any> }` - and
lists them `by` `name` (alphabetical) or by `category` (one line per
category, linking to the category page of the book, and the pages without
category at the end). A section with a `slug` is a page of its own and the
index only links to it: the place for the long alphabetic lists. A group
page (overloads, a `_tg_` set) is filed under the kind of its first topic.

The general `index` is made of sections that list books, each book with its
indexes and the sections that are pages of their own.

Every entry of a list is `- [name](file) (tag) - short (kw: a, b)`: the
`short` field of the topic header (`short_:` keeps the label out of the page;
without one, the first sentence of `desc`, cut at 160 characters) and its `kw`
keywords (`_kw_:` keeps them out of the page altogether), so a text search
over the index files finds a function by what it does, not only by its name.

### Sources

`folders`, `src` and `db` are what the `docs` build step compiles when it
points at the file: `<name>` macros resolve against `folders`, paths are
relative to the file, and `src` is the ordered list of patterns (see the
`docs` step). The step's own `src`, `db` and `root` keys, when given, win.

### What the project declares, what the machine does

The file splits in two along one line, and each half has somewhere else to be.

| members | whose they are | where they also live |
|---|---|---|
| `books`, `index`, `kinds` | the project: what the documentation **is** | written into the database by `compile`, in its `cfg` table |
| `folders`, `src`, `db` | the machine that collects: where the files **are** | the `.user` companion below |

The first half travels inside the database, so whoever generates from it needs
no `.doc-tool` of their own: a repository holding only the `.db` runs
`resolve`, `gendoc` and `gensite` as they are, and a repository that keeps a
`.doc-tool` for its local paths leaves `books`, `index` and `kinds` out of it -
every block the file does not declare comes from the database. What the file
declares wins over what the database offers; with neither, the built-in
configuration applies. The second half never travels: a `db` inside the
database would point at itself, and `src` and `folders` name files the
consumer does not have.

### The `.user` companion

Next to `<project>.doc-tool`, an optional `<project>.doc-tool.user` holds the
same members and replaces the ones it declares - whole, not merged: a
`folders` in the `.user` **is** the folder map, not an addition to it. It is
personal and never tracked (add it to `.gitignore`), and both files live in
the same folder, so a relative path means the same thing in either.

It exists for the half above that belongs to the machine. A `.doc-tool` that
declares `"db": "../site/ot4xb.db"` is carrying one developer's disk layout
into a versioned file; moved to the `.user`, the repository publishes only
what the project *is*, and where the database lands is a fact of each machine:

```json
{ "folders": { "out": "../../ot4xb-site/doc" } }
```

The two halves together are what makes a repository self-sufficient: the
sources and the configuration of the documentation on one side, the generated
database and the site on the other, and nothing in either that only means
something on the machine that ran the build.

## `gensite` - the static HTML site

Outside the build, from the same database `gendoc` reads:

```
ot4xb-tool [-q] doc site -site <file.site-def> [-doctool <file>] [-db <file.db>] [-out <dir>] [-title <text>] [-templates <dir>] [-assets <dir>] [-gencss]
ot4xb-tool [-q] doc site [-doctool <file>] -db <file.db> -out <dir> [-title <text>] [-templates <dir>] [-assets <dir>] [-gencss]
ot4xb-tool doc site -export-templates <dir>
```

Every page of the documentation - topics, groups, the indexes and the
category pages - is rendered from its tree (the documentation subset: see
the `.doc-tool` section) into minimal HTML - standard tags, no classes, one
tag per line, links to `x.md` pointing to `x.html` - and poured into a
template. The text is escaped here: identifiers, placeholders and paths
arrive as written and leave visible. goldmark runs on the `{{begin-md}}`
blocks alone, never on a page. A place outside the subset is an error with
its file and line, and nothing is written.
Only the pages are written: no script, no search index, and a style sheet
only when asked for. Everything is relative: the site works opened from disk
as well as served.

### The `.site-def` file

One database can feed any number of sites, so what belongs to a site lives
in its own JSON file, `<name>.site-def`, normally in the folder of the site
(a repository of its own, the one a static host deploys):

```json
{
  "doc_tool": "../ot4xb.doc-tool",
  "db": "../out/ot4xb.db",
  "templates": "./templates",
  "template_page": "doc-page.html",
  "template_index": "doc-index.html",
  "assets": "./assets",
  "out": "./out",
  "title": "ot4xb Reference",
  "gencss": false,
  "clean_urls": false,
  "keywords": ["ot4xb", "xbase"],
  "sitemap": { "base": "https://www.xbwin.com/ot4xb/doc/" }
}
```

The keywords of a page are the names of its topics (an overload without
its parameter list), then its `kw` field, then the site's `keywords`,
without repeats: they go to the `keywords` meta and to `.Keywords`.

`search` (`-search <file>` on the command line) writes a JSON file with
every page but the indexes - `file`, `title`, `kind`, `books`,
`categories`, `keywords`, `short` and `text`, the page body as plain text -
for a search engine that runs in the browser: a static `search.html` of
the site's assets loads it and scores the query; the other pages carry no
script at all. Without the key nothing is written.

Paths are relative to the file. `doc_tool` and `db` locate the
documentation (the flags of the same name override them); `templates` is
the folder whose files replace the built-in templates, and `template_page`
and `template_index` name the files to take from it - without them the names
are `page.html` and `index.html`, which lets a single folder hold the
templates of one site only; a name given here and missing from the folder is
an error, never a silent fall back to the built-in one; `assets` is a folder
copied into `out` as it is, files and subfolders - a style sheet, a favicon,
`robots.txt`, `_redirects` (the `static` folder of other generators);
`out` is where the site goes (required); `title` is the header of every page
(default: the title of the general index); `gencss` writes `style.css` into
`out` - the one of the templates folder when it exists, the built-in sheet
otherwise (default: false); `sitemap` asks for a sitemap (`-sitemap <base>`
on the command line): `base` is the absolute URL the site is served from,
its folder with the trailing slash (required), `file` the output name
(default `sitemap.xml`), `changefreq` and `priority` optional fields of
every `<url>`, `indexes` the priority of the index pages when it differs; `clean_urls`
drops the `.html` from every link, sitemap entry and search index entry
(default: false).
A key this build does not know is an error naming it, in the file and in
its `sitemap` object alike: dropped in silence, a typo and a stale binary
both look like an option that had no effect.
Every flag wins over the file, so the same `.site-def` can be written
somewhere else with `-out`.

`clean_urls` exists because a host that maps `/foo` to `foo.html` answers a
redirect when asked for `/foo.html`, so every internal link pays a round trip
before the page starts to load. The files on disk keep their extension either
way: only what points at them changes, and the general index becomes its own
folder - `./` inside a page, the base URL in the sitemap and the canonical.
It is off by default because a site built with it cannot be read from the
file system, nor served by anything that does not rewrite.

The `.doc-tool` file describes the documentation and knows nothing about
sites: the kinds, books and indexes `gensite` renders come from it (through
`doc_tool` or `-doctool`), the pages from the database.

### Templates

Go `html/template` files for the pages, plain for the style sheet:

| file | used for |
|---|---|
| `page.html` | topic and group pages |
| `index.html` | the composed pages: general index, book indexes, sections of their own, category pages |
| `style.css` | written to the site only with `gencss` |

The built-in ones are embedded in the binary - minimal HTML, standard tags,
no classes, no script, no style sheet: a header linking the general index,
the page, a footer - and `-export-templates <dir>` writes them out (never
overwriting) so they can be edited; any file of the same name in the
templates folder replaces the built-in one. A page template receives
`.Site`, `.Title`, `.Kind`, `.Books`, `.Categories`, `.Keywords`, `.Short`,
`.Source` (`path:line`), `.File`, `.Index` (the general index file),
`.Body` (the HTML), `.Trail` and `.Siblings`; the functions `join` and
`lower` are available.

`.Trail` is the page's logical location, as `resolve` materialized it: the
general index, the book index, the section (a page of its own, or the
heading of the index page - every heading carries an `id`) and, when the
section lists by category, the category page; each step has `.Title` and
`.File`. `.Siblings` are the other pages under the same last step, by
title: the functions of the same category, the classes of the same
section, the category pages of the same section. A site that has not been
resolved cannot be generated: `gensite` says so instead of guessing.

## `doccheck` — is the public surface documented?

The `.xbmac` registration list is the Xbase++ surface of the DLL, and only
that. `doccheck` cross-checks it against what the sources document:

```
ot4xb-tool [-q] doc check -src <dir> -xbmac <file.xbmac> [-full]
```

It scans the directory like `scandoc`, parses the list like `xbmac2h`, and
prints a summary plus one line per gap:

| finding | meaning |
|---|---|
| `UNDOCUMENTED function NAME (SRC: file.cpp)` | registered with `_XPP_REG_FUN_` / `_XPP_REG_WMAC` and no `function`, `internal-function` or `class` topic documents it — an Xbase++ class name is itself a registered function (its constructor), so a class doc counts |
| `UNDOCUMENTED structure NAME` | a `_XPP_REG_WST_` with no `class` topic (a GWST structure is a class) |
| `UNDOCUMENTED c-function name` | a `_CDECL_EXPORT_` with no `c-function` / `debug-c-function` topic (case-sensitive) |
| `UNREGISTERED function name (file:line)` | with `-full`: documented as a function but not in the list — stale doc, a wrong kind, or a name the list spells otherwise |

Only functions are checked in reverse: structures and C functions reach the
DLL through other registries (the WAPIST map, the `.def` / import library), so
comparing them to the `.xbmac` would be noise.

Known limit: a registered symbol is not always the documented name.
`ot4xb.ch` renames some exports with `#pragma Map( XBASE_NAME, "_SYMBOL" )`
(e.g. `UUIDCreate` is exported as `_UUIDCREATE` because Alaska ships a
`UUIDCreate` of its own) and the WAPIST structures are aliased by
`ot4xb_wapist_map.ch`. `doccheck` does not resolve those aliases yet, so such
a symbol shows up as undocumented although its Xbase++ name is.

## `compile` and `resolve` — the documentation database

The documented sources are compiled into an intermediate SQLite database —
the `.obj` of the documentation: never versioned, rebuilt at will, and the
single place every product (reference pages, lookups, changelogs) is
generated from afterwards. The sources stay the only truth.

```
ot4xb-tool [-q] doc compile [-doctool <file>] -root <projectdir> -db <file.db> -src <file|glob|dir> [-src …]
ot4xb-tool [-q] doc resolve [-doctool <file>] -db <file.db>
```

### `compile` — one pass per source

Every `-src` (a file, a glob - `*.*` is every file of a folder - or a
directory: its C/C++ sources, then the `.prg/.ch` of it and its subfolders)
is scanned with the Draft 4 scanner and written into the database under its
path relative to `-root` (`source/TBinFile.cpp`; `\` becomes `/`;
case-insensitive; alphabet `a-z 0-9 - . _ /`). The order of the arguments is
the parse order, and **the parse order is the document order**: nothing is
re-sorted across arguments, a file matched by an earlier argument is
discarded when a later one matches it again, and a pattern matching nothing
only warns. Put the C/C++ sources before the `.ch` headers, so the
annotations a header adds to an existing topic land after its main content.

Compilation is **multi-pass and append-only per source**: a pass registers
its file, deletes everything that file contributed before, and inserts it
again — it never looks at the other files. A file keeps its id and its
position across runs; a new file goes after the last. Each file is one
transaction: the whole ot4xb tree compiles in a few seconds.

What one pass writes: a **topic** per identity seen (created the first time,
reused after — a second block with the same identity, in this file or
another, only appends), a **segment** per marker (its raw text), a **field**
per `label: value` entry in written order, a **reference** per
`include-note-id` and per inline `{{ilink: …}}`, a **category** row per item
of every `category:` comma list. `_slug_` and `_tg_` are applied with the
rules of the spec: a computed slug is stored when the topic has none, an
explicit one replaces it and sets the flag, a second different explicit one
is refused and reported (the first stays); all the blocks of a topic group
must agree on its slug. Nothing is resolved during compilation: a reference
to something in another file — or in a file not compiled yet — is stored as
written.

### `resolve` — the a-posteriori step

Once every source is in, `resolve` verifies the references against the
topics and records what is broken as issues (code `resolve/…`), then prints
them as `file:line: error: message`. It then materializes the pages: the
`pages` and `page_trail` tables (see below) get one row per final page with
its logical location, computed from the books, indexes and sections of the
configuration — which is why `resolve` needs one: from `-doctool`, or,
failing that, from the database itself (see `cfg` below). The generators read
them and refuse a database that has not been resolved. It is re-runnable: its own issues are
dropped and recomputed every time.

| check | rule |
|---|---|
| missing target | `<kind ident>` looked up with the EXACT kind (no families); `<slug name>` among topic and group slugs, case-insensitively; `<tg name>` among the groups |
| include cycle | a note that includes itself, directly or through others; repetitions are fine |
| duplicate slug | two pages (topics without a group, or groups) sharing a slug case-insensitively; `gendoc` still writes both, suffixing the later file |

A topic with segments from several files is never an issue: that is how
scattered content and C++ overloads work.

### The tables

| table | columns | what |
|---|---|---|
| `sources` | `idsrc`, `pos`, `src` | one row per file; `pos` = parse order, stable across runs; `src` unique, case-insensitive |
| `topics` | `idtopic`, `kind`, `key`, `ident`, `slug`, `flags`, `idtg` | one row per documented thing; `(kind, key)` unique; `flags` bit 0 = slug written, not computed; `idtg` = its topic group or 0 |
| `topic_groups` | `idtg`, `idsrc`, `pos`, `name`, `key`, `slug`, `flags` | one row per `_tg_` name: the topics that render into one page |
| `segments` | `idseg`, `idtopic`, `idsrc`, `pos`, `line`, `raw`, `resolved`, `is_resolved` | one row per marker of a topic |
| `fields` | `idsrc`, `idseg`, `seq`, `label`, `value`, `hide_entry`, `hide_label` | every `label: value` of every segment, in written order, label canonical, visibility kept |
| `refs` | `idseg`, `idsrc`, `idtopic_in`, `reftokind`, `reftoident`, `reftype` | who references whom (`include`, `ilink`); `reftokind` is a kind, `slug` or `tg` |
| `issues` | `idsrc`, `idseg`, `line`, `severity`, `code`, `message` | the log, in the database (`scan/…`, `compile/…`, `resolve/…`) |
| `topic_category` | `idsrc`, `idtopic`, `category` | N:N — a topic belongs to one or more categories (`winapi/structures`) |
| `pages` | `file`, `title`, `kind`, `book`, `short`, `keywords`, `categories`, `parent` | written by `resolve`: one row per final page (topic, group, index, category page) with its first book, its short description and keywords as plain text, and `parent`, the file of the last step of its trail |
| `page_trail` | `file`, `pos`, `title`, `target` | written by `resolve`: the logical location of every page, step by step — general index, book index, section (`index-x.md#section`), category page |
| `cfg` | `k`, `v` | the part of the configuration that describes the documentation — `books`, `index`, `kinds` — one JSON document each, written by `compile` |
| `meta` | `key`, `value` | schema version (3), project data |

Every row carries `idsrc`, which is what makes "replace this file" a plain
delete. A **topic** is the whole documented thing; a **segment** is one
marker of it, so a class documented in three places is the union of its
markers, in order. `segments.pos` packs the file position (high bits) and the
position inside the file (low 20 bits) so a topic's segments sort with a
plain `ORDER BY pos`. `raw` is the marker text verbatim; `fields` is the same
content already split, so the description of anything is one `SELECT` away
(a short description, an index, a table by category: queries, not parsing).
`resolved` and `is_resolved` are reserved.

**Keys.** `key` is the normalized identity: Xbase++ symbols (functions,
classes) in upper case — their canonical form in the `.xbmac` and the export
table; C and C++ symbols as written, case-sensitive; doc ids (notes, topics)
in lower case. `ident` keeps the first spelling seen, for display. A topic
has no file and line of its own: its location is its first segment.

**Categories** come from the `category:` fields (comma lists allowed); there
is no categories table — a category exists because it is used, the list is
`SELECT DISTINCT category FROM topic_category`, and the `/` in the path is
the hierarchy.

**The configuration travels with the data.** `compile` stores the `books`,
`index` and `kinds` of the `.doc-tool` in `cfg`, right after it writes the
sources: at that moment the configuration is loaded and validated, and from
then on the database carries what generating needs. The rest of the file —
`db`, `src`, `folders` — never travels: it names paths of the machine that
compiled and means nothing anywhere else. So a repository holding only the
`.db` runs `resolve`, `gendoc` and `gensite` with no `.doc-tool` at all, and
one that keeps a `.doc-tool` for its own paths need not repeat the blocks in
it: every block the file leaves out comes from `cfg`. What the file declares
wins over what the database offers, and with neither the built-in
configuration applies.

The database is single-process by design: one connection, no concurrency;
another process wanting the data works on its own copy. Reads are always
materialized (`QueryAll`), never a live cursor. A database of another schema
version is refused: delete it and compile again.

## `gendoc` — the reference, one file per page

The first product generated from the documentation database: a flat folder
of Markdown files, **one per page**, plus an `index.md`. A page is a topic,
or a topic group — every topic that declared the same `_tg_`, the way the
overloads of a C++ function share one page. No other grouping: a folder with
all the content, that is the point.

```
ot4xb-tool [-q] doc gen [-doctool <file>] -db <file.db> -out <dir>
```

Run it on a compiled and resolved database. The output is CRLF.

### File names: slugs

A page's file is its slug plus `.md`. A topic names its own with `_slug_:`
in its header (`wapist_point`, `filetime64`); without one the tool computes
`kind-key` — `function-ft64_setts`, `cpp-function-json_ns.serialize` — from
the alphabet `a-z 0-9 _ - .` (lower-cased, `:` becomes `.`, anything else
`-`). A group's file is the group's slug (the name, unless a block wrote a
`_slug_`, which every block of the group must repeat). Two pages sharing a
slug are reported by `resolve`; the generator still writes both, suffixing
the later one (`-2`), so nothing overwrites anything.

### What a page holds

Exactly what was written, in the order it was written. A page is the
concatenation of the topic's segments (every marker, from every file, in
document order); within a segment, every field in turn:

- a labelled field renders as `**label:** value`; a label-hidden one
  (`desc_`) as its bare value; an entry-hidden one (`_slug_`) not at all; a
  `|:` field as its text. Multi-line values are dedented, a value that starts
  on its own line (`| params:` and a list below) keeps the label on its own
  line;
- the identity is the title (`# ident`; `## ident` for each topic of a
  group page under `# group`);
- `include-note-id` is replaced by the note's rendered body, right there —
  recursively, cycles cut;
- `{{ilink: <target> text}}` becomes `[text](page.md)` when the target
  exists, plain text otherwise; any other `{{label: value}}` inline renders
  as its bold label;
- within one marker, the entries that follow a list item continue that item
  on the same line (a hidden-label one after ` - `), so
  `|member_: - MEMBER LONG x | desc_: x coordinate.` reads
  `- MEMBER LONG x - x coordinate.` and consecutive members stay one list.

The tool adds no section, no table and no template of its own: the
`**BEGIN STRUCTURE**` lines, the `- MEMBER` items, the `See also:` text are
the author's. The indexes are the only pages the tool composes, and what
they hold comes from the `.doc-tool` file (see that section): the general
index, one or more index pages per book made of sections (a list by name, a
list of categories linking to one category page per book and category, or a
section that is a page of its own), and the category pages. Without a
`.doc-tool` file the built-in configuration applies: four books (C API, C++
API, Xbase++, Other), each with an index by category and an alphabetic page,
and an `index.md` linking them.
