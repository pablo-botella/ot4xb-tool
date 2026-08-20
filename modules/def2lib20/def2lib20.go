package def2lib20

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options controls how the import library is built.
type Options struct {
	// DLL is the DLL file name written into the library (import directory
	// name and archive member names). Default: the LIBRARY name of the .def,
	// with ".dll" appended when it has no extension.
	DLL string
	// Prefix is prepended to every export name to form the linker symbol
	// ("_" for cdecl C functions; empty for Xbase++ functions). The DLL export
	// name itself is never prefixed.
	Prefix string
	// Timestamp is written into every archive member header and COFF file
	// header (seconds since 1970). Zero means "use the .def modification
	// time" in BuildFile and "zero" in Build.
	Timestamp uint32
	// Monkey reproduces the conventions of Alaska's aimplib.exe instead
	// of Microsoft LINK's: the special symbols are named
	// "<base>_IMPORT_DESCRIPTOR", "NULL_IMPORT_DESCRIPTOR" and
	// "<base>_NULL_THUNK_DATA" (no "__" / 0x7F prefixes) and the hint of every
	// hint/name entry is 0xFFFF. Use it to link together with the Xbase++
	// runtime libraries (XppRt1.lib, XppSys.lib, ... all written by aimplib):
	// they share the same NULL_IMPORT_DESCRIPTOR symbol, so ALINK emits a
	// single import directory terminator and the executable comes out byte
	// for byte as with an aimplib library. With false, the MS names are used
	// (__IMPORT_DESCRIPTOR_<base>, __NULL_IMPORT_DESCRIPTOR,
	// \x7f<base>_NULL_THUNK_DATA, hint = ordinal or 0) as in the 1995 MS LINK
	// libraries.
	Monkey bool
	// Warn, when not nil, receives non-fatal notices (duplicate exports).
	Warn func(msg string)
}

// ErrUnsupported is wrapped by Build for .def features the long import
// format generator does not implement yet (NONAME / DATA exports).
var ErrUnsupported = errors.New("def2lib20: unsupported export attribute")

// Build returns the import library (a COFF archive) for def.
func Build(def *Def, o Options) ([]byte, error) {
	dll := o.DLL
	if dll == "" {
		dll = def.Library
		if dll == "" {
			return nil, errors.New("def2lib20: no LIBRARY name and no Options.DLL")
		}
		if filepath.Ext(dll) == "" {
			dll += ".dll"
		}
	}
	base := strings.TrimSuffix(dll, filepath.Ext(dll))
	nm := o.naming(base)

	list, dups := def.Imports()
	if o.Warn != nil {
		for _, d := range dups {
			o.Warn(fmt.Sprintf("line %d: duplicate export %s ignored", d.Line, d.Name))
		}
	}
	imports := make([]importEntry, 0, len(list))
	for _, e := range list {
		if e.NoName || e.Data {
			return nil, fmt.Errorf("%w: line %d: %s", ErrUnsupported, e.Line, e.Name)
		}
		hint := nm.hint
		if !o.Monkey && e.Ordinal > 0 && e.Ordinal <= 0xFFFF {
			hint = uint16(e.Ordinal)
		}
		imports = append(imports, importEntry{symbol: o.Prefix + e.Name, name: e.Name, hint: hint})
	}
	return buildLibrary(dll, nm, imports, o.Timestamp), nil
}

// BuildFile parses defPath and writes the import library to libPath
// (default: defPath with the extension replaced by .lib). It returns the
// number of imports written.
func BuildFile(defPath, libPath string, o Options) (int, error) {
	def, err := ParseDefFile(defPath)
	if err != nil {
		return 0, err
	}
	if o.Timestamp == 0 {
		if fi, err := os.Stat(defPath); err == nil {
			o.Timestamp = uint32(fi.ModTime().Unix())
		}
	}
	lib, err := Build(def, o)
	if err != nil {
		return 0, err
	}
	if libPath == "" {
		libPath = strings.TrimSuffix(defPath, filepath.Ext(defPath)) + ".lib"
	}
	if err := os.WriteFile(libPath, lib, 0o644); err != nil {
		return 0, err
	}
	list, _ := def.Imports()
	return len(list), nil
}

// naming holds the names of the three special symbols and the default hint.
type naming struct {
	descriptor     string
	nullDescriptor string
	nullThunk      string
	hint           uint16
}

func (o Options) naming(base string) naming {
	if o.Monkey {
		return naming{
			descriptor:     base + "_IMPORT_DESCRIPTOR",
			nullDescriptor: "NULL_IMPORT_DESCRIPTOR",
			nullThunk:      base + "_NULL_THUNK_DATA",
			hint:           0xFFFF,
		}
	}
	return naming{
		descriptor:     "__IMPORT_DESCRIPTOR_" + base,
		nullDescriptor: "__NULL_IMPORT_DESCRIPTOR",
		nullThunk:      "\x7f" + base + "_NULL_THUNK_DATA",
		hint:           0,
	}
}
