---
title: build steps
mkskill:
  pos: 20
---

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
| `srcsplit` | `srcsplit` over `in` (file, glob or directory): the code projection to `code` and/or the doc projection to `doc` (`*` = the source base name, or its path under `in` with `"recurse": true`, which also walks the subfolders of a directory `in`); `force` overwrites, `bak` keeps a `.bak` |
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
