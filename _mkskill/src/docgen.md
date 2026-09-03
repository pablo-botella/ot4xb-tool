---
title: gendoc
mkskill:
  pos: 110
---

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
