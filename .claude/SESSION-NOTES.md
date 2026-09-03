# ot4xb-tool — session handoff notes

Notes to pick up work in a fresh session. Repo:
`C:\HD\F\__pbnprj\_pub\xb\ot4xb-tool` (module `github.com/pablo-botella/ot4xb-tool`,
MIT, Go 1.26, one external dep: `github.com/pablo-botella/linereader`).

## What this is

A single Go binary (`ot4xb-tool`) that replaces the dead pre/post-build utilities
of the **ot4xb** Xbase++ DLL project, so ot4xb builds end to end without the old
Alaska helpers or `c:\util` exes. Each tool is a subcommand backed by a package
under `modules/`; a build-step runner drives them from a JSON project file.

The real ot4xb project lives at
`C:\HD\F\__pbnprj\_pub\xb\_ot4xb_\_ot4xb_\_ot4xb_` (git repo `_ot4xb_`); its build
sources are under `...\source\`. The tool file and its user companion already
exist there: `ot4xb.ot4xb-tool` (repo root) and `ot4xb.ot4xb-tool.user`.

## Conventions (hard rules)

- **Windows only.** File handling is Windows-minded: `/` and `\` interchangeable,
  glob/path matching **case-insensitive** (`*.hpp` matches `CRC32.HPP`).
- **Files the tool reads/writes are ANSI (Windows-1252) + CRLF**; bytes are never
  transcoded. `.go` and `.md` are UTF-8. Keep every non-.go/.md file CRLF. When
  an EOL has to be converted, it becomes CRLF (everything we produce is for Windows).
- **Repo is English** (comments, docs, messages). Conversation with Pablo is
  Spanish.
- **Git is Pablo's only.** Never commit/add/tag/push. Leave the working tree ready
  and say so; give exact commands if a commit is needed.
- **Containment (Pablo's global CLAUDE.md):** do exactly what is asked. Talking
  design is not a green light to write code — wait for an explicit verb
  ("hazlo", "haz el módulo", "adelante"). "espera" = stop at once. In a
  spec/review conversation, words only, no tool calls until asked. Pablo answers
  design questions in short successive messages; "me faltan datos" is a valid answer.

## Parsers use linereader + FSM

Producers read lines with `github.com/pablo-botella/linereader` (CR/LF/CRLF,
bytes). Line/format parsers are **hand-written FSMs over bytes — no regexp**
(Pablo asked for this explicitly; applies to every script the tool reads).

## Modules — all DONE, tested, verified

`go test ./...` is green (6 packages). Each module = producer file(s) + consumer
file(s) in one package (Pablo wanted the producer/consumer split at the .go-file
level, not separate packages).

- **`modules/def2lib20`** — `.def` → x86 COFF import library (.lib), classic
  "long" format that Alaska **ALINK** reads (the only format it accepts; VC6+
  short import objects are rejected). Files: `def.go` (producer: `.def`→`Def`,
  linereader), `coff.go` (consumer: COFF objects + `!<arch>` archive),
  `def2lib20.go` (API `Build`/`BuildFile`, `Options`), `doc.go`.
  `Options.Monkey` (bool) = imitate Alaska `aimplib`: names
  `<dll>_IMPORT_DESCRIPTOR` / `NULL_IMPORT_DESCRIPTOR` / `<dll>_NULL_THUNK_DATA`
  and hint `0xFFFF` (shares the terminator with the Xbase++ runtime libs, so the
  linked `.exe` is byte-identical to one built with aimplib). Off = MS LINK 2.60
  names. **Verified**: generated `ot4xb.lib` (909 imports) links a test program
  with ALINK and runs; with `-monkey` the exe differs from the aimplib-linked one
  only in the PE timestamp. Spec of the format: `pruebas/pecoff-implib-spec-notes.md`.
  FYI found 2026-08-20: the real `ot4xb.def` has a duplicate export
  `WAPIST_NMSELCHANGE` (line 5468); def2lib20 warns and ignores it.
- **`modules/xbmac2h`** — `.xbmac` registration list (`_XPP_REG_WAPI/WST_/WMAC/FUN_`)
  → four files: `<base>_xbexports.hpp`, `<base>_xbfunclist.hpp`, `<base>Cpp.def`,
  `<base>.def`. Files: `xbmac.go` (producer), `xbmac2h.go` (consumer generators +
  `Generate`). Only `//` comments; comments discarded, blank lines kept, unknown
  lines marked like the legacy tool. **Verified** command-line-identical to the
  Harbour `xbmac2h.exe` on the real `ot4xb.xbmac` (the legacy exe additionally
  wraps lines at col 79 and uppercases LIBRARY only when given a path — we do
  neither). **New 2026-08-20: `_CDECL_EXPORT_( name )`** (Kind `CdeclExport`,
  name keeps its case, not an `IsCommand()` kind): a plain C function the DLL
  already exports from its source that Xbase++ clients must import too. Output:
  `<base>.def` → `     name =  _name` (def2lib20 then imports it, symbol = `name`);
  `<base>Cpp.def` → **nothing** (a PRIVATE entry would drop it from `ot4xb_cpp.lib`;
  that is why Pablo had those lines commented out in the old pipeline); the two
  `.hpp` → `// _CDECL_EXPORT_( name )` comment. Tests cover it.
- **`modules/vbuild`** — the VersionAutoIncrement replacement: version script
  (`.VersionInfo`). Files: `script.go` (producer: header FSM + body lines),
  `macros.go` (`{$<...>$}` expander incl. the masked `{$<FILEVERSION(:mask:)>$}`),
  `vbuild.go` (`Run`). Header `[Product]  Version:{maj,min,hbuild,lbuild}`
  (0..255). Increment only with `-inc` (default: none); `:noinc` product freezes;
  overflow past major=255 is an error. Components: `major/minor/hbuild/lbuild`,
  `build`=`lbuild` (the 16-bit `hbuild*256+lbuild`). Macros incl. the mask with
  `<maj> <min> <hbuild> <lbuild> <build>` and width `<maj(03)>`=%03d /
  `<maj(3)>`=%3d. Output EOL `-eolrn`(default)/`-eolr`/`-eoln`/`-eols`(keep each
  line's own). SYSTEMTIME(19) typo fixed vs legacy. Spec: `_mkskill/src/vbuild.md`.
  **Verified** on the real `ot4xb.VersionInfo`.
- **`modules/artefacts`** — the packager. `Glob` (Windows glob, wildcards in the
  last path element only) + `Zip` (native Go zip, creates parent dirs, flattens
  each file into its zip folder, warns on no-match/duplicate). Consumes a
  `[]Content{In []glob, Out string}` structure. Dumb module: macros are resolved
  by the runner before calling it.
- **`modules/vsxbt`** — the **build-step runner**. Loads a `<project>.ot4xb-tool`
  JSON and runs one entry (`pre.release`, `post.debug`, ...). Files: `vsxbt.go`
  (load + entries + template + .user), `expand.go` (macro/path resolver + version),
  `steps.go` (step execution; the `def2lib20` step accepts `in`, `out`, `monkey`,
  `prefix`, `dll`). **Verified** end to end on a copy of the real project.
  No `cbk2obj` step: xppcbk is for applications, not for building libraries.
- **`modules/cbk2obj`** — **NEW 2026-08-20**: the xppcbk port. `.cbk` script →
  linkable x86-32 COFF `.obj` written directly (no FASM), optional `.asm`.
  Files: `script.go` (producer: linereader + byte scan, `Parse`/`ParseFile`,
  `Script`/`Callback`/`Kind`/`Diag`, `Templates`, `KindOf`), `codegen.go`
  (frame layout + instruction model with the exact legacy text per instruction),
  `asm.go` (FASM text, CRLF), `obj.go` (x86 encoder + COFF writer laid out like
  FASM's: all section data, then relocs, symbols: externs, `.text`+publics,
  `.data`), `cbk2obj.go` (`Build`, `BuildFile`, `Options{Timestamp, Asm}`,
  `Result`, `ErrScript`, `ErrNoCallbacks`), `doc.go` (syntax, templates, PRG
  conventions). Testdata: `sample.cbk` (the spec input), `sample_legacy.asm`
  (XPPCBK.EXE 1.0.17 output, reproduced **byte for byte** by the test-only legacy
  switch `genOptions{legacy:true, seed:60046}`), `sample.asm` (our golden;
  `CBK2OBJ_UPDATE=1 go test` rewrites it). `TestFasmDifferential` (env `FASM` =
  path of FASM.EXE, e.g. `C:\HD\F\__pbnprj\_pub\xb\dev-tools\xppcbk\FASM.EXE`;
  skipped otherwise) assembles our `.asm` and checks our `.obj` is **byte-identical**
  to FASM's beyond the header timestamp.
  Code = legacy templates with the defects fixed: C1 VOID leaves eax alone,
  C2 QWORD high dword in **edx**, C3 helper names with ONE underscore everywhere
  (`_conPutFloat/_conGetFloat/_conPutQWord/_conGetQWord`; the legacy `extrn` side
  was right, its `call __conGet...` was the bug), C4 `_xbfn_<N>` numbering from 1,
  C5 non-zero exit, C7 real tokenizing (`__CDECL` alone OK), C8 CR/LF/CRLF, C9 exact
  template match, C11 bad PARAM type = error, **C14 (new): no `__fltused`** (FASM
  keeps an unreferenced extrn in the symbol table and ALINK fails on it: no Alaska
  lib defines it — the legacy could never link DOUBLE/FLOAT scripts). Integer
  results via ot4xb's `_conGetLong` (keeps the int32 representation). Kept: C6
  (`USING` covers one callback), `XPPCBK VERSION` check (> 001.000.017 = error),
  `_CALLBACK_<NAME>` public name (PRG calls `_CALLBACK_<NAME>()`; XPP references
  plain upper-case symbols, no underscore added).
  **Verified end to end** (scratchpad `e2e`): `sample.cbk` → `.obj`, Xbase++ test
  program calling the six thunks through `nFpCall`/`qwFpCall`/`ndFpCall`, linked
  with ALINK against `ot4xb_plus.lib` (= ot4xb.def + the five helpers through
  def2lib20, no prefix, monkey — the shape `ot4xb.lib` will have) + `ot4xb.dll`:
  stdcall/cdecl, DWORD/BOOL/VOID/QWORD/DOUBLE results, DWORD/DOUBLE/BOOL/QWORD/
  FLOAT/WORD/BYTE params all correct. The e2e PRG is only in the scratchpad (ask
  Pablo whether he wants it under `pruebas/`).

## CLI (`cmd/ot4xb-tool/main.go`)

`-q` is a **global** flag (before the command): suppress normal output, errors
still go to stderr. Subcommands:

    ot4xb-tool [-q] [-tool file.ot4xb-tool] -bs entry
    ot4xb-tool [-q] vbuild [-inc major|minor|hbuild|lbuild|build] [-eolrn|-eolr|-eoln|-eols] file.VersionInfo
    ot4xb-tool [-q] xbmac2h [-lib NAME] file.xbmac
    ot4xb-tool [-q] def2lib20 [-monkey] [-o out.lib] [-dll name.dll] [-prefix _] [-ts n] file.def
    ot4xb-tool [-q] cbk2obj [-asm] [-o out.obj] [-ts n] file.cbk

`cbk2obj` prints script errors to stderr as `line: <n>  error: <msg>` (legacy
format) and exits 1 with "N script errors. No code generated". `-bs` with no
`-tool` finds the single `*.ot4xb-tool` of the current folder. Can also run
without building: `go run github.com/pablo-botella/ot4xb-tool/cmd/ot4xb-tool@latest ...`.

## The tool file (`<project>.ot4xb-tool`, JSON)

Drives the runner; replaces the old `pre/postbuild_*.bat`. Rules:
- JSON is **lenient**: `//` line comments and trailing commas OK.
- Every relative path resolves against the **folder of the tool file** (not the
  cwd). VS invokes it with cwd = source folder, but paths are anchored to the file.
- Path normalization: everything to `/`, collapse doubled separators at macro
  junctions, hand `\` to disk. Accepts `\` in input.
- `folders` + `vars` of an entry are one namespace, referenced as `<name>`
  (case-insensitive, may reference each other). Version macros `<v.maj>` `<v.min>`
  `<v.hbuild>` `<v.lbuild>` `<v.build>` read the header of the `versioninfo`
  script; width after a colon: `<v.maj:03>`=%03d, `<v.maj:3>`=%3d. Unknown macro
  = error (loud).
- `"template": "other"` starts the entry as a copy of another; folders/vars merge
  by name (entry wins), the rest replaces.
- Companion **`<toolfile>.ot4xb-tool.user`** (personal, git-ignore it, never
  tracked): auto-detected; after the main entry runs, the `.user` entry of the
  same name runs inheriting the main entry's folders/vars. Pablo's personal
  dev-time deploy (copies the last build to `C:\pli\ot4xb`).
- Steps: named (`vbuild` with `flags`, `xbmac2h`, `def2lib20`, `artefacts`) or a
  composite by keys in fixed order (`cleanup.before` → `xbmac2h` → `copy`).
  `copy` items take `{in:[globs], out, create}`; `create:false` = skip if the
  target folder is missing.
- `cleanup.before` deletes only what the tool can't infer.

The real files: `ot4xb.ot4xb-tool` (versioninfo + pre.release [vbuild + codegen:
cleanup.before + xbmac2h] + pre.debug[template] + post.release [def2lib20
`<src>/ot4xb.def` → `<dst>/ot4xb.lib` monkey + artefacts: two zips] + post.debug)
and `ot4xb.ot4xb-tool.user`. Bin zip = dll + `*.lib` + `*.txt` at `/`, `.ch` at
`/include`; source zip = `ot4xb.sln` + `ot4xb.ot4xb-tool` at `/`, `source/`, `source/ch`.

## Facts verified on 2026-08-20 (Xbase++ 1.90.355, ALINK 1.90.355, ot4xb 1.7.12)

- **ALINK + two long-format import libs for the same DLL**: with the same
  (monkey) descriptor symbol ALINK extracts one descriptor and hangs under it only
  the ILT/IAT slots starting at that descriptor's own library → works or dies
  silently **depending on link order**. With distinct descriptor symbols (second
  lib without monkey) both descriptors are emitted (two `ot4xb.dll` entries in
  the import directory) and it works in both orders. Pablo chose to avoid the
  question altogether (design E below).
- **XPP symbols**: the compiler references functions as plain upper-case names
  (`OT4XB`, `QOUT`, `SPIKE_GETLONG`), no underscore.
- **`__fltused`** is defined by no Alaska lib (XppRt0/XppRt1/...) nor ot4xb; an
  unreferenced `extrn` of it makes ALINK fail.
- `__conCall`, `__conCallPa`, `__retnl`, `__conNew`, `__conRelease`, `__conPut*`,
  `__conGet*` come from `XppRt1.lib`, always linked by ALINK (default libs:
  XPPRT0.LIB, XPPRT1.LIB, XPPSYS.LIB).
- ot4xb's `_conCallCon/_conCallLong/...` family is **C++ only** (overloads,
  `ot4xb_cpp.lib`, which ALINK cannot read) — not usable from generated code.
- ot4xb QWORD convention: eight-byte character values both ways.
- `nFpCall(fp, ...)`: numerics → LONG, logicals → BOOL, strings → LPSTR, `NIL`
  before a numeric → double, `NIL` before a string → 8-byte integer;
  `ndFpCall` returns a double, `qwFpCall` the raw 8 bytes. Handles stdcall and
  cdecl callees. Perfect for testing thunks without a Win32 API.

## Design E — how the callbacks reach the ot4xb helpers (decided by Pablo)

ALINK cannot read `ot4xb_cpp.lib` (MS short format), so the five C helpers the
thunks need (`_conGetLong`, `_conPutFloat`, `_conGetFloat`, `_conPutQWord`,
`_conGetQWord`) must come through **`ot4xb.lib`**: ot4xb registers them in
`ot4xb.xbmac` with `_CDECL_EXPORT_( _conGetLong )` etc., xbmac2h writes them into
`ot4xb.def` (`_conGetLong =  __conGetLong`), def2lib20 (no prefix) imports them
with symbol `_conGetLong`, and the generated code calls exactly those names.
Nothing is added to `ot4xbCpp.def` (exported from the source already; PRIVATE
would drop them from `ot4xb_cpp.lib`). `ot4xb.lib` and `ot4xb_cpp.lib` are never
linked in the same project. An application links `callbacks.obj` + `ot4xb.lib`
(+ the default Alaska libs); nothing else. The earlier idea of a separate
`xppcbk.lib` (`-libout`) was dropped.

## mysql4xb converted to the tool (2026-08-20)

`C:\HD\F\__pbnprj\_pub\xb\mysql4xb` (git repo; project in `mysql4xb\mysql4xb\`,
same layout as ot4xb: `source\`, `Release\`, `Debug\`, `.sln` at the project
root). Done at Pablo's request ("conviértelo a este nuevo tool"):
- New `mysql4xb\mysql4xb\mysql4xb.ot4xb-tool` (CRLF, tabs), a copy of ot4xb's
  with the names changed: versioninfo, pre.release (vbuild no inc + codegen:
  cleanup.before obj/def/hpp + xbmac2h), pre.debug (template), post.debug
  (def2lib20 monkey → `Debug\mysql4xb.lib`), post.release (def2lib20 +
  artefacts: `_<bo>.zip` = `<dst>/*.lib` + `mysql4xb.dll` + `source/*.txt` at `/`;
  `_<bo>_source.zip` = sln + tool file at `/`, sources/def/rc/txt/xbmac/
  VersionInfo/vcxproj/filters in `/source`). No `/include` (no `ch` folder), no
  `*.bat` in the source zip, **no autodeploy** (`mysql4xb_extract_all.bat`,
  `_<bo>_deploy.bat`, `c:\util\*deploy*` hooks dropped — Pablo: "olvídate del
  autodeploy ese"), **no `.user` yet** ("luego metemos un user").
- `source\mysql4xb.vcxproj`: the four build events now run
  `ot4xb-tool -tool "$(ProjectDir)..\mysql4xb.ot4xb-tool" -bs pre.debug|post.debug|pre.release|post.release`
  (`-tool` because VS's cwd is `source\`; `$(ProjectDir)..` works with or
  without the solution). Edited with PowerShell: UTF-8 no BOM, CRLF intact.
- Dry run on a scratchpad copy: all four entries exit 0; regenerated `.def` /
  `Cpp.def` export lists identical to the legacy ones, `.rc` / `_version.h`
  byte-identical, VersionInfo untouched; zips created (the dll was missing in
  the current `Release\`, so only a warning).
- `source\mysql4xb.VersionInfo` trimmed (Pablo: "sigue"): the three
  `{$<File:>$[...]$}` blocks generating `__mklib.bat`, `mysql4xb_extract_all.bat`
  and `__mk_package.bat` were removed (158 → 56 lines, CRLF, original kept in
  the scratchpad); LICENSE.TXT, `mysql4xb_version.h` and `mysql4xb.rc` remain
  and regenerate byte-identical (LICENSE.TXT only moves the year macro).
- `ot4xb-tool.exe` is on the PATH: the repo is **published**
  (`https://github.com/pablo-botella/ot4xb-tool`, tag `v0.0.0` = commit
  `f65a190` of 2026-08-20 18:23 local, which already includes cbk2obj and the
  `_CDECL_EXPORT_` xbmac2h; local `main` = `origin/main`, only `.claude/` and
  `pruebas/` untracked) and Pablo installed it with
  `go install github.com/pablo-botella/ot4xb-tool/cmd/ot4xb-tool@latest`
  → `C:\Users\pbn\go\bin\ot4xb-tool.exe`. The seven old bats of `source\`
  (`prebuild_*`, `postbuild_*`, `__mklib.bat`, `__mk_package.bat`,
  `mysql4xb_extract_all.bat`) were deleted at Pablo's request (git shows them
  as `D`). **Real VS "Rebuild all" (Release) verified on 2026-08-20 18:51**: pre
  and post events ran through the tool, 0 errors, bin zip = `mysql4xb.lib` +
  `mysql4xb_cpp.lib` + `mysql4xb.dll` + `LICENSE.TXT`, source zip = 25 files.
  Only noise: `EXEC : warning : artefacts: ...\*.c: no files match` (the project
  has no `.c`; drop the pattern from the tool file if Pablo wants silence).
  Asset added afterwards at Pablo's request: `<dst>/libmysql.dll` (the MySQL
  client 5.7.43 the DLL wraps; it lives in `Release\`, git-ignored, put there by
  `Release\__get_runtime.bat` from `C:\pli\mysql-5.7.43-win32\lib\`) goes to the
  root of the bin zip next to `mysql4xb.dll` — on a fresh clone without it the
  zip just lacks it with a warning. The `.user` for the `C:\pli\mysql4xb`
  deploy comes later.
- **Build dependencies parametrised** (Pablo chose the names): new
  `mysql4xb\Directory.Build.props` at the repo root defines `MYSQL_CLI_DIR`
  (default `C:\pli\mysql-5.7.43-win32`) and `OT4XB_DIR` (default `C:\pli\ot4xb`)
  only when not already set (env var / `msbuild /p:` win) and imports a
  git-ignored `local.props` if present (`local.props` added to `.gitignore`).
  The `.vcxproj` now uses `$(MYSQL_CLI_DIR)\include|lib`, `$(OT4XB_DIR)\cppinc`
  / `$(OT4XB_DIR)` and `$(ProjectDir)..\xpp` (was `$(SolutionDir)xpp`, which
  broke outside the solution). `c:\pli\atl` was dropped entirely: the folder
  does not exist and the sources include no ATL (VS ships ATL 14.00 anyway).
  Verified with `msbuild -getProperty/-getItem` (defaults, overrides,
  evaluated include dirs); a VS rebuild after this change is still Pablo's to
  run. Noted: `C:\pli\vcpkg\installed\x86-windows\include` shows up in the
  evaluated includes — Pablo has vcpkg user-wide integration.
- **ot4xb itself is still wired to the old bats** (`call prebuild_release.bat
  $(IntDir)ot4xb.obj` etc. → `c:\util\VersionAutoIncrement.exe`, `aimplib`,
  `__mk_package.bat`); its `ot4xb.ot4xb-tool` is verified but VS does not call
  it yet. Same rewiring as mysql4xb when Pablo says so.

## Documentation system — design in progress (ot4xb sources, 2026-08-20 evening)

Pilot on mysql4xb (smaller), then ot4xb. Pablo's goal: **proximity** — the line
that registers a function/member/method documents it. Findings: the `<xbdoc>`
XML blocks in ot4xb's `.cpp` are all well-formed (FileTime.cpp: 26/26 pass a
strict parser) but fail the goal for classes (a parallel `<method>` list far from
the `MethodCB` calls, without parameters); **markdown as target was discarded**;
Pablo does not want any program written yet. He started a new marker format in
`_ot4xb_\source\FileTime.cpp` around `wapist_FILETIME64` (I renamed to single-token
kinds at his request): `/*{{begin-class}}*/` … `/*{{end-class}}*/` scope, one
`/*{{ kind: value [intrinsic attrs] | key: value | … }}*/` per registration
(`class name:`, `gwst-class [| gwst-parent: X] [| parent: Y]`, `gwst-member: n
type: T pos: N | desc: …`, `method: Sig(…) | return: … | desc: … | param x: …`).
At Pablo's request I then wrote the markers for the remaining 41 methods and
the 8 properties of FILETIME64 (trailing one-liner when there are no params,
preceding multi-line block otherwise; texts copied from the `ft64_*` xbdoc
blocks; types with alternatives written with `/`; 57 markers, file still CRLF,
backup `scratchpad\FileTime.cpp.before-markers`). Found while doing it: the
class xbdoc says `ToLocalTime` returns Self but its code block
`{|s| FT64_TOLOCALTIME(s)}` returns NIL (no `,s`); documented as NIL.
Pending decisions: `|` inside text (use `/` for type alternatives?), `}}` never
inside, `class name:` → `class-name:`?, kinds for class methods / class vars /
plain functions (I used `property: name type: T | desc:` for PropertyCB), and
whether the xbdoc blocks are retired. **The whole design is written up in
`C:\HD\F\__pbnprj\_pub\xb\dev-tools\go-port\03-srcdoc-spec.md`** (draft 1,
English, CRLF): marker grammar, the three annotation forms (markers, `<xbdoc>`,
external documents with front matter incl. `.prg` examples), identity keys,
derivation from code blocks, the SQLite intermediate DB ("like an .obj",
modernc.org/sqlite, disposable, `-keep`), annotation precedence by form,
inheritance as a recursive CTE (parents complementary, clashes = issue), the
check leg, outputs, and the open decisions. Later the same evening the format
was settled further (entities / seven auxiliaries / fragments; standalone
fragment markers inside scopes; `calls:` from a method to the function it
wraps; external `.md` = extra notes with `xbdoc:` front matter; `source` = file
name only; **the tool reads only the comments, never the C code**; per-project,
non-persistent model) and **`FileTime.cpp` was fully converted** (25 function
scopes with the doc block inside, the 26 XML blocks removed, the 42 methods
reduced to signature + return + `calls:` + desc; backups in the scratchpad).
The spec is current — read it first. 2026-08-21: definitive layout set by Pablo
(scope markers on their own lines, doc block before the header, 120/140 columns,
continuation lines, backticks for code), `class-name:`, entity `c-function`
(`begin-c-function`/`end-c-function`, `header`/`prototype`/`xbase-syntax`,
separate C/C++ manual), `cpp-class` reserved; files converted so far:
FileTime.cpp, array2dbf.cpp, Bitwise.cpp (15 function + 14 c-function); the
converter is `scratchpad\xbdoc2markers.ps1` (xbdoc / ot4xb-c / ot4xb-api
single-function blocks; classes and group roots reported, not converted).
2026-08-21 (later): ClrTool.cpp (7 c-function, `winapi/color`, written from
scratch) and conCallEx.cpp (80 `OT4XB_API` C++ overloads of the `_conCall*`
family, category `ot4xb-api`) are done. Pablo's rules for overloads: **one scope
per overload with its own prototype** (not one block per family), a new field
`mangled-name:` with the MSVC decorated name, and — decided right after — a new
entity **`cpp-function`** (`begin-cpp-function`/`end-cpp-function`) for the exports
that are NOT `extern "C"`; `c-function` is now the `extern "C"` ones only. The
`category: ot4xb-api` stays on them ("siempre puedo recategorizarlas luego").
Bitwise.cpp got `mangled-name:` on its 14 `c-function` scopes too (Pablo: "estas son
extern c"): C decoration = `_` + name (cdecl x86), each symbol checked in the lib
(14/14); note that 10 of those 14 definitions carry no `extern "C"` in the .cpp —
the C linkage comes from the header declaration — so linkage is never inferred
from the definition line, the lib is the authority. Backup
`scratchpad\Bitwise.cpp.before-mangled`.
**Correction the same day (Pablo pasted the export table of ot4xb.dll, decorated and
undecorated, 2651 entries; he will also save both lists as .txt for me):**
`mangled-name:` means the name in the **export table of the DLL**, not the import-lib
symbol. For cdecl C exports the linker strips cl's `_`, so the export name is the
plain name (`_str_rt_r_`, not `__str_rt_r_`); stdcall exports keep `_name@N`; C++ ones
are the MSVC decoration. Bitwise.cpp's 14 were corrected (`__x` -> `_x`, backup
`Bitwise.cpp.before-mangled2`), conCallEx.cpp's 80 re-verified: all 94 present in the
DLL. Tooling: `scratchpad\dllexports.ps1` (`Get-DllExportNames`, pure PE parsing of
PE32/PE32+, no toolchain) and the dump `scratchpad\ot4xb.dll.exports.txt` (2651 names:
1136 C++ `?...`, 75 stdcall `@N`, the rest plain); `concallex-doc.ps1` now verifies
against the DLL. Never take mangled names from `ot4xb_cpp.lib` again. Pablo's own dumps of
the same table: `C:\HD\F\__pbnprj\_pub\xb\_ot4xb_\decoradas.txt` (decorated) and
`...\_ot4xb_\sin-decorar.txt` (undecorated C++ prototypes, e.g.
`struct MomHandleEntry * __cdecl _conCallCon(char *,char *)`), one line per export:
`ordinal (0x..), hint (0x..)|N/A, name, 0xRVA, demangler` — both CRLF, ASCII, 2651
lines, identical to the DLL dump (checked 2026-08-21), paired 1:1 by ordinal/RVA. Use
`sin-decorar.txt` to cross-check `cpp-function:` prototypes (typedefs expanded). ClrTool.cpp
got `mangled-name:` on its 7 `c-function` scopes too (2026-08-21, "ponselos para
homogeneizar"; plain export names, 7/7 in the DLL; backup `ClrTool.cpp.before-mangled`),
so every c-function/cpp-function scope written so far carries it (14 + 7 + 80).
**conEvalEx.cpp done (2026-08-21):** 70 `OT4XB_API` C++ overloads (7 families x 10:
`_conEvalCon/Void/Bool/Long/Double/Float/Lpstr`), Pablo: "casi identicos a call ex solo que
con codeblocks en lugar de nombre de funcion" — first parameter `ContainerHandle conb`,
`_conEvalB` underneath, typed families through the same `_conRelease_ret_*` helpers of
conCallEx.cpp. Script `scratchpad\conevalex-doc.ps1` (twin of concallex-doc.ps1: creates
`conEvalEx.cpp.before` on the first run, restores it on re-runs, verifies against the DLL
export table): 70 `cpp-function` scopes, 70/70 mangled names, 210/210 markers, CRLF/ASCII.
`_conEvalCon` blocks carry `see-also: _conCallCon`; the typed ones `see-also: _conEvalCon`.
Total scopes with `mangled-name:` so far: 14 + 7 + 80 + 70 = 171.
**conMCallEx.cpp done (2026-08-21, "su primo"):** 90 `OT4XB_API` C++ overloads (`_conMCallCon` 14,
`ConN`/`ConNR` 1+1 variadic with `ULONG nParams` and `pFN` FIRST, `Void` 13, `Lpstr` 13, `Bool`/`Long`/
`Double`/`Float` 12 each). Pablo: `Self` = the object, `pFN` = method name. Special cases
documented from the bodies: `ULONG * pDw` = numeric by reference (`*pDw` in as LONG, read back
with `_conGetLong` after the call); `LONG v1, BOOL v2` goes through `_conMCallConNR` with
temporaries; `_conMCallConN` does NOT release the parameter containers, `_conMCallConNR` does
(never `Self`). Script `scratchpad\conmcallex-doc.ps1` (handles pointer types `ULONG*` -> `PAK`):
90 `cpp-function` scopes, 90/90 mangled names, 270/270 markers, CRLF/ASCII, backup
`conMCallEx.cpp.before`. FINDING: the installed DLL exports 91 `_conMCall*` — the extra
`?_conMCallVoid@@YAXPAUMomHandleEntry@@PADJJJ@Z` = `_conMCallVoid( Self, pFN, LONG, LONG, LONG )`
is declared in `ot4xb_cpp_exported.h:200` but has no definition anywhere under `source\`;
either it lives in a file not on this machine or the DLL was built from a different tree.
Told Pablo; nothing done about it. Total scopes with `mangled-name:` so far: 261.
RESOLVED minutes later: it was MY parser. The definition sits in conMCallEx.cpp (line 155
of the original) with ONE LEADING BLANK before `OT4XB_API`; my regex was anchored at column
0 and skipped it (Pablo guessed "puede que se haya ocultado tras un comentario" — no, just
the blank). Fixed the script (`^\s*OT4XB_API`), regenerated: 91 definitions, 91 scopes,
91/91 mangled names, 0 extra, 273/273 markers; the blank before `OT4XB_API` is left as is
(code untouched). Lesson for every future survey/script: never anchor `OT4XB_API`/`extern`/
`XPPRET` at column 0, and always check "definitions outside a scope = 0" after a run. Total
scopes with `mangled-name:` so far: 262. Pablo also mentioned "en common estructures vi
alguna cosa rara" (winapi_CommonStructures.cpp) — not yet explained. Explained: while
inserting doc there (his own edit) `/*` and `*/` got tangled and ate code; he fixed it
himself. Balance check run afterwards (read-only, PowerShell: count `/*` vs `*/`, running
depth never < 0, no `*/` inside a `/*{{ ... }}*/` block, no `/*` nested in one): all 7
converted files and winapi_CommonStructures.cpp (398 plain `/* */` XML blocks, 0 markers)
are balanced. Keep running that check after every conversion.

**Container.cpp + the batch (2026-08-21):** Container.cpp converted (33 `function` + 52 `c-function`; the
`<xbdoc>/<c-api>` group block of the 4 numeric helpers became 4 `c-function` skeletons with its
texts) and Pablo's rule for undocumented exports was set: **no texts written by me — a SKELETON
with a `todo:` field** ("corro el riesgo de que las pase: deja el esqueleto de la doc y le pones un
to-do dentro para localizarlas luego"), then "puedes hacer eso mismo para el resto de los cpp".
Tooling (all in the scratchpad, all byte-safe 1252/CRLF): `xbdoc2markers.ps1` (v2: kind and
`mangled-name` from the DLL export table incl. stdcall `_name@N` and namespaced `?f@ns@@`, one-line
functions, indented code/namespaces — closing brace at the header's indentation —, `//` lines
between block and header, trailing header comments), `skeletons.ps1` (skeleton per undocumented
`OT4XB_API`/`XPPRET`/`_XPP_REG_FUN_` definition; provisional desc from old `//` comments or
`<c-api>` one-liners, flagged; `begin-class` + `class-name:` for class functions; definitions
still under an unconverted XML block — class, function-family, method… — are left alone and
reported as xml-pending), `batch.ps1` (per-file `.before` backup, runs both, then HARD checks:
code identical once all doc comments are stripped, CR=LF, markers and begin/end pairs balanced,
no `*/` inside blocks, no definition outside a scope, no marker line > 140, non-ASCII count
unchanged; a file failing any check is restored; `batch-report.csv`, `batch-log.txt`,
`batch-summary.txt`). Bugs the checks caught before the batch: nested array on the first wrapped
line, namespace bodies swallowed up to the namespace's `}`, a lost blank in rebuilt one-liners
(now the original line is kept), `# Source documentation system — specification (draft 1)  Status: design agreed in conversation on 2026-08-20 (Pablo Botella / Claude). No code exists yet, by decision: this document fixes the model first. Open points are marked **[open]**. Pilot project: mysql4xb (small), then ot4xb. Everything in the repositories is English.  ## 1. Goal and principles  1. **Proximity.** The line that registers a thing documents it. When a function,    member or method changes, its documentation is on the same line or the line    before it — never in a parallel list that nobody opens. (The `<xbdoc>` XML    blocks fail this for classes: FILETIME64 has 42 `<method>` entries forty lines    away from the 42 `MethodCB` calls, and without parameters.) 2. **The sources are the only truth.** Markers in the `.cpp`, XML blocks in the    `.cpp`, external documents in the repo. Nothing is authored in a database or in    a generated file. 3. **The database is an intermediate file, like an `.obj`.** Extracting is    compiling (`.cpp` → `.db`, with diagnostics); generating is linking (`.db` →    README, HTML, skill, …). It is never versioned; it is rebuilt every time and kept    only for debugging when asked (`-keep`). 4. **Annotations are complementary, never competing.** Several annotations for    the same entity may exist in different forms and places; all are collected    with their provenance. A configured precedence resolves single-valued keys;    genuine conflicts are reported, not guessed. 5. **Markdown is not the model.** It is one possible output (through mkskill);    the model is the set of entities, annotations and relations below.  ## 2. The Xbase++ object model (what the documentation describes)  - A **class object** is a singleton: it has class vars and class methods, no   instance vars or instance methods. "Creating the class" means creating the   class object (`TXbClass::Create()` → `_conClsObj("NAME")`; e.g.   `wapist_FILETIME64()` returns it). - `:new()` is an internal class method that creates an **instance**: it sees the   class vars/methods and has its own ivars and instance methods. - **GWST classes** are specific to ot4xb (`gwst-class`). They are always direct or   indirect children of GWST. Besides creating the class object, ot4xb stores the   structure definition (members, types, offsets) and its inheritance in a class   var; `:new()` is therefore a special, non-standard initialisation built on that   layout. This is written once (GWST / model chapter) and attached to every   `gwst-class` by the generators, never repeated per class. - A GWST class may have other, non-GWST parents (multiple inheritance). A parent   acting as a provider of helper (often class) methods is common. Parents are   complementary: the same name is not expected in two branches. - **Vocabulary (three levels).** *Entities*: `function` and `class` (exported to   Xbase++) and `c-function` (an `extern "C"` export of the DLL: the `<ot4xb-c>` /   `<ot4xb-api>` blocks). Functions and classes go to the Xbase++ reference;   c-functions go to **another manual**, the C/C++ API reference; a c-function   reachable from Xbase++ through `@ot4xb:name(...)` (nFpCall) says so in its   `xbase-syntax` fragment, which is the bridge between the two manuals.   `cpp-function` is the C++-linkage export (`OT4XB_API` without `extern "C"`,   overloadable, decorated name): same manual as the c-functions, one scope per   overload with its own prototype and `mangled-name` (`conCallEx.cpp`). A fifth   entity is reserved for the C++ manual: `cpp-class` (the C++ classes of   `ot4xb_cpp_exported.h`), with its own scope `begin-cpp-class` … `end-cpp-class`;   its auxiliaries (C++ methods, members) will be defined with the first one converted. *Auxiliaries* of a   class, seven of them: `method`, `gwst-member` (structure member),   `property` (instance property), `ivar` (instance variable), `class-method`,   `class-var`, `class-property` — structure members and properties are methods   underneath, but they are separate concepts for the reader. *Fragments*: what   hangs from an entity or an auxiliary — `desc`, `type`, `return`, `param`,   `flag`, `see-also`, `since`, `deprecated`, `note`, `example`, … A fragment   always has a parent; how external documents attach fragments comes later.  ## 3. Where documentation lives (annotation forms)  ### 3.1 Inline markers `/*{{ … }}*/`  Delimiters: `/*{{` and `}}*/`. They never clash with a normal comment and are found with a trivial scan. Two families:  **Scope markers** (`begin-class`/`end-class`, `begin-function`/`end-function`, `begin-c-function`/`end-c-function`, `begin-cpp-function`/`end-cpp-function`) — always paired, never nested, each on a line of its own: the opening one above the header, the closing one right below the closing brace. In a function scope the documentation block sits **between the opening marker and the header**, outside the body (opening at column 0, fields indented 12 blanks before the `|`, continuation lines 14, the closing `}}*/` indented 3):  ``` /*{{begin-function}}*/ /*{{function: _a2dbf_( aData , aStruct , cFName )             | category: misc/dbf             | desc: Create a FOX database from the provided bidimensional array directly without use the DBE engine.               Only supported types are C, N, L, D.             | param aData: Array - Bidimensional array with the data to be stored in the DBF file.             | return: Logical - .T. if the DBF file was created successfully, .F. otherwise.    }}*/ _XPP_REG_FUN_( _A2DBF_ ) {    ... } /*{{end-function}}*/  /*{{begin-class}}*/ /*{{class-name: FILETIME64             | category: date-time/filetime             | desc: GWST wrapper over the WinAPI FILETIME structure, with helper properties and methods ...             | note: ...    }}*/ XPPRET XPPENTRY wapist_FILETIME64( XppParamList pl ) {    ...   (the auxiliaries are documented where they are declared, inside) } /*{{end-class}}*/ ```  Everything between a pair belongs to that class or function. A marker inside a scope does not need to name its owner. A marker outside any scope (or an external document) must name its owner (see §4). An unbalanced pair is an error with file and line.  **Item markers** — one per registered thing, either trailing on the registration line or in a block right before it:  ``` kind: value [intrinsic attributes] | key: value | key: value | … ```  - The first token is the **kind**, always a single token (`gwst-member`, not   `gwst member`). Its value follows; its intrinsic attributes (the ones that   belong to the kind itself) follow the value as `key: value` pairs before the   first `|`. - `|` separates the descriptive fields: `desc`, `return`, `param <name>`,   `see-also`, `since`, `deprecated`, `note`, `example`, … - A marker may span lines. A line that starts with `|` (after trimming) opens a new   field; any other line continues the current field. Leading/trailing blanks are   trimmed and the lines of a field are joined with one blank. Blank fields are   ignored. Long texts are wrapped this way, the continuation indented under the   text (the style Pablo set in `array2dbf.cpp`):  ```    /*{{function: _a2dbf_( aData , aStruct , cFName )             | desc: Create a FOX database from the provided bidimensional array directly without use the DBE engine.               Only supported types are C, N, L, D.             | param aStruct: Array - Array with the structure definition. Each element is an array with the following structure:               [ cFieldName , cFieldType , nFieldLength , nFieldDecimals ].    }}*/ ```  Kinds and their intrinsic attributes:  | kind | value | intrinsic attributes | placed on | |---|---|---|---| | `class-name` | the class name (+ `category`, `desc`, `note` … of the class) | — | the block between `begin-class` and the class function header, like a function block | | `gwst-class` | — | — (fields: `gwst-parent`, `parent`) | the `pc->GwstParent()` line | | `gwst-member` | member name | `type:`, `pos:` | the `pc->Member_*("…")` line | | `property` | property name | `type:` | the `pc->PropertyCB("…", …)` line | | `method` | signature: `Name( p1 , [p2] , [@p3] )` | — | the `pc->MethodCB("…", …)` line | | `ivar` | variable name | `type:` | the `pc->Var("…")` line | | `class-method` | signature | — | the `pc->ClassMethod*("…", …)` line | | `class-var` | variable name | `type:` | the corresponding registration | | `class-property` | property name | `type:` | the `pc->ClassProperty*("…", …)` line | | `function` | signature | — | inside `begin-function` … `end-function`, or a free marker naming it | | `c-function` | C signature (`void _str_rt_r_( LPBYTE p, DWORD cb, BYTE r )`) | — | its own scope markers `begin-c-function` … `end-c-function`, the block before the `extern "C" OT4XB_API …` header; extra fragments `header:` (declaring .h), `prototype:` (when it differs), `xbase-syntax:` (`@ot4xb:_str_rt_r_( @buffer, len(buffer), nBitsToRotate )`), `mangled-name:` (the name in the export table of the DLL: `_str_rt_r_`; a cdecl export carries no decoration, a stdcall one keeps `_name@N`) | | `cpp-function` | C++ prototype (`ContainerHandle _conCallCon( LPSTR pFN, LONG val )`) | — | a C++-linkage export (`OT4XB_API` without `extern "C"`): its own scope markers `begin-cpp-function` … `end-cpp-function`, the block before the header; an overloaded function gets one scope per overload, each with its own prototype; extra fragments `header:`, `prototype:` (when it differs), `mangled-name:` (the decorated name as exported, `?_conCallCon@@YAPAUMomHandleEntry@@PADJ@Z`) | | `param` | parameter name | `type:` | inside a function scope, next to the code that reads the parameter | | `return` | type | — | inside a function scope, next to the code that returns |  **Fragment markers** — a fragment may also stand alone, as its own marker, next to the code that motivates it:  ``` /*{{note: The flag 0x01 may also be used for …}}*/ /*{{flag 0x01: cTimeString may be a FILETIME64-compatible object to copy from}}*/ /*{{param nFlags: Numeric - 0x01 allows object copy; 0x10 allows NIL as now UTC}}*/ /*{{return: Self}}*/ ```  Its owner follows from proximity: inside `begin-function` … `end-function` it belongs to the function; inside `begin-class` … `end-class` it belongs to the **last auxiliary declared before it** (the most recent `method:`, `property:`, …), or to the class itself before the first auxiliary; outside any scope it is an error (a fragment has no owner of its own). So a `flag` can sit on the `if (flags & 0x01)` that handles it and a `note` next to the `MethodCB` that deserves it.  Signature conventions: `[x]` optional, `@x` by reference, descriptive names (they need not match the code block's variable names; the **count** must).  Fields:  | key | meaning | single or accumulative | |---|---|---| | `desc` | description | single | | `category` | grouping key (`date-time/filetime`), on classes and functions. Each manual has its own taxonomy: the categories of c-functions and cpp-functions belong to the C/C++ manual, independent of the Xbase++ ones even when a name coincides (`bitwise`) | single | | `header`, `prototype`, `xbase-syntax`, `mangled-name` | c-functions and cpp-functions: declaring header file; C/C++ prototype when it differs from the signature; `xbase-syntax` (c-functions) the call from Xbase++; `mangled-name` the name as it appears in the export table of the DLL (`ot4xb.dll`, never the import library, whose strings are linker symbols): for a cpp-function the MSVC C++ decoration (`?_conCallCon@@YAPAUMomHandleEntry@@PAD@Z`), one per overload since every overload is its own scope; for a c-function the exported name (`_str_rt_r_`: a cdecl export carries no decoration, a stdcall one keeps `_name@N`); grouping the overloads of a family is a later step | single | | `return` | return type (`Self`, `NIL`, `Character`, …); a trailing `- text` may follow | single | | `param <name>` | `Type - text` for that parameter | single per name | | `gwst-parent` | the GWST-branch parent whose layout is inherited (absent = GWST itself; GWST is never written) | single | | `parent` | the other, non-GWST parents, comma separated | single (list) | | `flag <value>` | what one flag value does (`flag 0x10: NIL stores the current UTC time`); qualify with the parameter name when several parameters take flags (`flag nFlags 0x10: …`); named constants are fine | accumulative | | `calls` | in a method: the Xbase++ function the code block calls (`calls: ft64_SetTs`). The real description, parameters and flags are written once, in that function's own scope; the method keeps its signature (same parameter names as the function, minus the Self one), its `return` and whatever complements it. A cross-reference between sections, not a copy | single | | `see-also` | related entities | accumulative | | `since`, `deprecated` | version annotations; the changelog is derived from them | single | | `note`, `example` | free text / code | accumulative | | `todo` | a documentation **skeleton**: every undocumented export gets its scope and block with what can be derived mechanically (signature, `header`, `mangled-name`, `param name: type - TODO`, `return: type - TODO`) plus one `todo:` line listing what is still missing (desc, category, param texts, …); a `desc` copied from an old informal `//` comment or from a group block is flagged there as provisional. `grep "| todo:"` locates them; the generator must treat a block with `todo` as unfinished (decided by Pablo on 2026-08-21: "deja el esqueleto de la doc y le pones un to-do dentro para localizarlas luego") | single | | any other key | kept as written (key, value, provenance) and rendered generically until it gets a format of its own — the set of fragments is open | — |  Text rules:  - `|` separates fields, always. Code goes between backticks, as in markdown:   inline code between single backticks (an Xbase++ code block such as   ``{|s| ft64_Now(s),s}``, an expression such as ``((0 | x1) | x2)``, an operator such as   ``|``), multi-line code between triple-backtick fences (```), typically in an   `example`. Inside backticks everything is literal — a `|` there is text — and the   generator renders the span as code. Type alternatives are written with `/`:   `Date/Character/FILETIME64 object`. - Lines are at most 120 characters (140 tolerated). An item marker stays on the   registration line only when the whole line fits; otherwise it becomes a block   right before the registration (`/*{{kind: value` at the code indentation, fields   indented 9 more blanks before the `|`, closing `}}*/` at the code indentation).   Long fields wrap into continuation lines (§ above). - Every scope is framed by the `// ----` separator lines of the file: separator,   `begin-…`, documentation block, header … closing brace, `end-…`, separator.   Old informal comments that the markers supersede are removed when converting. - `}}` never appears inside a marker (it closes it). `*/` never appears inside a   marker (it closes the C++ comment; the compiler, not the extractor, complains). - Kinds, keys and entity names are matched case-insensitively (Xbase++ names are   case-insensitive); the written case is kept for display.  Example, the state of `FileTime.cpp` after 2026-08-20 (FILETIME64 fully marked; trailing one-liners without parameters, preceding blocks with parameters):  ``` /*{{begin-class}}*/ /*{{class-name: FILETIME64             | category: date-time/filetime    }}*/ XPPRET XPPENTRY wapist_FILETIME64( XppParamList pl )       pc->ClassName("FILETIME64");       pc->GwstParent(); /*{{ gwst-class  }}*/       pc->Member_DWord( "dwLowDateTime" ); /*{{ gwst-member:dwLowDateTime type: DWORD pos: 0 | desc: The low-order part of the file time. }}*/       pc->Member_DWord64("qft"); /*{{ gwst-member:qft  type: uint64 pos: 0 | desc: int64 representing the same file time, ... }}*/       /*{{method: SetTimeStamp( cTimeString , [@nGetShiftInMinutes] , [nFlags])                | return: Self                | desc: Stores a timestamp string into a FILETIME64 value.                | param nFlags : 0x01 allows cTimeString == object copy; 0x10 allows cTimeString == NIL as now UTC; 0x30 allows cTimeString == NIL as now local.       }}*/       pc->MethodCB("SetTimeStamp","{|s,c,sh,flags| ft64_SetTs(s,c,@sh,flags),s}");       pc->MethodCB("GetTimeStamp","{|s| ft64_GetTs(s)}"); /*{{ method: GetTimeStamp() | return: Character | desc: Returns the value as YYYY-MM-DD hh:mm:ss (ft64_GetTs with the default format). }}*/       pc->PropertyCB("dDate", ...); /*{{ property: dDate type: Date | desc: Read/write. The date component as an Xbase++ date (GetDateTime); assigning stores the date (SetDateTime with no time). }}*/ } /*{{end-class}}*/ ```  ### 3.2 XML blocks `<xbdoc>`  The existing form: a `/* … <xbdoc> … </xbdoc> … */` block before a function or a class-creating function, with two schemas (`<function>`: name, category, description, syntax, parameters/parameter{name,type,description}, return{type, description} or text, remarks, see-also; `<class>`: name, parent, source, category, description, members/member@type@name@offset@size, properties/ property@name@type, methods/method@name@returns, remarks). They stay valid as a form. Parsing is strict XML: a malformed block is reported as an issue with file and line and skipped as a whole (the rest of the file goes on). Free text with `<` or `&` must be escaped or wrapped in `<![CDATA[ … ]]>`. On 2026-08-20 all 26 blocks of `FileTime.cpp` were well-formed.  ### 3.3 External documents (with front matter) **[open: details later]**  A file in the repository whose front matter names the owner and the key, and whose body is the annotation. Read with fmlines (front matter only, body untouched). The body may be prose or **code**: a `.prg` example (`owner: FILETIME64:AddDays`, `key: example`) is an `example` annotation, and when it is a complete program it doubles as a test — documentation that compiles and runs does not rot silently.  Front matter of an external `.md` (same mould as `_mkskill/src`: YAML with the namespaced key `xbdoc:`):  ```markdown --- xbdoc:   class: FILETIME64             # the entity: class: or function:   method: AddDays               # optional: the auxiliary, by its kind (method:, property:, ivar:, gwst-member:, class-method:, class-var:, class-property:) --- body = the note ```  `class:` or `function:` is mandatory (`class:` alone = the class itself; with an auxiliary key = that auxiliary); `source` is the file name, set by the extractor. **An external `.md` is always an extra note**: an accumulative `note` fragment added to whatever the markers say; it never replaces a `desc`, `return` or `param`, so there is no `fragment:` key and no precedence question. It attaches to a class, to one of its auxiliaries or to a function, never deeper (no notes on a parameter or a flag). A `.prg` example cannot start with `---`: its front matter goes in a sibling `.fm` of the same name (fmlines `extern`), the body is the `example`.  ## 4. Identity  Every annotation attaches to an entity by key:  - `project` — the project itself (overview chapters). - `class: Name` — a class. - `class: Name` + `<auxiliary-kind>: name` — one of its seven auxiliaries. - `function: name` — a free function.  Inside a scope the owner is implicit.  References written by the programmer (`calls`, `see-also`, `parent`, `gwst-parent`) use these same keys. Nothing is inferred: the generator renders the links that are written, in both directions only when both are written, and it resolves each name itself — what kind of thing it is, which section it lives in, how to link it; a name that exists neither in the project nor in the list of external names is a diagnostic. Comparison is case-insensitive. An annotation whose owner cannot be resolved in any loaded project is an issue.  ## 5. The tool reads only the comments  The extractor reads the markers (and the XML blocks, and the external documents) and nothing else: the C++ code between markers is opaque text. It does not parse `MethodCB` / `PropertyCB` / `Member_*` calls, code blocks or the `.xbmac`. Hence:  - A marker carries everything that must appear: public name, signature, return,   parameters with type and text, `type`/`pos` of structure members. - There is no `registration` data and no cross-check against the code (nothing   is derived, nothing is compared). Proximity is what keeps the documentation in   step with the code, not a mechanical check. - Writing conventions for class registrations (by the author, not the tool): the   `s` of a code block is Self and is not documented; a method whose block ends in   `,s` is `return: Self`, one ending in `,NIL` is `return: NIL`; literal arguments   (`.T.`, a fixed format, a multiplier) are explained in `desc`; a method that   wraps a documented function repeats the relevant parameter texts (the tool will   not fetch them).  Found while annotating FILETIME64: the class XML says `ToLocalTime` returns Self, but `{|s| FT64_TOLOCALTIME(s)}` has no `,s` and returns NIL. The marker documents NIL; the code decides — and only a human reading it notices.  ## 6. The intermediate model (and its container)  The model below — entities, annotations with provenance, relations, resolution rules, checks — is what matters; the container is an implementation detail. It could be plain Go structures in memory, an XML dump (cargoxml: readable, tolerant to foreign data) or a database. The suggested container is SQLite through `modernc.org/sqlite` (pure Go, no cgo; speed is not a concern), because inheritance and the checks become queries and a kept file can be interrogated. **One database per project.** A reference to a class of another project (a project built on ot4xb deriving from one of its classes, e.g. a `gwst-class` with `gwst-parent: RECT`) is an **external** reference: it is stored by name, rendered as a name (or as a link to the other project's documentation) and never resolved locally. Optionally one database per source file, attached at generation time (`ATTACH`) like objects linked together. **Not even persistent by default**: it is built in memory (`:memory:`) on every run and dies with it; `-keep` writes it to disk for debugging. Incremental extraction by `sha256` only makes sense when a kept database is reused.  Tables (essential columns):  | table | columns | |---|---| | `meta` | key, value — project name, version, repo, extraction data (git commit, time, counters) | | `unit` | id, kind (`class`/`function`), name, source, category, is_gwst | | `member` | id, unit_id, kind (`method`/`gwst-member`/`property`/`ivar`/`class-method`/`class-var`/`class-property`), name, type, pos, size, access, ordinal | | `param` | owner_kind, owner_id, ordinal, name, type, optional, byref | | `annotation` | owner_kind, owner_id, key, value, form (`marker`/`xbdoc`/`external`), source, ordinal | | `relation` | from_unit, to_name, kind (`parent`/`gwst-parent`/`see-also`/`calls`), ordinal | | `tag` | owner_kind, owner_id, key, value — keys the grammar does not know yet | | `issue` | severity, code, source, line, message — documentation diagnostics only | | `doc_fts` | FTS5 over descriptions, remarks, chapters — lookup for people and agents |  Descriptions, return texts and parameter texts are **not** columns of `unit`, `member` or `param`: they are rows of `annotation`, so that several annotations of different forms can coexist for the same entity with their provenance. Provenance is `source`: the **file name**, detected by the extractor and attached to every entity, auxiliary and fragment (never written by hand); line numbers are not part of the model — they appear only in diagnostics.  Resolution is a **view**, not stored data:  - Single-valued keys (`desc`, `return`, `param x`, `since`, …) come only from   markers and `<xbdoc>` blocks (external documents only add notes and examples).   When both forms document the same key, the marker wins over the XML block;   within one form the closest to the code wins. Two different values at the same   level → issue. - Accumulative keys (`example`, `see-also`, `note`): all values, in precedence   order.  ## 7. Inheritance resolution  A recursive CTE over `relation` (`parent`, `gwst-parent`) yields every ancestor with depth and path (cycle guard by depth and by path). Multiple inheritance may be parallel: several branches can reach the same ancestor (the usual diamond, every branch ending in GWST). Ancestors are therefore deduplicated by entity (`DISTINCT` on the ancestor; every `path` is kept for display). The effective members of a class are the **union** of its own members and those of every ancestor, each row carrying `inherited_from` and `depth`; outputs can show "inherited from GWST". Parents are complementary: a member reached through two paths from the **same origin** is one member; the same name coming from two **different origins**, or a parent repeating a name the child defines, is not resolved silently — it is an issue.  GWST inheritance is special and positional: there is exactly **one** `gwst-parent` per class, and the structure members are inherited in successive positions — the chain GWST → … → `gwst-parent` lays its members first and the class appends its own `gwst-member` rows after them. The other (parallel) parents contribute methods and variables, never layout. The offset of an own member is therefore the accumulated size of the whole GWST chain plus what the class itself accumulated before it, unless an explicit `GwstSetOffset` rewinds it (`qft` at `pos: 0` in FILETIME64); that is what the check recomputes and compares with the documented `pos:`. Cycles are issues. A parent that does not exist in the database is an **external** parent (a class of another project, e.g. an ot4xb class used by a project built on it): its members are not listed, the class is shown as inheriting from it by name, and `check` only complains when the name is not in the configured list of known external names (so typos are still caught).  ## 8. Consistency checks (the `check` leg, no output written)  Only the documentation itself is checked — there is no code to compare with:  - Malformed marker (unknown kind, missing value, unbalanced or nested scopes,   forbidden `}}` / `*/` / `|` in text) or malformed XML block. - Duplicate names inside a class, duplicate single-valued annotation at equal   precedence. - `parent` / `gwst-parent` / `see-also` targets that exist neither in the project   nor in the configured list of external names. - A method signature whose `param` lines name parameters the signature does not   have (or the other way round). - Documented `pos` of structure members not matching the documented sizes along   the GWST chain (optional; from the documentation only).  Every issue carries file and line. A non-zero exit code makes it usable in CI.  ## 9. Outputs (the "link" products)  - Markdown sections into `_mkskill/src/` → README / AGENTS / SKILL through mkskill. - An HTML reference (one page per class / function, inherited members listed). - Changelog derived from `since` / `deprecated` annotations in the sources (not   from comparing databases). - Agent lookup: a query command over the database (`find FILETIME64:AddDays`),   so a future Xbase++ skill does not need a monolithic text. - Examples with front matter compiled/run as tests.  ## 10. Open decisions  1. ~~`class name:` → `class-name:`~~ — settled: `class-name:`. 2. ~~Confirm `property:` and the spelling of the class auxiliaries~~ — settled:    the seven auxiliaries are `method`, `gwst-member`, `property`, `ivar`,    `class-method`, `class-var`, `class-property`. 3.   `/` for type alternatives; `|` in text   — settled: `/` for alternatives; code    between backticks (single inline, triple for blocks), where `|` is text. 4. ~~Precedence order of forms~~ — settled: marker over XML block; externals only add. 5. ~~Front matter schema of external documents~~ — settled: `xbdoc:` with    `class:`/`function:` (+ auxiliary kind) and `fragment` (§3.3); multi-fragment    files, if ever, later. 6. Whether the `<xbdoc>` blocks are kept as a living form or retired once the    markers cover their content. **Done for `FileTime.cpp` on 2026-08-20**: its 26    blocks were migrated to markers (25 `begin-function` scopes with the doc block    inside, the class notes on `class-name:`) and removed; the other source files    keep their XML until converted. 7. Where the extractor/generator lives (a subcommand of ot4xb-tool, or its own    tool) — out of scope until the model is closed.  ## Appendix — current state (2026-08-21)  - `_ot4xb_\source\conMCallEx.cpp`: the 91 `OT4XB_API` C++ overloads of the `_conMCall*`   family (method calls: `Self` = the object, `pFN` = method name; `_conMCallCon` 14,   `_conMCallConN`/`_conMCallConNR` variadic with `nParams`, the typed `Void/Bool/Long/Double/`   `Float/Lpstr`, `Void` 14), same layout as conCallEx.cpp: one `cpp-function` scope per overload,   `mangled-name:` 91/91 in the export table (one definition is indented by a blank before   `OT4XB_API`; scope markers must tolerate leading blanks). - `_ot4xb_\source\conEvalEx.cpp`: the 70 `OT4XB_API` C++ overloads of the `_conEval*`   family (`_conEvalCon`, `_conEvalVoid`, `_conEvalBool`, `_conEvalLong`, `_conEvalDouble`,   `_conEvalFloat`, `_conEvalLpstr`: the `_conCall*` twins that evaluate a code block `conb`   instead of calling a function by name) documented exactly like conCallEx.cpp: one   `cpp-function` scope per overload, `category: ot4xb-api`, `header`, `mangled-name:` (70/70   in the export table of `ot4xb.dll`), `desc`, `param`, `return`, `see-also`. - `_ot4xb_\source\conCallEx.cpp`: the 80 `OT4XB_API` C++ overloads of the `_conCall*`   family (`_conCallConR`, `_conCallCon`, `_conCallVoid`, `_conCallBool`, `_conCallLong`,   `_conCallDouble`, `_conCallFloat`, `_conCallLpstr`) documented as `cpp-function`, one   `begin-cpp-function` … `end-cpp-function` scope per overload with its own prototype,   `category: ot4xb-api`, `header: ot4xb_cpp_exported.h`, `mangled-name:` (every one of   the 80 decorated names checked against the export table of `ot4xb.dll`), `desc`, `param`, `return`,   `see-also`. The non-exported `_conRelease_ret_*` helpers stay undocumented. Grouping   the overloads of a family is deferred. - `_ot4xb_\source\ClrTool.cpp`: 7 `extern "C"` colour functions documented from scratch as   `c-function` (category `winapi/color`, `mangled-name:` = the plain export name, checked   against the export table of `ot4xb.dll`); statics left undocumented. - `_ot4xb_\source\Bitwise.cpp`: fully converted — 15 `function` + 14 `c-function`   scopes (the `<ot4xb-c>` blocks), operators in prose between backticks; `mangled-name:` added to the 14 c-functions on 2026-08-21 (export names, checked against the export table of `ot4xb.dll`). - `_ot4xb_\source\array2dbf.cpp`: converted (1 function; Pablo set the definitive   layout on it). - `_ot4xb_\source\FileTime.cpp`: fully converted. 25 functions with   `begin-function` … `end-function` and a doc block inside (`function:` signature,   `category`, `desc`, `param`, `flag`, `return`, `note`, `see-also`); FILETIME64 with   `begin-class` … `end-class`, `class-name:` block (category, desc, notes),   `gwst-class`, 3 `gwst-member`, 8 `property`, 42 `method` markers reduced to   signature + `return` + `calls:` + one-line `desc` (+ `param` where the method   parameter differs from the function one); 132 markers, no `<xbdoc>` left, no   informal trailing comments on the `_XPP_REG_FUN_` lines. Type alternatives   written with `/`. Backups: `scratchpad\FileTime.cpp.before-markers` (original),   `FileTime.cpp.before-functions` (after the first marker pass). - mysql4xb: no documentation yet (README stub, no XML blocks); its four classes   (`MYSQL4XB`, `MYSQL4XB_RESULTSET_T`, `MYSQL4XB_RESULT_ROWS_T`,   `MYSQL4XB_RESULT_FIELD_T`) are plain `TXbClass` runtime classes — no GWST, no   declared parent — built with ot4xb's class machinery; the pilot for this system. `/`# Source documentation system — specification (draft 1)  Status: design agreed in conversation on 2026-08-20 (Pablo Botella / Claude). No code exists yet, by decision: this document fixes the model first. Open points are marked **[open]**. Pilot project: mysql4xb (small), then ot4xb. Everything in the repositories is English.  ## 1. Goal and principles  1. **Proximity.** The line that registers a thing documents it. When a function,    member or method changes, its documentation is on the same line or the line    before it — never in a parallel list that nobody opens. (The `<xbdoc>` XML    blocks fail this for classes: FILETIME64 has 42 `<method>` entries forty lines    away from the 42 `MethodCB` calls, and without parameters.) 2. **The sources are the only truth.** Markers in the `.cpp`, XML blocks in the    `.cpp`, external documents in the repo. Nothing is authored in a database or in    a generated file. 3. **The database is an intermediate file, like an `.obj`.** Extracting is    compiling (`.cpp` → `.db`, with diagnostics); generating is linking (`.db` →    README, HTML, skill, …). It is never versioned; it is rebuilt every time and kept    only for debugging when asked (`-keep`). 4. **Annotations are complementary, never competing.** Several annotations for    the same entity may exist in different forms and places; all are collected    with their provenance. A configured precedence resolves single-valued keys;    genuine conflicts are reported, not guessed. 5. **Markdown is not the model.** It is one possible output (through mkskill);    the model is the set of entities, annotations and relations below.  ## 2. The Xbase++ object model (what the documentation describes)  - A **class object** is a singleton: it has class vars and class methods, no   instance vars or instance methods. "Creating the class" means creating the   class object (`TXbClass::Create()` → `_conClsObj("NAME")`; e.g.   `wapist_FILETIME64()` returns it). - `:new()` is an internal class method that creates an **instance**: it sees the   class vars/methods and has its own ivars and instance methods. - **GWST classes** are specific to ot4xb (`gwst-class`). They are always direct or   indirect children of GWST. Besides creating the class object, ot4xb stores the   structure definition (members, types, offsets) and its inheritance in a class   var; `:new()` is therefore a special, non-standard initialisation built on that   layout. This is written once (GWST / model chapter) and attached to every   `gwst-class` by the generators, never repeated per class. - A GWST class may have other, non-GWST parents (multiple inheritance). A parent   acting as a provider of helper (often class) methods is common. Parents are   complementary: the same name is not expected in two branches. - **Vocabulary (three levels).** *Entities*: `function` and `class` (exported to   Xbase++) and `c-function` (an `extern "C"` export of the DLL: the `<ot4xb-c>` /   `<ot4xb-api>` blocks). Functions and classes go to the Xbase++ reference;   c-functions go to **another manual**, the C/C++ API reference; a c-function   reachable from Xbase++ through `@ot4xb:name(...)` (nFpCall) says so in its   `xbase-syntax` fragment, which is the bridge between the two manuals.   `cpp-function` is the C++-linkage export (`OT4XB_API` without `extern "C"`,   overloadable, decorated name): same manual as the c-functions, one scope per   overload with its own prototype and `mangled-name` (`conCallEx.cpp`). A fifth   entity is reserved for the C++ manual: `cpp-class` (the C++ classes of   `ot4xb_cpp_exported.h`), with its own scope `begin-cpp-class` … `end-cpp-class`;   its auxiliaries (C++ methods, members) will be defined with the first one converted. *Auxiliaries* of a   class, seven of them: `method`, `gwst-member` (structure member),   `property` (instance property), `ivar` (instance variable), `class-method`,   `class-var`, `class-property` — structure members and properties are methods   underneath, but they are separate concepts for the reader. *Fragments*: what   hangs from an entity or an auxiliary — `desc`, `type`, `return`, `param`,   `flag`, `see-also`, `since`, `deprecated`, `note`, `example`, … A fragment   always has a parent; how external documents attach fragments comes later.  ## 3. Where documentation lives (annotation forms)  ### 3.1 Inline markers `/*{{ … }}*/`  Delimiters: `/*{{` and `}}*/`. They never clash with a normal comment and are found with a trivial scan. Two families:  **Scope markers** (`begin-class`/`end-class`, `begin-function`/`end-function`, `begin-c-function`/`end-c-function`, `begin-cpp-function`/`end-cpp-function`) — always paired, never nested, each on a line of its own: the opening one above the header, the closing one right below the closing brace. In a function scope the documentation block sits **between the opening marker and the header**, outside the body (opening at column 0, fields indented 12 blanks before the `|`, continuation lines 14, the closing `}}*/` indented 3):  ``` /*{{begin-function}}*/ /*{{function: _a2dbf_( aData , aStruct , cFName )             | category: misc/dbf             | desc: Create a FOX database from the provided bidimensional array directly without use the DBE engine.               Only supported types are C, N, L, D.             | param aData: Array - Bidimensional array with the data to be stored in the DBF file.             | return: Logical - .T. if the DBF file was created successfully, .F. otherwise.    }}*/ _XPP_REG_FUN_( _A2DBF_ ) {    ... } /*{{end-function}}*/  /*{{begin-class}}*/ /*{{class-name: FILETIME64             | category: date-time/filetime             | desc: GWST wrapper over the WinAPI FILETIME structure, with helper properties and methods ...             | note: ...    }}*/ XPPRET XPPENTRY wapist_FILETIME64( XppParamList pl ) {    ...   (the auxiliaries are documented where they are declared, inside) } /*{{end-class}}*/ ```  Everything between a pair belongs to that class or function. A marker inside a scope does not need to name its owner. A marker outside any scope (or an external document) must name its owner (see §4). An unbalanced pair is an error with file and line.  **Item markers** — one per registered thing, either trailing on the registration line or in a block right before it:  ``` kind: value [intrinsic attributes] | key: value | key: value | … ```  - The first token is the **kind**, always a single token (`gwst-member`, not   `gwst member`). Its value follows; its intrinsic attributes (the ones that   belong to the kind itself) follow the value as `key: value` pairs before the   first `|`. - `|` separates the descriptive fields: `desc`, `return`, `param <name>`,   `see-also`, `since`, `deprecated`, `note`, `example`, … - A marker may span lines. A line that starts with `|` (after trimming) opens a new   field; any other line continues the current field. Leading/trailing blanks are   trimmed and the lines of a field are joined with one blank. Blank fields are   ignored. Long texts are wrapped this way, the continuation indented under the   text (the style Pablo set in `array2dbf.cpp`):  ```    /*{{function: _a2dbf_( aData , aStruct , cFName )             | desc: Create a FOX database from the provided bidimensional array directly without use the DBE engine.               Only supported types are C, N, L, D.             | param aStruct: Array - Array with the structure definition. Each element is an array with the following structure:               [ cFieldName , cFieldType , nFieldLength , nFieldDecimals ].    }}*/ ```  Kinds and their intrinsic attributes:  | kind | value | intrinsic attributes | placed on | |---|---|---|---| | `class-name` | the class name (+ `category`, `desc`, `note` … of the class) | — | the block between `begin-class` and the class function header, like a function block | | `gwst-class` | — | — (fields: `gwst-parent`, `parent`) | the `pc->GwstParent()` line | | `gwst-member` | member name | `type:`, `pos:` | the `pc->Member_*("…")` line | | `property` | property name | `type:` | the `pc->PropertyCB("…", …)` line | | `method` | signature: `Name( p1 , [p2] , [@p3] )` | — | the `pc->MethodCB("…", …)` line | | `ivar` | variable name | `type:` | the `pc->Var("…")` line | | `class-method` | signature | — | the `pc->ClassMethod*("…", …)` line | | `class-var` | variable name | `type:` | the corresponding registration | | `class-property` | property name | `type:` | the `pc->ClassProperty*("…", …)` line | | `function` | signature | — | inside `begin-function` … `end-function`, or a free marker naming it | | `c-function` | C signature (`void _str_rt_r_( LPBYTE p, DWORD cb, BYTE r )`) | — | its own scope markers `begin-c-function` … `end-c-function`, the block before the `extern "C" OT4XB_API …` header; extra fragments `header:` (declaring .h), `prototype:` (when it differs), `xbase-syntax:` (`@ot4xb:_str_rt_r_( @buffer, len(buffer), nBitsToRotate )`), `mangled-name:` (the name in the export table of the DLL: `_str_rt_r_`; a cdecl export carries no decoration, a stdcall one keeps `_name@N`) | | `cpp-function` | C++ prototype (`ContainerHandle _conCallCon( LPSTR pFN, LONG val )`) | — | a C++-linkage export (`OT4XB_API` without `extern "C"`): its own scope markers `begin-cpp-function` … `end-cpp-function`, the block before the header; an overloaded function gets one scope per overload, each with its own prototype; extra fragments `header:`, `prototype:` (when it differs), `mangled-name:` (the decorated name as exported, `?_conCallCon@@YAPAUMomHandleEntry@@PADJ@Z`) | | `param` | parameter name | `type:` | inside a function scope, next to the code that reads the parameter | | `return` | type | — | inside a function scope, next to the code that returns |  **Fragment markers** — a fragment may also stand alone, as its own marker, next to the code that motivates it:  ``` /*{{note: The flag 0x01 may also be used for …}}*/ /*{{flag 0x01: cTimeString may be a FILETIME64-compatible object to copy from}}*/ /*{{param nFlags: Numeric - 0x01 allows object copy; 0x10 allows NIL as now UTC}}*/ /*{{return: Self}}*/ ```  Its owner follows from proximity: inside `begin-function` … `end-function` it belongs to the function; inside `begin-class` … `end-class` it belongs to the **last auxiliary declared before it** (the most recent `method:`, `property:`, …), or to the class itself before the first auxiliary; outside any scope it is an error (a fragment has no owner of its own). So a `flag` can sit on the `if (flags & 0x01)` that handles it and a `note` next to the `MethodCB` that deserves it.  Signature conventions: `[x]` optional, `@x` by reference, descriptive names (they need not match the code block's variable names; the **count** must).  Fields:  | key | meaning | single or accumulative | |---|---|---| | `desc` | description | single | | `category` | grouping key (`date-time/filetime`), on classes and functions. Each manual has its own taxonomy: the categories of c-functions and cpp-functions belong to the C/C++ manual, independent of the Xbase++ ones even when a name coincides (`bitwise`) | single | | `header`, `prototype`, `xbase-syntax`, `mangled-name` | c-functions and cpp-functions: declaring header file; C/C++ prototype when it differs from the signature; `xbase-syntax` (c-functions) the call from Xbase++; `mangled-name` the name as it appears in the export table of the DLL (`ot4xb.dll`, never the import library, whose strings are linker symbols): for a cpp-function the MSVC C++ decoration (`?_conCallCon@@YAPAUMomHandleEntry@@PAD@Z`), one per overload since every overload is its own scope; for a c-function the exported name (`_str_rt_r_`: a cdecl export carries no decoration, a stdcall one keeps `_name@N`); grouping the overloads of a family is a later step | single | | `return` | return type (`Self`, `NIL`, `Character`, …); a trailing `- text` may follow | single | | `param <name>` | `Type - text` for that parameter | single per name | | `gwst-parent` | the GWST-branch parent whose layout is inherited (absent = GWST itself; GWST is never written) | single | | `parent` | the other, non-GWST parents, comma separated | single (list) | | `flag <value>` | what one flag value does (`flag 0x10: NIL stores the current UTC time`); qualify with the parameter name when several parameters take flags (`flag nFlags 0x10: …`); named constants are fine | accumulative | | `calls` | in a method: the Xbase++ function the code block calls (`calls: ft64_SetTs`). The real description, parameters and flags are written once, in that function's own scope; the method keeps its signature (same parameter names as the function, minus the Self one), its `return` and whatever complements it. A cross-reference between sections, not a copy | single | | `see-also` | related entities | accumulative | | `since`, `deprecated` | version annotations; the changelog is derived from them | single | | `note`, `example` | free text / code | accumulative | | `todo` | a documentation **skeleton**: every undocumented export gets its scope and block with what can be derived mechanically (signature, `header`, `mangled-name`, `param name: type - TODO`, `return: type - TODO`) plus one `todo:` line listing what is still missing (desc, category, param texts, …); a `desc` copied from an old informal `//` comment or from a group block is flagged there as provisional. `grep "| todo:"` locates them; the generator must treat a block with `todo` as unfinished (decided by Pablo on 2026-08-21: "deja el esqueleto de la doc y le pones un to-do dentro para localizarlas luego") | single | | any other key | kept as written (key, value, provenance) and rendered generically until it gets a format of its own — the set of fragments is open | — |  Text rules:  - `|` separates fields, always. Code goes between backticks, as in markdown:   inline code between single backticks (an Xbase++ code block such as   ``{|s| ft64_Now(s),s}``, an expression such as ``((0 | x1) | x2)``, an operator such as   ``|``), multi-line code between triple-backtick fences (```), typically in an   `example`. Inside backticks everything is literal — a `|` there is text — and the   generator renders the span as code. Type alternatives are written with `/`:   `Date/Character/FILETIME64 object`. - Lines are at most 120 characters (140 tolerated). An item marker stays on the   registration line only when the whole line fits; otherwise it becomes a block   right before the registration (`/*{{kind: value` at the code indentation, fields   indented 9 more blanks before the `|`, closing `}}*/` at the code indentation).   Long fields wrap into continuation lines (§ above). - Every scope is framed by the `// ----` separator lines of the file: separator,   `begin-…`, documentation block, header … closing brace, `end-…`, separator.   Old informal comments that the markers supersede are removed when converting. - `}}` never appears inside a marker (it closes it). `*/` never appears inside a   marker (it closes the C++ comment; the compiler, not the extractor, complains). - Kinds, keys and entity names are matched case-insensitively (Xbase++ names are   case-insensitive); the written case is kept for display.  Example, the state of `FileTime.cpp` after 2026-08-20 (FILETIME64 fully marked; trailing one-liners without parameters, preceding blocks with parameters):  ``` /*{{begin-class}}*/ /*{{class-name: FILETIME64             | category: date-time/filetime    }}*/ XPPRET XPPENTRY wapist_FILETIME64( XppParamList pl )       pc->ClassName("FILETIME64");       pc->GwstParent(); /*{{ gwst-class  }}*/       pc->Member_DWord( "dwLowDateTime" ); /*{{ gwst-member:dwLowDateTime type: DWORD pos: 0 | desc: The low-order part of the file time. }}*/       pc->Member_DWord64("qft"); /*{{ gwst-member:qft  type: uint64 pos: 0 | desc: int64 representing the same file time, ... }}*/       /*{{method: SetTimeStamp( cTimeString , [@nGetShiftInMinutes] , [nFlags])                | return: Self                | desc: Stores a timestamp string into a FILETIME64 value.                | param nFlags : 0x01 allows cTimeString == object copy; 0x10 allows cTimeString == NIL as now UTC; 0x30 allows cTimeString == NIL as now local.       }}*/       pc->MethodCB("SetTimeStamp","{|s,c,sh,flags| ft64_SetTs(s,c,@sh,flags),s}");       pc->MethodCB("GetTimeStamp","{|s| ft64_GetTs(s)}"); /*{{ method: GetTimeStamp() | return: Character | desc: Returns the value as YYYY-MM-DD hh:mm:ss (ft64_GetTs with the default format). }}*/       pc->PropertyCB("dDate", ...); /*{{ property: dDate type: Date | desc: Read/write. The date component as an Xbase++ date (GetDateTime); assigning stores the date (SetDateTime with no time). }}*/ } /*{{end-class}}*/ ```  ### 3.2 XML blocks `<xbdoc>`  The existing form: a `/* … <xbdoc> … </xbdoc> … */` block before a function or a class-creating function, with two schemas (`<function>`: name, category, description, syntax, parameters/parameter{name,type,description}, return{type, description} or text, remarks, see-also; `<class>`: name, parent, source, category, description, members/member@type@name@offset@size, properties/ property@name@type, methods/method@name@returns, remarks). They stay valid as a form. Parsing is strict XML: a malformed block is reported as an issue with file and line and skipped as a whole (the rest of the file goes on). Free text with `<` or `&` must be escaped or wrapped in `<![CDATA[ … ]]>`. On 2026-08-20 all 26 blocks of `FileTime.cpp` were well-formed.  ### 3.3 External documents (with front matter) **[open: details later]**  A file in the repository whose front matter names the owner and the key, and whose body is the annotation. Read with fmlines (front matter only, body untouched). The body may be prose or **code**: a `.prg` example (`owner: FILETIME64:AddDays`, `key: example`) is an `example` annotation, and when it is a complete program it doubles as a test — documentation that compiles and runs does not rot silently.  Front matter of an external `.md` (same mould as `_mkskill/src`: YAML with the namespaced key `xbdoc:`):  ```markdown --- xbdoc:   class: FILETIME64             # the entity: class: or function:   method: AddDays               # optional: the auxiliary, by its kind (method:, property:, ivar:, gwst-member:, class-method:, class-var:, class-property:) --- body = the note ```  `class:` or `function:` is mandatory (`class:` alone = the class itself; with an auxiliary key = that auxiliary); `source` is the file name, set by the extractor. **An external `.md` is always an extra note**: an accumulative `note` fragment added to whatever the markers say; it never replaces a `desc`, `return` or `param`, so there is no `fragment:` key and no precedence question. It attaches to a class, to one of its auxiliaries or to a function, never deeper (no notes on a parameter or a flag). A `.prg` example cannot start with `---`: its front matter goes in a sibling `.fm` of the same name (fmlines `extern`), the body is the `example`.  ## 4. Identity  Every annotation attaches to an entity by key:  - `project` — the project itself (overview chapters). - `class: Name` — a class. - `class: Name` + `<auxiliary-kind>: name` — one of its seven auxiliaries. - `function: name` — a free function.  Inside a scope the owner is implicit.  References written by the programmer (`calls`, `see-also`, `parent`, `gwst-parent`) use these same keys. Nothing is inferred: the generator renders the links that are written, in both directions only when both are written, and it resolves each name itself — what kind of thing it is, which section it lives in, how to link it; a name that exists neither in the project nor in the list of external names is a diagnostic. Comparison is case-insensitive. An annotation whose owner cannot be resolved in any loaded project is an issue.  ## 5. The tool reads only the comments  The extractor reads the markers (and the XML blocks, and the external documents) and nothing else: the C++ code between markers is opaque text. It does not parse `MethodCB` / `PropertyCB` / `Member_*` calls, code blocks or the `.xbmac`. Hence:  - A marker carries everything that must appear: public name, signature, return,   parameters with type and text, `type`/`pos` of structure members. - There is no `registration` data and no cross-check against the code (nothing   is derived, nothing is compared). Proximity is what keeps the documentation in   step with the code, not a mechanical check. - Writing conventions for class registrations (by the author, not the tool): the   `s` of a code block is Self and is not documented; a method whose block ends in   `,s` is `return: Self`, one ending in `,NIL` is `return: NIL`; literal arguments   (`.T.`, a fixed format, a multiplier) are explained in `desc`; a method that   wraps a documented function repeats the relevant parameter texts (the tool will   not fetch them).  Found while annotating FILETIME64: the class XML says `ToLocalTime` returns Self, but `{|s| FT64_TOLOCALTIME(s)}` has no `,s` and returns NIL. The marker documents NIL; the code decides — and only a human reading it notices.  ## 6. The intermediate model (and its container)  The model below — entities, annotations with provenance, relations, resolution rules, checks — is what matters; the container is an implementation detail. It could be plain Go structures in memory, an XML dump (cargoxml: readable, tolerant to foreign data) or a database. The suggested container is SQLite through `modernc.org/sqlite` (pure Go, no cgo; speed is not a concern), because inheritance and the checks become queries and a kept file can be interrogated. **One database per project.** A reference to a class of another project (a project built on ot4xb deriving from one of its classes, e.g. a `gwst-class` with `gwst-parent: RECT`) is an **external** reference: it is stored by name, rendered as a name (or as a link to the other project's documentation) and never resolved locally. Optionally one database per source file, attached at generation time (`ATTACH`) like objects linked together. **Not even persistent by default**: it is built in memory (`:memory:`) on every run and dies with it; `-keep` writes it to disk for debugging. Incremental extraction by `sha256` only makes sense when a kept database is reused.  Tables (essential columns):  | table | columns | |---|---| | `meta` | key, value — project name, version, repo, extraction data (git commit, time, counters) | | `unit` | id, kind (`class`/`function`), name, source, category, is_gwst | | `member` | id, unit_id, kind (`method`/`gwst-member`/`property`/`ivar`/`class-method`/`class-var`/`class-property`), name, type, pos, size, access, ordinal | | `param` | owner_kind, owner_id, ordinal, name, type, optional, byref | | `annotation` | owner_kind, owner_id, key, value, form (`marker`/`xbdoc`/`external`), source, ordinal | | `relation` | from_unit, to_name, kind (`parent`/`gwst-parent`/`see-also`/`calls`), ordinal | | `tag` | owner_kind, owner_id, key, value — keys the grammar does not know yet | | `issue` | severity, code, source, line, message — documentation diagnostics only | | `doc_fts` | FTS5 over descriptions, remarks, chapters — lookup for people and agents |  Descriptions, return texts and parameter texts are **not** columns of `unit`, `member` or `param`: they are rows of `annotation`, so that several annotations of different forms can coexist for the same entity with their provenance. Provenance is `source`: the **file name**, detected by the extractor and attached to every entity, auxiliary and fragment (never written by hand); line numbers are not part of the model — they appear only in diagnostics.  Resolution is a **view**, not stored data:  - Single-valued keys (`desc`, `return`, `param x`, `since`, …) come only from   markers and `<xbdoc>` blocks (external documents only add notes and examples).   When both forms document the same key, the marker wins over the XML block;   within one form the closest to the code wins. Two different values at the same   level → issue. - Accumulative keys (`example`, `see-also`, `note`): all values, in precedence   order.  ## 7. Inheritance resolution  A recursive CTE over `relation` (`parent`, `gwst-parent`) yields every ancestor with depth and path (cycle guard by depth and by path). Multiple inheritance may be parallel: several branches can reach the same ancestor (the usual diamond, every branch ending in GWST). Ancestors are therefore deduplicated by entity (`DISTINCT` on the ancestor; every `path` is kept for display). The effective members of a class are the **union** of its own members and those of every ancestor, each row carrying `inherited_from` and `depth`; outputs can show "inherited from GWST". Parents are complementary: a member reached through two paths from the **same origin** is one member; the same name coming from two **different origins**, or a parent repeating a name the child defines, is not resolved silently — it is an issue.  GWST inheritance is special and positional: there is exactly **one** `gwst-parent` per class, and the structure members are inherited in successive positions — the chain GWST → … → `gwst-parent` lays its members first and the class appends its own `gwst-member` rows after them. The other (parallel) parents contribute methods and variables, never layout. The offset of an own member is therefore the accumulated size of the whole GWST chain plus what the class itself accumulated before it, unless an explicit `GwstSetOffset` rewinds it (`qft` at `pos: 0` in FILETIME64); that is what the check recomputes and compares with the documented `pos:`. Cycles are issues. A parent that does not exist in the database is an **external** parent (a class of another project, e.g. an ot4xb class used by a project built on it): its members are not listed, the class is shown as inheriting from it by name, and `check` only complains when the name is not in the configured list of known external names (so typos are still caught).  ## 8. Consistency checks (the `check` leg, no output written)  Only the documentation itself is checked — there is no code to compare with:  - Malformed marker (unknown kind, missing value, unbalanced or nested scopes,   forbidden `}}` / `*/` / `|` in text) or malformed XML block. - Duplicate names inside a class, duplicate single-valued annotation at equal   precedence. - `parent` / `gwst-parent` / `see-also` targets that exist neither in the project   nor in the configured list of external names. - A method signature whose `param` lines name parameters the signature does not   have (or the other way round). - Documented `pos` of structure members not matching the documented sizes along   the GWST chain (optional; from the documentation only).  Every issue carries file and line. A non-zero exit code makes it usable in CI.  ## 9. Outputs (the "link" products)  - Markdown sections into `_mkskill/src/` → README / AGENTS / SKILL through mkskill. - An HTML reference (one page per class / function, inherited members listed). - Changelog derived from `since` / `deprecated` annotations in the sources (not   from comparing databases). - Agent lookup: a query command over the database (`find FILETIME64:AddDays`),   so a future Xbase++ skill does not need a monolithic text. - Examples with front matter compiled/run as tests.  ## 10. Open decisions  1. ~~`class name:` → `class-name:`~~ — settled: `class-name:`. 2. ~~Confirm `property:` and the spelling of the class auxiliaries~~ — settled:    the seven auxiliaries are `method`, `gwst-member`, `property`, `ivar`,    `class-method`, `class-var`, `class-property`. 3.   `/` for type alternatives; `|` in text   — settled: `/` for alternatives; code    between backticks (single inline, triple for blocks), where `|` is text. 4. ~~Precedence order of forms~~ — settled: marker over XML block; externals only add. 5. ~~Front matter schema of external documents~~ — settled: `xbdoc:` with    `class:`/`function:` (+ auxiliary kind) and `fragment` (§3.3); multi-fragment    files, if ever, later. 6. Whether the `<xbdoc>` blocks are kept as a living form or retired once the    markers cover their content. **Done for `FileTime.cpp` on 2026-08-20**: its 26    blocks were migrated to markers (25 `begin-function` scopes with the doc block    inside, the class notes on `class-name:`) and removed; the other source files    keep their XML until converted. 7. Where the extractor/generator lives (a subcommand of ot4xb-tool, or its own    tool) — out of scope until the model is closed.  ## Appendix — current state (2026-08-21)  - `_ot4xb_\source\conMCallEx.cpp`: the 91 `OT4XB_API` C++ overloads of the `_conMCall*`   family (method calls: `Self` = the object, `pFN` = method name; `_conMCallCon` 14,   `_conMCallConN`/`_conMCallConNR` variadic with `nParams`, the typed `Void/Bool/Long/Double/`   `Float/Lpstr`, `Void` 14), same layout as conCallEx.cpp: one `cpp-function` scope per overload,   `mangled-name:` 91/91 in the export table (one definition is indented by a blank before   `OT4XB_API`; scope markers must tolerate leading blanks). - `_ot4xb_\source\conEvalEx.cpp`: the 70 `OT4XB_API` C++ overloads of the `_conEval*`   family (`_conEvalCon`, `_conEvalVoid`, `_conEvalBool`, `_conEvalLong`, `_conEvalDouble`,   `_conEvalFloat`, `_conEvalLpstr`: the `_conCall*` twins that evaluate a code block `conb`   instead of calling a function by name) documented exactly like conCallEx.cpp: one   `cpp-function` scope per overload, `category: ot4xb-api`, `header`, `mangled-name:` (70/70   in the export table of `ot4xb.dll`), `desc`, `param`, `return`, `see-also`. - `_ot4xb_\source\conCallEx.cpp`: the 80 `OT4XB_API` C++ overloads of the `_conCall*`   family (`_conCallConR`, `_conCallCon`, `_conCallVoid`, `_conCallBool`, `_conCallLong`,   `_conCallDouble`, `_conCallFloat`, `_conCallLpstr`) documented as `cpp-function`, one   `begin-cpp-function` … `end-cpp-function` scope per overload with its own prototype,   `category: ot4xb-api`, `header: ot4xb_cpp_exported.h`, `mangled-name:` (every one of   the 80 decorated names checked against the export table of `ot4xb.dll`), `desc`, `param`, `return`,   `see-also`. The non-exported `_conRelease_ret_*` helpers stay undocumented. Grouping   the overloads of a family is deferred. - `_ot4xb_\source\ClrTool.cpp`: 7 `extern "C"` colour functions documented from scratch as   `c-function` (category `winapi/color`, `mangled-name:` = the plain export name, checked   against the export table of `ot4xb.dll`); statics left undocumented. - `_ot4xb_\source\Bitwise.cpp`: fully converted — 15 `function` + 14 `c-function`   scopes (the `<ot4xb-c>` blocks), operators in prose between backticks; `mangled-name:` added to the 14 c-functions on 2026-08-21 (export names, checked against the export table of `ot4xb.dll`). - `_ot4xb_\source\array2dbf.cpp`: converted (1 function; Pablo set the definitive   layout on it). - `_ot4xb_\source\FileTime.cpp`: fully converted. 25 functions with   `begin-function` … `end-function` and a doc block inside (`function:` signature,   `category`, `desc`, `param`, `flag`, `return`, `note`, `see-also`); FILETIME64 with   `begin-class` … `end-class`, `class-name:` block (category, desc, notes),   `gwst-class`, 3 `gwst-member`, 8 `property`, 42 `method` markers reduced to   signature + `return` + `calls:` + one-line `desc` (+ `param` where the method   parameter differs from the function one); 132 markers, no `<xbdoc>` left, no   informal trailing comments on the `_XPP_REG_FUN_` lines. Type alternatives   written with `/`. Backups: `scratchpad\FileTime.cpp.before-markers` (original),   `FileTime.cpp.before-functions` (after the first marker pass). - mysql4xb: no documentation yet (README stub, no XML blocks); its four classes   (`MYSQL4XB`, `MYSQL4XB_RESULTSET_T`, `MYSQL4XB_RESULT_ROWS_T`,   `MYSQL4XB_RESULT_FIELD_T`) are plain `TXbClass` runtime classes — no GWST, no   declared parent — built with ot4xb's class machinery; the pilot for this system. ` being the same variable in PowerShell. Excluded from
the batch: winapi_CommonStructures.cpp (398 GWST classes, Pablo's own recent fix there, separate
treatment later). Open question for Pablo: the class skeleton writes `class-function: NAME` (my
key, not in the spec) — keep or drop.

**Batch result (2026-08-21):** 75 files run, 72 OK first pass; the 3 restored were my own checks
firing (blank line between an XML block and its header; K&R `header {`; one-line body `{ ... }`
below the header) — all three layouts added, base32 and PointerEx re-run OK; `ot4xb_argon2.cpp` is
Pablo's now ("voy con el de argon a revisar": its block holds TWO <function> in one <xbdoc> and
attribute-style <parameter>; excluded from batch.ps1). Global check over source\*.cpp: 85 files,
3993 marker lines, 690 `todo` fields, 439 XML blocks left (398 CommonStructures + 41 pending), 0
unbalanced / 0 `*/` inside blocks / 0 negative depth / 0 lines > 140, 1 bare definition
(ARGON2_VERIFY, Pablo's file). Pending XML (manual): 16 class, 9 function-family, 5 method, 4
multi-function, 1 class-group, 3 mixed, 3 function blocks not followed by their function
(OSVer.cpp, ot4xb_dirty_dlgedit.cpp, TXbClass_internal.cpp — look at what sits between). Class
skeletons were ALSO made for 9 classes whose XML class block is not right above the class function
(GWST, OT4XB_CNG, OT4XB_HASH, TBinFile, TFileWriter, TXbClass, _LARGE_INTEGER_/LARGE_INTEGER/
ULARGE_INTEGER, OT4XB_GENERIC_POINTER; TXbClass_internal has `class-name: TODO`) — duplicate
doc to merge by hand when those classes are converted. Not exported by the installed DLL:
`*_watch_thread_conc` x4 (Container.cpp), `*_xwatch_thread` x4 (memory.cpp), `ot4xb_qsort_ui32*` x3
(fix_item_list.cpp). Offered to Pablo, not decided: multi-function blocks (match each <function>
to its definition by <name>), attribute-style <parameter> (converter already reads them now).
Skeleton cosmetics to improve some day: signature wrapping prefers a blank, not a `, `.

**File by file from here (Pablo: "vamos fichero por fichero").** ot4xb_argon2.cpp done by hand on his
instruction ("separas las 2 funciones y los flags pues tendras que repetirlos en las 2"): two
`function` scopes, every text from the XML (attribute-style params), the six `flag` lines in
argon2_hash and the three encoded ones (0x0000/0x0001/0x0002) in argon2_verify — my reading of
his remark that raw output is not accepted by verify; he may want all six there too. Informal `//`
syntax comment before ARGON2_VERIFY removed. Backup `ot4xb_argon2.cpp.before-split` (his reviewed
version). Code identical, 6/6 markers, 0 XML. Then the 3 orphan `function` blocks were merged into
their skeletons with `scratchpad\merge-orphan.ps1 -File <cpp> -Name <xml name>` (XML texts replace
the skeleton block, kind recomputed, XML removed; backups `*.before-merge`): OSVer.cpp
(`ot4xb_fill_OSVERSIONINFOEX`, block sat above another function), ot4xb_dirty_dlgedit.cpp (a
separator between block and header — converter now skips separators too), TXbClass_internal.cpp
(`_xbmtpf1_`: the skeleton had guessed `class` because the body uses TXbClass — detection now needs
`->ClassName(` / `.ClassName(`). All three: code identical, 0 todo, 0 XML. Pending XML blocks now 38
(+ CommonStructures). Review order proposed to Pablo: OSVer first, then the files with pending
XML (multi-function: PeekPoke, TLXbStack, TBinFile; families; classes), then the rest.

**Export cross-check (`scratchpad\check-exports.ps1 [-Files a.cpp,b.cpp]`):** every `function:` scope's
documented Xbase++ name (upper-cased) must be an exported symbol — the C symbol may differ, e.g.
`wapimc_LOWORD` is exported as `LOWORD` through the .def (14 such in NumAndBytes.cpp, all fine) —
and every c/cpp scope's `mangled-name` must be exported. Result 2026-08-21: 479 function scopes
(477 ok), 825 c/cpp scopes (814 ok + 11 skeletons without mangled-name = the not-exported ones).
The only real discrepancy: DrTool.cpp — the XML doc `GetProcessArgv()` sits above the one-line
alias `_XPP_REG_FUN_( GETPROCCESSARGV ) { GETPROCESSARGV( pl ); }` and the DLL exports ONLY the
misspelled `GETPROCCESSARGV`; the correctly spelled implementation `GETPROCESSARGV` is not exported
(it got a skeleton). Its own note claims the misspelling is the alias and `GetProcessArgv()` the
real one — from Xbase++ only `GetProccessArgv()` is callable with the installed DLL. Told Pablo;
probably a missing entry in ot4xb.xbmac. Use this checker after every file.

**ot4xb_expando.cpp done (2026-08-21, with Pablo):** the class is built in`static void create_class(XppParamList pl)` inside a namespace (Pablo: done that way to use a
namespace, historical reasons); `_XPP_REG_FUN_(_OT4XB_EXPANDO_)` is only the exported entry point
and stays unmarked with his own `// docu` comment (he removed my skeleton there himself). Script
`scratchpad\expando-class.ps1`: `begin-class`/`end-class` around create_class, `class-name:
_ot4xb_expando_` block (category, `syntax:` = the XML's own key for the `:new( [nFlags] )` line,
desc, 3 notes from remarks, `see-also` with text, `example` as a fenced code block inside the
block), and 32 markers on the registration lines: 7 `ivar` (`access: internal`), 23 `method`, 2
`class-method`, flags grouped by method from the XML `<flags name=...>`; 3 `todo` left (desc of
`init`, `ot4xb_expando_init`, ivar `__m__unserialize__info__`). Pablo: "es mucho mejor separarlo con
la explicacion junto a sus miembros". Code identical to `ot4xb_expando.cpp.before-class`; NOTE he
edited the file concurrently (commented out the dead `if (method_* == 0) { ; }` block, to be
deleted) and his save landed on top of my version: both survive, 38/38 markers, balance ok. Lesson:
when Pablo says he is editing, re-read before every write and never run the regenerate-from-backup
scripts on that file. Review outcome with Pablo (same day): `access: internal` was
WRONG as a key — in Xbase++ HIDDEN/PROTECTED ivars cannot be manipulated from outside the class
scope, where the code-block methods run, so ot4xb ivars are always EXPORTED (`pc->EXPORTED()`) even
when they are pure implementation state; the XML's `access="internal"` meant "not meant to be
manipulated outside the class". Now: every `__m__*` ivar carries `desc` + `note: Internal state, not
meant to be manipulated outside the class.` and the class block explains why once; the `access:`
I had invented on two methods is gone. The `init` registration documents `new( [nFlags] )` ("init es
el metodo que se llama automaticamente desde new asi que puedes documentar new"), so the
class-block `syntax:` line was dropped. 2 todo left (`ot4xb_expando_init` desc, ivar
`__m__unserialize__info__` desc). GetNoIVar/SetNoIVar = the hooks for `o:var` / `o:var := x` when
the ivar does not exist, delegating to get_prop/set_prop ("los nombres adecuados").

**DrTool.cpp classes done (2026-08-21):** `scratchpad\drtool-classes.ps1` (backup
`DrTool.cpp.before-classes`) converted the two XML class blocks: `_TDriveInfo_` (plain TXbClass: 7 `ivar`
with `type`, `new()` on the `init` line — desc written from the code block: "Creates the object with
all members reset to empty values." —, 13 read-only `property` with `type: Logical` and `note:
Read-only property.`) and `WIN32_FIND_DATA` (GWST: `gwst-class` on `GwstParent()`, 10 `gwst-member`
with `type` incl. the `Child(...,"FILETIME64")` ones, 16 `property`, ivar `_find_handle_` with Pablo's
old `//` comment as desc, 3 `method` with the `//` syntax comments absorbed; `syntax:` of `:New()` kept
in the class block because there is no init line; example fenced). 214/214 markers, 0 XML, code
identical, 13 `todo` left = the c-functions without texts. Open for Pablo: GetProcessArgv (doc above
the misspelled exported alias, real GETPROCESSARGV not exported).

**DrTool.cpp reconciled + re-documented (2026-08-21, evening).** IMPORTANT git/version lesson:
while I documented DrTool.cpp, Pablo had it open in another editor with an OLDER buffer; his save
landed on top, then he **reverted DrTool.cpp to master** (git) and saved the pisado buffer as
DrTool - copia.cpp (later deleted). Net: my documented version was lost. master was the complete
code base but MISSING Pablo's session code changes (they were only in the copia, which itself had
lost 4 functions + the WIN32_FIND_DATA class). I compared code (comments stripped) across master /
copia / my original backup: master = complete code, minus 6 of Pablo's edits. MSVC code analysis
(SAL) flagged those spots; Pablo pasted the warnings and I applied all six to master, byte-safe,
each with a unique anchor: (1) lDiskReady if( p->ht != NULL ){...} else { _xfree(pPath); timeout;
return FALSE } (C6258/handle=0 x3); (2) #pragma warning( disable: 6258 ) before the shutdown
TerminateThread loop — accepted because ot4xb is never loaded/unloaded dynamically, so shutdown
cleanup need not be clean (new memory ot4xb-not-dynamically-loaded); (3) GetWinDir captures
GetWindowsDirectory return (C6031); (4,5) two pLastP != NULL guards (C6011). Backups:
DrTool.cpp.master-fixed = master + the 6 fixes, no doc. Then re-documented from that base with
xbdoc2markers + skeletons + drtool-classes2.ps1 (consolidated class script with every reviewed
adjustment: _TDriveInfo_ 7 ivar + 13 RO property; WIN32_FIND_DATA gwst-class, 10 gwst-member with C
types + pos/size — sizeof 320 —, 16 property, _find_handle_, 3 methods, notes, see-also class GWST,
no :New syntax). Result: code identical to master-fixed, 214/214 markers, 0 XML, 13 todo (the
c-functions). LESSON: never run regenerate-from-backup scripts on a file Pablo may be editing; when
a file looks wrong, compare CODE (comments stripped) across every copy before touching anything, and
let Pablo drive the git side.

**Duplicate-name (typo/alias) pattern in DrTool.cpp (2026-08-22/23).** For a historically
misspelled exported name, both names stay exported: the correctly spelled one is the real
implementation and gets the full doc; the misspelled one is a thin alias that delegates, with a
short doc + calls: + a note (kept only so existing external code keeps working). Applied to three
names: GetProcessArgv/GetProccessArgv (Xbase functions, both already in ot4xb.xbmac lines 83-84;
GETPROCESSARGV needs a DLL rebuild), get_current_directory/get_currrent_directory and
set_current_directory/set_currrent_directory (c-functions; the good name added to
ot4xb_c_exported.h, internal calls switched to it). Pablo: cambias en todas partes dentro de ot4xb
al correcto y mantienes el alias por si se uso fuera; documentas el typo que conservamos. When
restructuring, extract the impl body and rebuild two begin/end-c-function scopes; the forward
declaration is dropped once the good name is defined first.

DrTool.cpp FULLY DOCUMENTED (2026-08-23): 0 todo, 220/220 markers, 2 begin/end-class,
40 function scopes, 13 c/cpp-function scopes, code identical to master-fixed, CRLF/1252, no XML.
The recursion callbacks were the interesting part: ot4xb_recurse_dir_ex is the real impl (flags
0x01 wildcard/list, 0x02 hidden, 0x04 system, 0x10000 mask list; callback returns 0=ok/1=cancel),
ot4xb_recurse_dir wraps it with flags 0. ot4xb_recurse_dir_item_codeblock is a ready-made
_PFN_OT4XB_RECURSE_DIR_CREATE_ITEM_ that evaluates an Xbase code block { |cargo,cPath,pW32FindData| }.
Cargo model (Pablo, learned in pieces): a __i32 churro (plain 3-int32 array, direct order, exported
__I32 in PeekPoke.cpp) built as __i32( _var2con(codeblock), _var2con(cargo), _var2con(result) ) so
that offsets 0/4/8 = codeblock/cargo/result, matching the code _conEvalB(pcon[2]=result,
pcon[0]=codeblock, ...). At the end: result := _conRelease( PeekDWord(bin_cargo, 8) ) and release the
other two. KEY: at PRG level _conRelease returns the container Xbase value AND frees it (acts as
_con2var + release) - put that in the _conRelease doc when we reach it. Still not exported in the
installed DLL (checker flags, expected until rebuild): GetProcessArgv, get_current_directory,
set_current_directory. Recurring PowerShell trap this session: comma binds tighter than +, so
@(a, prefix + x) splits - parenthesize; it split 4 markers as a lone /*{{ line (balance did not
catch it, a raw dump did).
`scratchpad\concallex-doc.ps1` restores `conCallEx.cpp.before`, documents every
overload and computes the decoration (x86 cdecl `YA`; two back-reference tables:
names — the function name is 0, struct names follow, also from the return type —
and parameter types; `ContainerHandle` = `struct MomHandleEntry *` =
`PAUMomHandleEntry@@`), accepting a name only if the literal string exists in
`C:\pli\ot4xb\ot4xb_cpp.lib` (2026-06-09): 80/80 verified, none extra, none
missing. Overload grouping: "luego ya las agruparemos" (later). The five
`_conRelease_ret_*` helpers (not exported) stay undocumented. The `_conCallConR`
desc says the caller keeps ownership of the containers passed — true today only
because `TContainerHandleList::ReleaseAll` releases nothing (the indexing bug);
flip that sentence if Pablo fixes it. PowerShell trap found here: inside
`@( 'a', 'desc: ' + $x )` the comma binds tighter than `+`, so the text becomes a
separate element — parenthesize `('desc: ' + $x)`; this caused the lost/split
fields of both conCallEx attempts.
Remaining: ~43 files, 62 `<ot4xb-api>` blocks (Container 52, OSVer 5, ot4xb 4,
TCriticalSection 1) plus the xbdoc ones; winapi_CommonStructures.cpp last. Details
and the Xbase++ class-model explanation Pablo gave (class object singleton vs
instance, GWST class var holding the structure definition) are in the memory
note `xbase-class-model-and-srcdoc-markers`.

## NEXT UP / pending

- **ot4xb.xbmac is done** (2026-08-20, at Pablo's request): the five
  `_CDECL_EXPORT_( ... )   // SRC: Container.cpp` lines sit after the last
  `_XPP_REG_FUN_` entry (CRLF, column-69 comments, rest of the file untouched;
  original copy kept in the scratchpad). The whole pipeline was verified on a
  copy: xbmac2h → `ot4xb.def` (917 imports) → def2lib20 monkey → `ot4xb.lib` →
  ALINK → the e2e program runs with every callback correct. **Pablo: rebuild
  ot4xb** (pre.release regenerates the .def/.hpp files, post.release builds the
  new `ot4xb.lib`) and ship it; applications then link `callbacks.obj` +
  `ot4xb.lib` only. The duplicate `WAPIST_NMSELCHANGE` registration of the
  `.xbmac` was removed too (Pablo's request), so def2lib20 no longer warns.
  Lesson: never `sed -i` a CRLF file here (MSYS sed rewrites it as LF); edit
  with PowerShell/Go and check with `file`.
- Decide whether the e2e PRG/cbk of the scratchpad go under `pruebas/`.
- Repo README / light docs (Pablo keeps Xbase++ maintenance minimal; maybe a
  plain README; `vbuild.md` already sits under `_mkskill/src`).
- GitHub autorelease workflow (CI runs `go run ...-bs ...` on a Windows runner).
  Deferred. Longer-term aparcado: a `cl`/`msbuild` step (build without VS), own
  linker, autodeploy/extract for third parties. Not now.

Heads-up: the machine-code / COFF topic can trip Fable's broad `[cyber]`
safeguard and the harness may auto-switch to Opus 4.8. It is a false positive on
legitimate build tooling; re-select Fable with `/model` if wanted.

## Quick verify

    cd C:\HD\F\__pbnprj\_pub\xb\ot4xb-tool
    gofmt -l .   &&   go vet ./...   &&   go test ./...
    set FASM=C:\HD\F\__pbnprj\_pub\xb\dev-tools\xppcbk\FASM.EXE && go test ./modules/cbk2obj/   (FASM differential)

The `pruebas/` folder has Pablo's real `ot4xb.def`/`ot4xb.lib`, the 010-editor
template `COFFLib.bt` (parses a COFF `.lib` in full), and the PE/COFF import-lib
spec notes with verbatim spec quotes. Legacy xppcbk sources + FASM.EXE:
`C:\HD\F\__pbnprj\_pub\xb\dev-tools\xppcbk`; analysis docs:
`C:\HD\F\__pbnprj\_pub\xb\dev-tools\go-port\01-analysis-*.md`, `02-plan.md`.


FileTime.cpp FULLY DOCUMENTED (2026-08-23): the batch had documented the 25 Xbase++
functions (_XPP_REG_FUN_ FT64_*) and the FILETIME64 class but left the 16 exported C
primitives (OT4XB_API __cdecl ft64_*) with no scope of their own (begin-c was 0). The
pattern: the C ft64_* is the real implementation, its _XPP_REG_FUN_ sibling right below
is the Xbase wrapper (adds flags/lLocal/object-or-NIL). Gave each C function its own
begin-c-function scope (mangled-name = plain cdecl name, all in ot4xb_c_exported.h),
desc derived from the sibling but trimmed to the primitive. Final: begin-c/end-c 16/16,
25/25 fn, 1/1 class, markers 180, 0 todo, 0 xml, depth ok, CRLF/1252, unscoped-cfns 0.
Notables: return buffers via _xgrab (GetTs 256, strf_l 1024) or _xstrdup for ToHttp;
ft64_now's BOOL means "high-precision used" (GetSystemTimePreciseAsFileTime) not success;
UnixTime pair uses the 116444736000000000 (100ns) / 11644473600 (s) epoch offsets, _64
variants are C-only (no Xbase sibling); CKF32TS is an in-house ot4xb convention - a
non-binary 8-byte text timestamp with millisecond resolution, encoded/decoded by
base32_ns::Encode/DecodeCkf32Ts. Script lesson: for consecutive c-functions the
end-c-function of one sits directly above the next def, so the idempotency guard must
abort only on a bare "}}*/" (a header close), NOT on "/*{{end-c-function}}*/" (a scope
boundary) - the broad guard false-positived on ft64_get_Ckf32TsStr. One-liner defs
(ft64_strf) need end inserted on the def line +1, detected by balanced { } on the line.


fpCall.cpp (2026-08-23): old-manual <function-family> XML block extracted OUT to
C:\HD\F\__pbnprj\_pub\xb\_ot4xb_\fpCall_oldmanual_xbdoc.txt (reference only, NOT verified).
Pablo CONFIRMED (firm) one by one: Set_FpCall_Flags, nFpGet, F2T,
next_xbfpcall_use_critical_section, XbFpCall, _FpCall_PushFlags_. Then he left and gave
explicit free rein: "document the whole file as provisional, we review one by one on my
return". So the remaining 26 entities (20 Xbase fn + 5 c-fn + OT4XB_GENERIC_POINTER class)
were auto-drafted FROM CODE, each carrying "| todo: provisional - ..." so they are
greppable for the one-by-one review. Final: 26/26 fn, 5/5 c-fn, 1/1 class, 1 shared note,
5 includes, 26 provisional todos, depth ok, CRLF/1252, no XML.

NEW GRAMMAR (Pablo, define-on-the-fly): shared reusable note.
  Definition (placed at END of file): /*{{begin-note | id: <id>
               | title: <Title>}}*/  <free-text body>  /*{{end-note}}*/
  Include from a function header field: | note: {{include-note-id: <id>}}
First one: id fp-np-parameter-inference, title "Non Prototyped Function Pointer Parameters
Type Inference" - the Xbase-value -> C-type inference (Logical->BOOL; Numeric->LONG or
DOUBLE after a NIL marker; Character->locked-buffer pointer or QWORD after NIL; Array->temp
array ptr; GWST object->extended ptr; @by-ref -> pointer to value). Verified in
TXbFpParam::PrepareStackValues. Included in nFpCall/ndFpCall/qwFpCall/cPrintf/__printf.
Formatting chosen by me (option A: title inline + free-text body) - VALIDATE with Pablo.
TODO: record this note entity in 03-srcdoc-spec.md.

CONVENTIONS fixed this session: Xbase functions carry NO mangled-name (Pablo: "it confuses
people"; Xbase is case-insensitive) - the one exception is the wapist map of
winapi_CommonStructures, see [[wapist-func-mapping-ch]]. Names in readable case (Pablo gave
Set_FpCall_Flags, nFpGet, F2T; the rest I cased provisionally). NO links to Alaska docs
("the user has the manual"). Do NOT mention DllExecuteCall in the docs: nFpCall/ndFpCall/
qwFpCall do NOT use it (they call via inline asm push/call, saving+restoring ESP so both
stdcall and cdecl work); ONLY XbFpCall uses DllExecuteCall - saying otherwise feeds a
common user misconception. Return registers differ per call fn: nFpCall EAX(32-bit),
ndFpCall FPU/ST0(double), qwFpCall EDX:EAX(qword) - that's why there are three.

FINDINGS to raise on review: (1) bWriteLogLine BUG - ZeroMemory(buffer) runs AFTER vsprintf
and BEFORE dwWriteLogData, so a zeroed buffer is written, not the text (not touched, flagged
in its todo). (2) skeleton return types were wrong: _ot4xb_cprintf_c_escape_ is LPSTR not
void, bWriteLogLine is BOOL not void - corrected. (3) shared-note candidates NOT confirmed
by Pablo, only nFpCall/ndFpCall/qwFpCall/cPrintf were: I also put the include on __printf
(twin of cPrintf) and flagged cFmtResMsg/cFmtStrMsg/lWriteLogLine in their todos (they use
TXbFpParam too but with bDisableByRef=TRUE) - Pablo decides. (4) FpQCall/iFpQCall/fpLQCall/
fpLQCall2 prototype logic lives in TXbFpQParam.cpp (another file); their prototype/template
note belongs there. (5) OT4XB_GENERIC_POINTER class aux (ivar/methods) kept as header
fields, may need to move inline. (6) c-fn header/mangled for bWriteLogLine assumed
ot4xb_c_exported.h - confirm.

UPDATE (2026-08-24): Pablo DELETED __printf entirely (it was problematic - writes to the
CRT stdout, and "the Xbase++ console is not always the console"; the good pattern is
cPrintf() piped into the standard QOut()/QQOut()). Removed from fpCall.cpp (function + its
provisional doc + the ot4xb_printf_internal helper) and from ot4xb.xbmac. The COUT
#xcommand (DECLARE APPLICATION COUT CLASS METHODS) that used it was also deleted (never
documented). fpCall.cpp integrity re-verified after his edit: 25/25 fn (was 26), 5/5 c-fn,
1/1 class, shared note 1/1 with 4 includes (nFpCall/ndFpCall/qwFpCall/cPrintf), no orphan
includes, depth ok, CRLF/1252. The 3 GENERATED files still naming __PRINTF
(ot4xb_xbexports.hpp, ot4xb_xbfunclist.hpp, ot4xb.def) are left as-is: they regenerate on
recompile (they carry "////////// UNKNOW LINE #N>>>file.cpp<<<" markers = generated).

__printf compat #xtranslate (2026-08-24): documented ONLY in cPrintf's doc (a note + example
in fpCall.cpp), NOT added to ot4xb.ch (Pablo: "fuera de ot4xb.ch, solo en la doc"). The note
tells users to put, in THEIR OWN header, this drop-in for the removed __printf, routed to the
Xbase++ console via cPrintf: #xtranslate __printf( <x,...> ) => QQOut( cPrintf( <x> ) )
(passthrough, no leading NIL added, 100% compatible; QQOut = no auto newline, like __printf).

CORRECTION (2026-08-24) - shared-note grammar (supersedes the earlier "option A"):
My first form left the note body as bare text between }}*/ and /*{{end-note}}*/, i.e.
OUTSIDE any comment -> it breaks C compilation (my depth-check missed it because the
/* */ balance was still 0). Pablo fixed it live. Correct form (three self-contained C
comments; body inside its OWN comment):
    /*{{begin-note | note-id: <id>
                 | title: <Title>}}*/
    /*{{note:
    <free-text body>
    }}*/
    /*{{end-note}}*/
Also: the id field is now `note-id:` (matches include-note-id:). fpCall.cpp fixed and
verified (4 includes resolve, C-valid); 03-srcdoc-spec.md updated. Lesson: for anything
that spans lines in a .cpp, keep every line inside a comment; a balanced /* */ count is
NOT proof the text is commented.

Shared note fp-np-parameter-inference body: replaced my summary with the FULL old-manual
"Parameter type conversion" doc (Pablo pasted it; matches PrepareStackValues): LOGICAL ->
BOOL / BOOL*; NUMERIC -> LONG / LONG* (decimals truncated; shorter prototypes WORD/__int16/
BYTE/CHAR ok since 32-bit packs; for shorter-value pointers adjust with LoWord/LoByte);
CHARACTER by value -> LPCSTR (read-only), by reference -> LPSTR (read-write); NIL+CHARACTER
-> __int64 (8-byte binary, build with Double2LongLong) / __int64*; NIL+NUMERIC -> double /
double*; 32-bit float via PackFloat32() or float* by ref; ARRAY of numerics -> int32 buffer
(written back if by ref); GWST object -> pointer to the structure. C-valid (body inside the
/*{{note: ... }}*/ comment), 4 includes resolve. Only >120 lines left are Pablo's own code
(232, 485) and the XbFpCall example fence (188, len 121, tolerable).

INCLUDE FORMAT CHANGE (2026-08-24, Pablo): a shared-note include is NOT a header field.
It is a STAND-ALONE marker /*{{include-note-id: <id>}}*/ (its own self-closed comment),
placed loose inside a scope between begin and end (typically right after the header,
before the code). It counts as an include ONLY there - never nest {{ }} inside a | field.
Old form | note: {{include-note-id: X}} is dropped. fpCall.cpp migrated (10 loose includes:
nFpCall/ndFpCall/qwFpCall each get _dwGetFpParam_ + fp-np-call + fp-np-parameter-inference;
cPrintf gets fp-np-parameter-inference), all resolve. Notes now in fpCall.cpp: note-id
_dwGetFpParam_ (how to provide fp, above the helper), fp-np-parameter-inference (full manual
param-conversion table, at end), fp-np-call (CDECL/STDCALL/FASTCALL-THISCALL-no + gencode 21
+ pushflags, at end). Reference PDF: C:\HD\F\__pbnprj\_pub\xb\ot4xb-docs\
ot4xb-call-function-pointers-and-dll.pdf (8 pages, the authoritative fpCall family doc).
Also updated: scandoc-parser-prompt.md (.claude, new InScopeBody FSM state) and
03-srcdoc-spec.md.

## 2026-08-24 - con*Ex overloads migrated + scandoc verified
- Migrated conCallEx.cpp (80), conEvalEx.cpp (70), conMCallEx.cpp (91) = 241
  cpp-function headers to identity key `name(TYPES)` compact (no spaces). Unique
  names stay name-only: _conCallConR, _conMCallConN, _conMCallConNR. Full
  prototype moved to `| syntax:`; return type lives only in syntax + mangled-name.
- Verified: `ot4xb-tool scandoc -src <f>` -> exit=0, "N entities, 0 notes", zero
  warnings/errors on all three. Control on old-form array2dbf.cpp shows the
  expected `old-form header value (a signature)` warning, so the clean result is real.
- Earlier same turn: fixed 2 split-signature syntax fields in DrTool.cpp
  (recurse_dir/_ex, now fenced) + ft64_strf_l fenced. fpCall/DrTool/FileTime clean.
- Pending (deferred): ~81 other files still old-form (scandoc flags them); on request.

## 2026-08-26 - QCall (Qualified) family documented
- Naming: FpQCall / IFpQCall / FpLQCall / FpLQCall2 (Q=Qualified, I=Interface -> uppercase; the
  return-type prefixes n/nd/qw/c/l stay lowercase). Identities in fpCall.cpp fixed to these
  (case-sensitive; the C registration FPQCALL... left untouched).
- 3 shared notes, each next to its source:
    fp-qtype-prototype  + qcall-return-values   -> TXbFpQParam.cpp   (type switch + FCall/return)
    qcall-parameter-values                      -> TXbFpQParam_init.cpp (IO_QT_ / parameters)
- fp-qtype-prototype: the 4-byte QTYPE code list (full: by value / by reference / strings), return
  code first, __vo return-only; real usage = a #xtranslate hard-coding the codes (see the shipped
  *_prototypes.ch, e.g. wininet_prototypes.ch); higher-level DLL IMPORT/QTEMPLATE/AS<TYPE> -> ot4xb.ch/.md.
- fpCall.cpp includes (multifile): FpQCall = _dwGetFpParam_ + the 3 notes; IFpQCall/FpLQCall/FpLQCall2
  = the 3 notes. scandoc single-file flags "no matching begin-note" / "never included" (cross-file,
  resolved by the parser thread's 2nd pass) - expected; the doc is correct.
- Grammar confirmed by Pablo: include-note-id is MULTIFILE; notes are global to the project,
  resolved in a second pass.
