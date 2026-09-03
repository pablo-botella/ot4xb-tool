---
name: ot4xb-tool
description: ot4xb-tool
---

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
go install github.com/pablo-botella/ot4xb-tool/cmd/ot4xb-tool@latest    # the binary
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
| `scandoc` | scan sources for `/*{{ }}*/` documentation markers (Draft 4): list the topics and the issues |
| `srcsplit` | split an authoring source into its code projection (no doc blocks) and its doc projection |
| `doccheck` | cross-check the documented surface against the `.xbmac` registration list |
| `compile` | compile documented sources into the intermediate SQLite database (any number of passes) |
| `resolve` | the a-posteriori step over that database: broken references, include cycles, slug clashes |
| `gendoc` | the reference Markdown from that database: one file per topic or topic group (slugs) plus an index |

The build-side commands (`-bs`, `vbuild`, `xbmac2h`, `def2lib20`, `cbk2obj`)
replace the legacy Harbour/xppcbk tools of the ot4xb build, byte-compatible
where the old outputs are consumed by other tools. The documentation commands
(`scandoc`, `srcsplit`, `doccheck`, `compile`, `resolve`) form a pipeline: the
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
| `artefacts` | the artefacts module (release file layout) |
| *(composite)* | `cleanup.before` (delete files), then `xbmac2h`, then `copy` |

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
ot4xb-tool [-q] scandoc -src <file|dir> [-fields] [-issues]
```

| option | meaning |
|---|---|
| `-src` | one file, or a directory: its `.cpp/.c/.h/.hpp` files, then the `.prg/.ch` of it and its subfolders (`ch/`), sorted — the same order `compile` uses |
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
  block is text. Values are Markdown, continuation lines verbatim.
- The header's first entry is the identity (`kind: ident`). A marker that
  starts with `|` is a **fragment** of the open topic; `|:` is text placed
  right there. `include-note-id: X` transcludes note X at that position.
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
with two, content before the header or outside any topic), plus the CRLF
warning. Cross-file checks — missing link targets, include cycles, slug
clashes — belong to `resolve`, over the compiled database.

## `srcsplit` — the two projections of an authoring source

Some files cannot carry their own `/*{{ }}*/` documentation as shipped: an
Xbase++ header (`.ch`) must stay lean and Windows-1252, for one. So the file
is **authored** with the doc blocks in place, and `srcsplit` produces its two
projections — the *clean* one (the source with every doc block removed) and
the *doc* one (only the doc blocks, verbatim, in order):

```
ot4xb-tool [-q] srcsplit -src <file|glob> [-code <dst>] [-doc <dst>] [-bak | -force] [-check]
```

| option | meaning |
|---|---|
| `-src` | one source, or a glob (`ch/src/*.chsrc`) |
| `-code <dst>` | write the clean projection to `dst` |
| `-doc <dst>` | write the doc projection to `dst` |
| `-bak` | an existing, different destination is copied to `<dst>.bak` before being overwritten |
| `-force` | an existing, different destination is overwritten, no copy |
| `-check` | compare against the existing destinations and report drift, write nothing (the CI leg): exit 1 when any would change |

Give one or both: **a projection whose destination is not named is not
produced** — `-code` alone just yields clean sources, nothing else is written.
Extensions are not implied: you name every destination. A destination may
carry a `*`, replaced by the source's base name (`-src ch/src/*.chsrc -code
ch/*.ch -doc ch/doc/*.chdoc` turns `ot4xb.chsrc` into `ch/ot4xb.ch` and
`ch/doc/ot4xb.chdoc`); a destination without `*` is a single file, which is
only allowed when `-src` matches a single source.

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

## `doccheck` — is the public surface documented?

The `.xbmac` registration list is the Xbase++ surface of the DLL, and only
that. `doccheck` cross-checks it against what the sources document:

```
ot4xb-tool [-q] doccheck -src <dir> -xbmac <file.xbmac> [-full]
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
ot4xb-tool [-q] compile -root <projectdir> -db <file.db> -src <file|glob|dir> [-src …]
ot4xb-tool [-q] resolve -db <file.db>
```

### `compile` — one pass per source

Every `-src` (a file, a glob, or a directory: its C/C++ sources, then the
`.prg/.ch` of it and its subfolders) is scanned with the Draft 4 scanner and
written into the database under its path relative to `-root`
(`source/TBinFile.cpp`; `\` becomes `/`; case-insensitive; alphabet `a-z 0-9
- . _ /`). C/C++ sources go first and the `.ch` headers last, so the
annotations a header adds to an existing topic land after its main content;
within that, the order of the arguments is the parse order, and **the parse
order is the document order**.

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
them as `file:line: error: message`. It is re-runnable: its own issues are
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
| `meta` | `key`, `value` | schema version (2), project data |

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
ot4xb-tool [-q] gendoc -db <file.db> -out <dir>
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
the author's. `index.md` lists every page under its kind (and the groups),
linking to it.
