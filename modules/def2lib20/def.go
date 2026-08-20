// def.go is the producer side of def2lib20: it parses a module definition
// file (.def) into a Def, the structure coff.go consumes.
//
// It reads the file line by line with github.com/pablo-botella/linereader
// (CR, LF and CRLF terminators alike), treats the content as bytes (ANSI,
// no transcoding), understands ';' comments, the LIBRARY statement and the
// EXPORTS section:
//
//	entryname[=internalname] [@ordinal [NONAME]] [DATA|CONSTANT] [PRIVATE]
//
// Other statements (NAME, DESCRIPTION, STACKSIZE, HEAPSIZE, VERSION,
// SECTIONS, ...) are recognised and skipped. Lines the parser does not
// understand are reported as errors with their line number.

package def2lib20

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/linereader"
)

// Export is one entry of the EXPORTS section.
type Export struct {
	// Name is the exported name: the name the DLL exports and the name the
	// importing side refers to (a consumer may decorate it, e.g. with "_").
	Name string
	// Internal is the optional internal name after '=' (informative for an
	// import library: the DLL export table only knows Name).
	Internal string
	// Ordinal is the optional @ordinal; 0 when absent.
	Ordinal int
	// NoName: NONAME, export by ordinal only.
	NoName bool
	// Data: DATA or CONSTANT, a data export (no thunk).
	Data bool
	// Private: PRIVATE, exported by the DLL but excluded from import libraries.
	Private bool
	// Line is the 1-based line number in the .def file.
	Line int
}

// Def is a parsed module definition file.
type Def struct {
	// Library is the LIBRARY name as written (may be empty, may carry an
	// extension). ParseFile fills it from the file name when absent.
	Library string
	// Exports are the EXPORTS entries in file order, duplicates and PRIVATE
	// entries included; use Imports for the effective import list.
	Exports []Export
}

// Imports returns the exports an import library must contain — PRIVATE
// entries removed and duplicates (same Name, case-sensitive) collapsed to
// their first occurrence — plus the duplicates that were dropped.
func (f *Def) Imports() (list []Export, dups []Export) {
	seen := make(map[string]bool, len(f.Exports))
	for _, e := range f.Exports {
		if e.Private {
			continue
		}
		if seen[e.Name] {
			dups = append(dups, e)
			continue
		}
		seen[e.Name] = true
		list = append(list, e)
	}
	return list, dups
}

// ParseDefFile reads and parses a .def file. When the file has no LIBRARY
// statement, Library is set to the file name without extension.
func ParseDefFile(path string) (*Def, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	f, err := ParseDef(fh)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if f.Library == "" {
		base := filepath.Base(path)
		f.Library = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return f, nil
}

// ParseDef parses a .def from r.
func ParseDef(r io.Reader) (*Def, error) {
	lr := linereader.NewLineReader(r, 0, 0)
	f := &Def{}
	inExports := false
	ln := 0
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		ln++
		line := string(raw)
		if i := strings.IndexByte(line, ';'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		switch strings.ToUpper(fields[0]) {
		case "LIBRARY":
			if len(fields) > 1 {
				f.Library = fields[1]
			}
			inExports = false
			continue
		case "EXPORTS":
			inExports = true
			if len(fields) == 1 {
				continue
			}
			line = strings.TrimSpace(line[len(fields[0]):])
		case "NAME", "DESCRIPTION", "STACKSIZE", "HEAPSIZE", "VERSION", "SECTIONS", "SEGMENTS", "CODE", "DATA", "STUB", "EXETYPE", "IMPORTS":
			inExports = false
			continue
		}
		if !inExports {
			continue
		}
		e, err := parseExport(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", ln, err)
		}
		e.Line = ln
		f.Exports = append(f.Exports, e)
	}
	return f, nil
}

// parseExport parses "entryname[=internalname] [@ordinal [NONAME]] [DATA] [PRIVATE]".
// Blanks around '=' are tolerated ("NAME =  _NAME" as written by xbmac2h).
func parseExport(line string) (Export, error) {
	var e Export
	for strings.Contains(line, " =") || strings.Contains(line, "= ") {
		line = strings.ReplaceAll(line, " =", "=")
		line = strings.ReplaceAll(line, "= ", "=")
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return e, fmt.Errorf("empty export")
	}
	name := fields[0]
	if i := strings.IndexByte(name, '='); i >= 0 {
		e.Internal = name[i+1:]
		name = name[:i]
	}
	if name == "" {
		return e, fmt.Errorf("missing export name in %q", line)
	}
	e.Name = name
	for _, t := range fields[1:] {
		switch {
		case strings.HasPrefix(t, "@"):
			if _, err := fmt.Sscanf(t, "@%d", &e.Ordinal); err != nil || e.Ordinal <= 0 {
				return e, fmt.Errorf("bad ordinal %q", t)
			}
		case strings.EqualFold(t, "NONAME"):
			e.NoName = true
		case strings.EqualFold(t, "DATA"), strings.EqualFold(t, "CONSTANT"):
			e.Data = true
		case strings.EqualFold(t, "PRIVATE"):
			e.Private = true
		default:
			return e, fmt.Errorf("unknown export attribute %q", t)
		}
	}
	return e, nil
}
