// Package vbuild keeps a project's version number in one text file — the
// version script (<project>.VersionInfo) — and generates from it the files
// that must carry that number: the .rc VERSIONINFO block, version headers,
// a LICENSE with the current year, whatever the script declares.
//
// The script is a version header line, [Product]  Version:{a,b,c,d},
// followed by a body of free text with {$<...>$} macros and
// {$<File:>$[path]$} markers that split the body into output files. See
// _mkskill/src/vbuild.md for the full syntax. All parsing is FSM over
// bytes (no regular expressions); files are treated as ANSI bytes.
//
// Run reads the script, increments the version when asked (the default is
// no increment; a ":noinc" product never increments), rewrites the header
// line in place when the version changed, expands the macros and writes
// the output files.
package vbuild

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OutEol selects the line terminator of the generated files. The zero
// value is CRLF, the vbuild default.
type OutEol int

const (
	// OutCrLf ends every output line with CRLF (default, -eolrn).
	OutCrLf OutEol = iota
	// OutCr ends every output line with CR (-eolr).
	OutCr
	// OutLf ends every output line with LF (-eoln).
	OutLf
	// OutSave keeps, for every line, the terminator it had in the script
	// (-eols); a last line without terminator stays without it.
	OutSave
)

// Options controls Run.
type Options struct {
	// Inc selects the component to increment; NoInc (zero) regenerates the
	// outputs without touching the version.
	Inc Component
	// Eol selects the line terminator of the generated files: OutCrLf
	// (default), OutCr, OutLf, or OutSave — every line keeps the terminator
	// it had in the script.
	Eol OutEol
	// Now is the run's timestamp; zero means time.Now().
	Now time.Time
	// UUID forces the run's uuid (tests); empty generates one on first use.
	UUID string
	// Print, when not nil, receives the new header line when the version
	// changed (the CLI prints it unless -q).
	Print func(line string)
}

// Result reports what Run did.
type Result struct {
	Old, New Version
	// Changed is set when the version was incremented (header rewritten).
	Changed bool
	// Header is the header line after the run.
	Header string
	// Files lists the generated files in script order.
	Files []string
}

// Run processes the script at path.
func Run(path string, o Options) (*Result, error) {
	s, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	res := &Result{Old: s.Header.Version}
	inc := o.Inc
	if s.Header.NoInc {
		inc = NoInc
	}
	newVer, err := s.Header.Version.Inc(inc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	res.New = newVer
	res.Changed = newVer != s.Header.Version
	s.Header.Version = newVer
	res.Header = s.Header.Format()

	if res.Changed {
		if err := rewriteHeader(path, s); err != nil {
			return nil, err
		}
		if o.Print != nil {
			o.Print(res.Header)
		}
	}

	ctx := &Context{
		Version: newVer,
		Now:     o.Now,
		UUID:    o.UUID,
		Folder:  filepath.Base(filepath.Dir(abs(path))),
	}
	if ctx.Now.IsZero() {
		ctx.Now = time.Now()
	}
	files, err := split(s, ctx, filepath.Dir(path), o.Eol)
	if err != nil {
		return nil, err
	}
	res.Files = files
	return res, nil
}

// rewriteHeader writes the script back: the normalised header line (keeping
// the terminator it had) followed by the body lines byte-identical.
func rewriteHeader(path string, s *Script) error {
	var out bytes.Buffer
	out.WriteString(s.Header.Format())
	eol := s.HeaderEol
	if eol == EolNone && len(s.Body) > 0 {
		eol = EolCrLf
	}
	out.Write(eol.Bytes())
	for _, l := range s.Body {
		out.Write(l.Text)
		out.Write(l.Eol.Bytes())
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// split expands the body macros and writes the {$<File:>$[...]$} blocks.
// baseDir anchors relative paths (the folder of the script). outEol is the
// terminator of the generated lines; OutSave keeps each line's own.
func split(s *Script, ctx *Context, baseDir string, outEol OutEol) ([]string, error) {
	var fixed []byte
	switch outEol {
	case OutCrLf:
		fixed = []byte("\r\n")
	case OutCr:
		fixed = []byte("\r")
	case OutLf:
		fixed = []byte("\n")
	}
	var files []string
	var cur *bytes.Buffer
	var curPath string
	flush := func() error {
		if cur == nil {
			return nil
		}
		if err := os.WriteFile(curPath, cur.Bytes(), 0o644); err != nil {
			return fmt.Errorf("%s: %w", curPath, err)
		}
		files = append(files, curPath)
		cur = nil
		return nil
	}
	for _, line := range s.Body {
		expanded := Expand(line.Text, ctx)
		if marker, ok := fileMarker(expanded); ok {
			if err := flush(); err != nil {
				return nil, err
			}
			p := filepath.FromSlash(strings.TrimSpace(marker))
			if !filepath.IsAbs(p) {
				p = filepath.Join(baseDir, p)
			}
			curPath = p
			cur = &bytes.Buffer{}
			continue
		}
		if cur == nil {
			continue // prologue: lines before the first marker belong to no file
		}
		cur.Write(expanded)
		if outEol == OutSave {
			cur.Write(line.Eol.Bytes())
		} else {
			cur.Write(fixed)
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return files, nil
}

// fileMarker recognises a line holding only {$<File:>$[path]$} (blanks
// around allowed; "File:" any case) and returns the path.
func fileMarker(line []byte) (string, bool) {
	i, n := 0, len(line)
	for i < n && isBlank(line[i]) {
		i++
	}
	if !hasFold(line[i:], "{$<file:>$[") {
		return "", false
	}
	i += len("{$<file:>$[")
	start := i
	end := bytes.LastIndex(line, []byte("]$}"))
	if end < start {
		return "", false
	}
	for j := end + 3; j < n; j++ {
		if !isBlank(line[j]) {
			return "", false
		}
	}
	return string(line[start:end]), true
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
