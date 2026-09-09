---
title: doc-tool file
mkskill:
  pos: 85
---

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
