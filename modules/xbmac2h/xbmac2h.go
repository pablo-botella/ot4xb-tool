// Package xbmac2h generates, from a .xbmac registration list, the four files
// the legacy xbmac2h tool wrote next to the list. xbmac.go is the producer
// (.xbmac → File, read with linereader); this file is the consumer:
//
//	<base>_xbexports.hpp   XPPRET XPPENTRY <sym>(XppParamList ); inside extern "C"
//	<base>_xbfunclist.hpp  {"NAME",<sym>} initialiser rows
//	<base>Cpp.def          LIBRARY/EXPORTS with "NAME =  <sym>  PRIVATE" (MSVC side)
//	<base>.def             LIBRARY/EXPORTS with "NAME =  _<sym>" (Xbase++ side, input of def2lib20)
//
// A _CDECL_EXPORT_( name ) line (a plain C function the DLL already exports
// from its source, wanted by Xbase++ clients through the def2lib20 import
// library as well) is not an Xbase++ function: the two .hpp files get only
// a comment, <base>Cpp.def gets nothing (the function is exported on its
// own; a PRIVATE entry would drop it from the MSVC import library) and
// <base>.def gets "name =  _name" (export name, MSVC-decorated symbol).
//
// Formats are byte-compatible with the Harbour xbmac2h (CRLF, same blanks),
// except that comments of the .xbmac are discarded instead of copied.
// Blank lines are kept; unrecognised lines are reported through
// Options.Warn and marked in the outputs exactly like the legacy tool did.
package xbmac2h

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const crlf = "\r\n"

// Options controls Generate.
type Options struct {
	// LibName is the LIBRARY name written into the .def files. Default: the
	// .xbmac file name without extension, as written (the legacy tool
	// upper-cased it only when the file was given with a path).
	LibName string
	// Warn, when not nil, receives one notice per unrecognised line.
	Warn func(msg string)
}

// OutputPaths returns the four output paths for a .xbmac path:
// <base>_xbexports.hpp, <base>_xbfunclist.hpp, <base>Cpp.def, <base>.def.
func OutputPaths(xbmacPath string) (exports, funclist, cppdef, def string) {
	base := strings.TrimSuffix(xbmacPath, filepath.Ext(xbmacPath))
	return base + "_xbexports.hpp", base + "_xbfunclist.hpp", base + "Cpp.def", base + ".def"
}

// Generate parses xbmacPath and writes the four files next to it. It
// returns the paths written.
func Generate(xbmacPath string, o Options) ([]string, error) {
	f, err := ParseFile(xbmacPath)
	if err != nil {
		return nil, err
	}
	if o.Warn != nil {
		for _, l := range f.Unknowns() {
			o.Warn(fmt.Sprintf("%s:%d: unknown line: %s", filepath.Base(xbmacPath), l.N, strings.TrimRight(l.Text, " \t")))
		}
	}
	lib := o.LibName
	if lib == "" {
		base := filepath.Base(xbmacPath)
		lib = strings.TrimSuffix(base, filepath.Ext(base))
	}
	pExports, pFuncList, pCppDef, pDef := OutputPaths(xbmacPath)
	outs := []struct {
		path  string
		write func(w io.Writer) error
	}{
		{pExports, func(w io.Writer) error { return WriteExports(w, f) }},
		{pFuncList, func(w io.Writer) error { return WriteFuncList(w, f) }},
		{pCppDef, func(w io.Writer) error { return WriteCppDef(w, f, lib) }},
		{pDef, func(w io.Writer) error { return WriteDef(w, f, lib) }},
	}
	var written []string
	for _, out := range outs {
		if err := writeFile(out.path, out.write); err != nil {
			return written, err
		}
		written = append(written, out.path)
	}
	return written, nil
}

func writeFile(path string, write func(w io.Writer) error) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(fh)
	if err := write(bw); err != nil {
		fh.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		fh.Close()
		return err
	}
	return fh.Close()
}

// ---------------------------------------------------------------- generators

const rule = "// ---------------------------------------------------------------------------" // "// " + 75 dashes

func unknownHpp(l Line) string {
	return fmt.Sprintf("//////////  UNKNOW LINE #%d>>>%s<<<", l.N, strings.TrimRight(l.Text, " \t"))
}

func unknownDef(l Line) string {
	return fmt.Sprintf(";;;;;;;;;;  UNKNOW LINE #%d>>>%s<<<", l.N, strings.TrimRight(l.Text, " \t"))
}

// cdeclHpp is the trace a _CDECL_EXPORT_ line leaves in the .hpp files:
// the function is declared in its own C header, not here.
func cdeclHpp(l Line) string { return "// _CDECL_EXPORT_( " + l.Name + " )" }

// WriteExports writes <base>_xbexports.hpp.
func WriteExports(w io.Writer, f *File) error {
	bw := bufio.NewWriter(w)
	head := rule + crlf + "#ifdef __cplusplus" + crlf + "extern \"C\" {" + crlf + "#endif" + crlf + rule + crlf
	bw.WriteString(head)
	for _, l := range f.Lines {
		switch {
		case l.Kind.IsCommand():
			bw.WriteString("XPPRET XPPENTRY " + l.Symbol() + "(XppParamList );" + crlf)
		case l.Kind == CdeclExport:
			bw.WriteString(cdeclHpp(l) + crlf)
		case l.Kind == Blank:
			bw.WriteString(crlf)
		case l.Kind == Unknown:
			bw.WriteString(unknownHpp(l) + crlf)
		}
	}
	bw.WriteString(rule + crlf + "#ifdef __cplusplus" + crlf + "}" + crlf + "#endif" + crlf + rule + crlf)
	return bw.Flush()
}

// WriteFuncList writes <base>_xbfunclist.hpp.
func WriteFuncList(w io.Writer, f *File) error {
	bw := bufio.NewWriter(w)
	first := true
	for _, l := range f.Lines {
		switch {
		case l.Kind.IsCommand():
			lead := "   ,    "
			if first {
				lead = "        "
				first = false
			}
			bw.WriteString(lead + "{\"" + l.Name + "\"," + l.Symbol() + "}" + crlf)
		case l.Kind == CdeclExport:
			bw.WriteString(cdeclHpp(l) + crlf)
		case l.Kind == Blank:
			bw.WriteString(crlf)
		case l.Kind == Unknown:
			bw.WriteString(unknownHpp(l) + crlf)
		}
	}
	return bw.Flush()
}

// WriteCppDef writes <base>Cpp.def (MSVC side: PRIVATE exports; the
// _CDECL_EXPORT_ functions are left out, they are exported from the source).
func WriteCppDef(w io.Writer, f *File, lib string) error {
	bw := bufio.NewWriter(w)
	bw.WriteString("LIBRARY " + lib + crlf + "EXPORTS" + crlf)
	for _, l := range f.Lines {
		switch {
		case l.Kind.IsCommand():
			if l.Kind.Prefix() == "" {
				bw.WriteString("     " + l.Name + "  PRIVATE" + crlf)
			} else {
				bw.WriteString("     " + l.Name + " =  " + l.Symbol() + "  PRIVATE" + crlf)
			}
		case l.Kind == Blank:
			bw.WriteString(crlf)
		case l.Kind == Unknown:
			bw.WriteString(unknownDef(l) + crlf)
		}
	}
	return bw.Flush()
}

// WriteDef writes <base>.def (Xbase++ side: export name and
// underscore-decorated symbol; the _CDECL_EXPORT_ functions follow the same
// mould, "name =  _name").
func WriteDef(w io.Writer, f *File, lib string) error {
	bw := bufio.NewWriter(w)
	bw.WriteString("LIBRARY " + lib + crlf + "EXPORTS" + crlf)
	for _, l := range f.Lines {
		switch {
		case l.Kind.IsCommand():
			bw.WriteString("     " + l.Name + " =  _" + l.Symbol() + crlf)
		case l.Kind == CdeclExport:
			bw.WriteString("     " + l.Name + " =  _" + l.Name + crlf)
		case l.Kind == Blank:
			bw.WriteString(crlf)
		case l.Kind == Unknown:
			bw.WriteString(unknownDef(l) + crlf)
		}
	}
	return bw.Flush()
}
