---
title: vbuild
mkskill:
  pos: 30
---

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
