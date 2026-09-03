# COFF import libraries ("long" format) — what the official spec says

Source: *Microsoft Portable Executable and Common Object File Format Specification*,
Revision 11, January 23, 2017 (the text of these sections has been the same since the
1999 revisions; it is also the current "PE Format" page on Microsoft Learn).
Quotes are verbatim; comments in *italics* are ours, checked against the 1995 import
libraries shipped with Xbase++ (`C:\Alaska\XPPW32\lib\KERNEL32.LIB`, built by
"Microsoft LINK 2.60.5112 (NT)", i.e. Visual C++ 2.x) which `alink` accepts.

## 7. Archive (Library) File Format

> The COFF archive format provides a standard mechanism for storing collections of
> object files. These collections are commonly called libraries in programming
> documentation. The first 8 bytes of an archive consist of the file signature. The rest
> of the archive consists of a series of archive members, as follows:
> * The first and second members are "linker members." [...] The linker members
>   contain the directory of the archive.
> * The third member is the "longnames" member. This member consists of a series of
>   null-terminated ASCII strings in which each string is the name of another archive
>   member.
> * The rest of the archive consists of standard (object-file) members. Each of these
>   members contains the contents of one object file in its entirety.

### 7.1 Signature: `!<arch>\n`

### 7.2 Archive Member Headers

> Each member (linker, longnames, or object-file member) is preceded by a header. An
> archive member header has the following format, in which each field is an ASCII text
> string that is left justified and padded with spaces to the end of the field. There is
> no terminating null character in any of these fields. [...] Each member header starts
> on the first even address after the end of the previous archive member.

| Offset | Size | Field | Description |
|---|---|---|---|
| 0 | 16 | Name | name with a slash (/) appended; `/` = linker member (both); `//` = longnames; `/n` = name at offset n of the longnames member |
| 16 | 12 | Date | ASCII decimal seconds since 1/1/1970 |
| 28 | 6 | User ID | Microsoft tools emit all blanks |
| 34 | 6 | Group ID | Microsoft tools emit all blanks |
| 40 | 8 | Mode | ASCII octal ST_MODE |
| 48 | 10 | Size | ASCII decimal size of the member, header excluded |
| 58 | 2 | End of Header | `` `\n`` (0x60 0x0A) |

> `//` The archive member is the longnames member [...]. The longnames member is the
> third archive member and **must always be present even if the contents are empty**.

*Xbase++'s KERNEL32.LIB and MS `lib.exe` output follow this; ImpLib SDK omits the
member when no long name exists.*

### 7.3 First Linker Member

> The name of the first linker member is "\". The first linker member is included for
> backward compatibility. It is not used by current linkers, but its format must be
> correct. [...]
> Number of Symbols (4, **big-endian**) — Offsets (4*n, **big-endian** file offsets to
> archive member headers) — String Table (null-terminated symbol names).
> The elements in the offsets array must be arranged in ascending order. This fact
> implies that the symbols in the string table must be arranged according to the order
> of archive members.

### 7.4 Second Linker Member

> The second linker member has the name "\" as does the first linker member. Although
> both linker members provide a directory of symbols and archive members that contain
> them, **the second linker member is used in preference to the first by all current
> linkers**. The second linker member includes symbol names in lexical order, which
> enables faster searching by name.
> Number of Members (4) — Offsets (4*m, ascending) — Number of Symbols (4) —
> Indices (2*n, 1-based indexes into the offsets array) — String Table (names in
> **ascending lexical order**).

*(little-endian). `alink` 1.90 rejects a library without this member ("ALK4002: invalid
or corrupt file") — verified 2026-08-19.*

### 7.5 Longnames Member

> The name of the longnames member is "\\". [...] A name appears here only when there
> is insufficient room in the Name field (16 bytes). The longnames member can be empty,
> though its header must appear.

## 8. Import Library Format

> Traditional import libraries, that is, libraries that describe the exports from one
> image for use by another, typically follow the layout described in section 7,
> "Archive (Library) File Format." The primary difference is that import library members
> contain pseudo-object files instead of real ones, in which each member includes the
> section contributions that are required to build the import tables that are described
> in section 6.4, "The .idata Section." The linker generates this archive while building
> the exporting application.
>
> The section contributions for an import can be inferred from a small set of
> information. The linker can either generate the complete, verbose information into the
> import library for each member at the time of the library's creation or write only the
> canonical information to the library and let the application that later uses it
> generate the necessary data on the fly.
>
> In an import library with the **long format**, a single member contains the following
> information:
> Archive member header / File header / Section headers / Data that corresponds to each
> of the section headers / COFF symbol table / Strings
>
> In contrast, a **short import library** is written as follows:
> Archive member header / Import header / Null-terminated import name string /
> Null-terminated DLL name string

*The long format is what VC++ 2.x–5.x wrote, what VC6 still wrote with
`/LINK50COMPAT`, and what `alink`/`aimplib` use. The short format (import header
`Sig1=0, Sig2=0xFFFF`) is VC6+ and `alink` does not read it.*

### The mechanism that ties the members together (section 5.2, relocations)

> If the symbol referred to by the SymbolTableIndex field has the storage class
> IMAGE_SYM_CLASS_SECTION, the symbol's address is the beginning of the section. The
> section is usually in the same file, except when the object file is part of an archive
> (library). In that case, the section can be found in any other object file in the
> archive that has the same archive-member name as the current object file. (The
> relationship with the archive-member name is used in the linking of import tables,
> that is, the .idata section.)

*This is why every member of an import library carries the same member name
(`KERNEL32.dll/`, `ot4xb.dll/`) and why the descriptor member relocates its
`.idata$2` entry against **undefined** section symbols `.idata$4` and `.idata$5`
(storage class 104, value = section flags): they resolve to the start of the ILT/IAT
built from all the members with that name.*

### 4.2 Grouped Sections (Object Only)

> The "$" character (dollar sign) has a special interpretation in section names in object
> files. When determining the image section that will contain the contents of an object
> section, the linker discards the "$" and all characters that follow it. [...] However,
> the characters following the "$" determine the ordering of the contributions to the
> image section. All contributions with the same object-section name are allocated
> contiguously in the image, and the blocks of contributions are sorted in lexical order
> by object-section name.

*Hence the import tables are assembled by name ordering:*

| section | content (per DLL / per import) | provided by |
|---|---|---|
| `.idata$2` | one 20-byte import directory entry per DLL | descriptor member (`__IMPORT_DESCRIPTOR_<dll>`) |
| `.idata$3` | the null directory entry that terminates the table | `__NULL_IMPORT_DESCRIPTOR` member |
| `.idata$4` | import lookup table: one 4-byte entry per import + 4-byte null | symbol members (associative COMDAT) + `\x7f<dll>_NULL_THUNK_DATA` member |
| `.idata$5` | import address table: same layout; the `__imp_<sym>` symbol lives here | symbol members (COMDAT `__imp_<sym>`) + null thunk member |
| `.idata$6` | DLL name (descriptor member) and hint/name entries (2-byte hint + name + NUL, even padded) | descriptor + symbol members |
| `.text` | the thunk `<sym>: jmp dword ptr [__imp_<sym>]` (FF 25 + DIR32 reloc), COMDAT | symbol members |

*COMDAT selection values used: 1 (`IMAGE_COMDAT_SELECT_NODUPLICATES`) for `.text` and
`.idata$5`, 5 (`IMAGE_COMDAT_SELECT_ASSOCIATIVE`, number = section index of `.idata$5`)
for `.idata$4` and `.idata$6` — exactly what KERNEL32.LIB (1995) shows. The descriptor
symbol names `__IMPORT_DESCRIPTOR_<dll>`, `__NULL_IMPORT_DESCRIPTOR`, `\x7f<dll>_NULL_THUNK_DATA`
are MS LINK conventions, not in the spec; Alaska's `aimplib` uses `<dll>_IMPORT_DESCRIPTOR`,
`NULL_IMPORT_DESCRIPTOR`, `<dll>_NULL_THUNK_DATA` (no prefix) — both work with `alink`
because only their own members reference them.*

### 6.4 The .idata Section (image side, for reference)

Import directory entry (20 bytes): Import Lookup Table RVA (0), Time/Date Stamp (4),
Forwarder Chain (8), Name RVA (12), Import Address Table RVA (16) — the three
relocations of the descriptor member patch offsets 0, 12 and 16 (DIR32NB = RVA).
