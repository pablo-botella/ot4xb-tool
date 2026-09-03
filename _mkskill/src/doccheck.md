---
title: doccheck
mkskill:
  pos: 90
---

## `doccheck` — is the public surface documented?

The `.xbmac` registration list is the Xbase++ surface of the DLL, and only
that. `doccheck` cross-checks it against what the sources document:

```
ot4xb-tool [-q] doccheck -src <dir> -xbmac <file.xbmac> [-full]
```

It scans the directory like `scandoc`, parses the list like `xbmac2h`, and
prints a summary plus one line per gap:

| finding | meaning |
|---|---|
| `UNDOCUMENTED function NAME (SRC: file.cpp)` | registered with `_XPP_REG_FUN_` / `_XPP_REG_WMAC` and no `function`, `internal-function` or `class` topic documents it — an Xbase++ class name is itself a registered function (its constructor), so a class doc counts |
| `UNDOCUMENTED structure NAME` | a `_XPP_REG_WST_` with no `class` topic (a GWST structure is a class) |
| `UNDOCUMENTED c-function name` | a `_CDECL_EXPORT_` with no `c-function` / `debug-c-function` topic (case-sensitive) |
| `UNREGISTERED function name (file:line)` | with `-full`: documented as a function but not in the list — stale doc, a wrong kind, or a name the list spells otherwise |

Only functions are checked in reverse: structures and C functions reach the
DLL through other registries (the WAPIST map, the `.def` / import library), so
comparing them to the `.xbmac` would be noise.

Known limit: a registered symbol is not always the documented name.
`ot4xb.ch` renames some exports with `#pragma Map( XBASE_NAME, "_SYMBOL" )`
(e.g. `UUIDCreate` is exported as `_UUIDCREATE` because Alaska ships a
`UUIDCreate` of its own) and the WAPIST structures are aliased by
`ot4xb_wapist_map.ch`. `doccheck` does not resolve those aliases yet, so such
a symbol shows up as undocumented although its Xbase++ name is.
