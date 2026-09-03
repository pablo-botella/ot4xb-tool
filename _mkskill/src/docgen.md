---
title: gendoc
mkskill:
  pos: 110
---

## `gendoc` — the reference, one file per topic

The first product generated from the documentation database: a flat folder
of Markdown files, **one per topic**, plus an `index.md`. No grouping yet — a
folder with all the content, that is the point.

```
ot4xb-tool [-q] gendoc -db <file.db> -out <dir>
```

Run it on a compiled and resolved database. Files whose bytes are already
right are not rewritten; the output is CRLF, UTF-8.

### File names: slugs

Every topic gets a file name stem from its kind and normalized key —
`function-array2ppmarshall`, `method-large_integer.new64`,
`note-con-get-long-ex`, `cpp-function-json_ns.serialize-xppparamlist` — using
only `a-z 0-9 _ - .`: lower-cased, `:` and `::` become `.`, anything else
becomes `-`, runs collapse. Case is dropped on purpose (a Windows file system
would merge `Foo.md` and `FOO.md` anyway); the topics that then collide all
get a short hash of their exact `kind|key` appended, so the result never
depends on order.

A topic may name its own file with an **explicit slug** — `| slug: fpqcall`
on its marker (entities, members and notes alike). It wins over the computed
one; it is the way to give a page a stable, known name, and the only way to
have internal links to it clear. Explicit slugs are compared
**case-insensitively**: an invalid one (outside the alphabet) is reported and
the computed name used; the same explicit slug on several topics is an
authoring error — reported with `file:line`, and all of them are suffixed so
none overwrites another.

### What a page holds

- The title (the identity as written), the kind, its categories.
- Per segment (a topic documented in several places has several, in
  document order): its source `file:line`, then the content, re-parsed from
  the stored marker text by the same scanner: syntax, description,
  parameters, return, flags, examples, notes, and every other field as
  `name: value` — unknown fields included, nothing is dropped.
- For a class or structure: a **Members** list linking to each member's own
  page (members are topics too). For a topic: its commands.
- **References**: included notes, parents, links, see-also and calls — as
  links when the target exists, as plain text otherwise. Included notes are
  linked, not expanded: content resolution is a later step.

`index.md` lists every topic under its kind, linking to its page.
