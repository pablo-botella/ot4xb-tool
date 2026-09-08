---
title: overview
mkskill:
  pos: 10
  replace-macros: true
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

<$$$msk.install$$$>

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
| `resolve` | the a-posteriori step over that database: broken references, include cycles, slug clashes; then the pages with their logical location, materialized for the generators |
| `gendoc` | the reference Markdown from that database: one file per topic or topic group (slugs) plus an index |
| `gensite` | a static HTML site from that database, as a `.site-def` describes it (templates, assets); not a build step |

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
