---
title: srcsplit
mkskill:
  pos: 80
---

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
