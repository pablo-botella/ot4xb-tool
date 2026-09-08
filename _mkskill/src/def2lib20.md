---
title: def2lib20
mkskill:
  pos: 50
---

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
