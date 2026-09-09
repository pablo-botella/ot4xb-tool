---
title: compile and resolve
mkskill:
  pos: 100
---

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
