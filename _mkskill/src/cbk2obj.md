---
title: cbk2obj
mkskill:
  pos: 60
---

## `cbk2obj` — callback objects

An Xbase++ program that hands a callback to a Win32 API needs a small stub
per callback: native code with the right calling convention that jumps into
the Xbase++ side. `cbk2obj` compiles a callback script (`.cbk`, the format of
the legacy `xppcbk` tool) straight into a linkable x86 COFF object:

```
ot4xb-tool [-q] cbk2obj [-asm] [-o out.obj] [-ts seconds] file.cbk
```

| option | meaning |
|---|---|
| `-o out.obj` | output object (default: the `.cbk` name with `.obj`) |
| `-asm` | also write the equivalent FASM source (`.asm`) next to the object, for inspection |
| `-ts seconds` | COFF timestamp, seconds since the Unix epoch, for reproducible outputs |

The script is read line by line (CR, LF or CRLF), as bytes. Parsing is a
hand-written scan; errors do not stop it: every faulty line yields a
diagnostic with its line number and the legacy message, and parsing goes on,
as `xppcbk` did. A `XPPCBK VERSION v` line above the version this tool accepts
is an error.

The generated stubs call their runtime helpers through the ot4xb import
library (`ot4xb.lib`), never by runtime injection; the object links like any
other. See the ot4xb sources for the scripts in use.
