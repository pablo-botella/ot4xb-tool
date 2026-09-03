---
title: xbmac2h
mkskill:
  pos: 40
---

## `xbmac2h` — code generation from the registration list

The `.xbmac` file is the registration list of an ot4xb-style DLL: one line per
function the DLL offers to Xbase++. `xbmac2h` generates from it the four files
the build needs, next to the list:

```
ot4xb-tool [-q] xbmac2h [-lib NAME] file.xbmac
```

| output | content |
|---|---|
| `<base>_xbexports.hpp` | `XPPRET XPPENTRY <sym>(XppParamList );` prototypes inside `extern "C"` |
| `<base>_xbfunclist.hpp` | `{"NAME",<sym>}` initialiser rows (the function table) |
| `<base>Cpp.def` | `LIBRARY`/`EXPORTS` with `NAME =  <sym>  PRIVATE` — the MSVC side |
| `<base>.def` | `LIBRARY`/`EXPORTS` with `NAME =  _<sym>` — the Xbase++ side, input of `def2lib20` |

`-lib NAME` sets the `LIBRARY` name of the `.def` files (default: the file name
without extension).

### The list

```
_XPP_REG_FUN_( name )      // plain Xbase++ function, C symbol NAME
_XPP_REG_WMAC( name )      // macro-style wrapper, C symbol wapimc_NAME
_XPP_REG_WST_( name )      // structure wrapper, C symbol wapist_NAME
_XPP_REG_WAPI( name )      // Win32 API wrapper, C symbol wapi_NAME
_CDECL_EXPORT_( name )     // a plain C function the DLL exports under its own name
```

Blanks are free; `//` starts a comment to the end of the line. The names of
the four registration commands are upper-cased (as the legacy tool did); a
`_CDECL_EXPORT_` name keeps its case, it is a C symbol. A `_CDECL_EXPORT_`
function is not an Xbase++ function: the two `.hpp` files get only a comment,
`<base>Cpp.def` gets nothing (a `PRIVATE` entry would drop it from the MSVC
import library) and `<base>.def` gets `name =  _name` so that Xbase++ clients
reach it through the `def2lib20` import library as well.

The formats are byte-compatible with the Harbour `xbmac2h` (CRLF, same
blanks), except that the comments of the `.xbmac` are discarded instead of
copied. Blank lines are kept; unrecognised lines are reported and marked in the
outputs exactly like the legacy tool did.
