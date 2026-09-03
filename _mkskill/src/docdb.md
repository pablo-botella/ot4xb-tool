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
ot4xb-tool [-q] compile -root <projectdir> -db <file.db> -src <file|glob|dir> [-src …]
ot4xb-tool [-q] resolve -db <file.db>
```

### `compile` — one pass per source

Every `-src` (a file, a glob, or a directory meaning its C/C++ sources) is
scanned and written into the database under its path relative to `-root`
(`source/TBinFile.cpp`; `\` becomes `/`; case-insensitive; alphabet `a-z 0-9
- . _ /`). The order of the arguments is the parse order, and **the parse
order is the document order**.

Compilation is **multi-pass and append-only per source**: a pass registers its
file, deletes everything that file contributed before, and inserts it again —
it never looks at the other files. So sources can be compiled in any number
of runs, and recompiling one file replaces exactly its rows. A file keeps its
id and its position across runs; a new file goes after the last.

Nothing is resolved during compilation: a reference to something in another
file — or in a file not compiled yet — is stored as written.

### `resolve` — the a-posteriori step

Once every source is in, `resolve` verifies the references against the
topics and records what is broken as issues (code `resolve/…`), then prints
them as `file:line: error: message`. It is re-runnable: its own issues are
dropped and recomputed every time.

| check | rule |
|---|---|
| missing target | a reference whose `(kind, identity)` exists nowhere; `class` and `structure` are one family, either name finds either |
| include cycle | a shared note that includes itself, directly or through others (transclusion cannot loop); repetitions are fine |
| duplicate definition | an identity with several segments where the kind cannot reopen — `class`, `structure` and `cpp-class` may be documented in several blocks or files, nothing else may |
| skipped | `see-also` and `calls` carry bare names and are not resolved yet; they are migrating to `ilink` |

### The tables

| table | columns | what |
|---|---|---|
| `sources` | `idsrc`, `pos`, `src` | one row per file; `pos` = parse order, stable across runs; `src` unique, case-insensitive |
| `topics` | `idtopic`, `kind`, `key`, `ident` | one row per documented thing, created the first time a segment of it is seen; `(kind, key)` unique |
| `segments` | `idseg`, `idtopic`, `idsrc`, `pos`, `line`, `raw`, `resolved`, `is_resolved` | one row per contribution of a file to a topic |
| `refs` | `idseg`, `idsrc`, `idtopic_in`, `reftokind`, `reftoident`, `reftype` | who references whom |
| `issues` | `idsrc`, `idseg`, `line`, `severity`, `code`, `message` | the log, in the database |
| `topic_category` | `idsrc`, `idtopic`, `category` | N:N — a topic belongs to one or more categories (`winapi/structures`) |
| `meta` | `key`, `value` | schema version, project data |

Every row carries `idsrc`, which is what makes "replace this file" a plain
delete. A **topic** is the whole documented thing; a **segment** is what one
file says about it, so a class documented in three places is three segments
of one topic, in order. `segments.pos` packs the file position (high bits)
and the position inside the file (low 20 bits) so a topic's segments sort
with a plain `ORDER BY pos`. `raw` is the marker text of the segment,
verbatim and in order — never the C++ between markers; it is re-parsed by
the same scanner when a product is generated. `resolved` and `is_resolved`
are reserved for the a-posteriori content resolution (not built yet).

**Keys.** `key` is the normalized identity of the topic's family: Xbase++
symbols (functions, classes, structures, commands and their members) in
upper case — their canonical form in the `.xbmac` and the export table; C and
C++ symbols as written, case-sensitive; doc-internal ids (notes, topics) in
lower case. `ident` keeps the first spelling seen, for display. Components
are topics too: `method LARGE_INTEGER:New64`.

**Categories** come from the `category:` fields; there is no categories
table — a category exists because it is used, the list is
`SELECT DISTINCT category FROM topic_category`, and the `/` in the path is the
hierarchy.

The database is single-process by design: one connection, no concurrency;
another process wanting the data works on its own copy. Reads are always
materialized (`QueryAll`), never a live cursor.

### What is generated from it

Nothing yet: the reference Markdown (one file per topic), the agent lookup
over `doc_fts` and the changelog from `since`/`deprecated` are the next
steps, and all of them are a query plus a template over this database.
