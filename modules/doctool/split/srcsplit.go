// Package srcsplit splits an authoring source that carries /*{{ ... }}*/
// documentation blocks into its two projections: the CODE projection (the
// source with every doc block removed - what ships) and the DOC projection
// (only the doc blocks, verbatim, in order - what the documentation pipeline
// reads). The ot4xb .chsrc -> .ch + .chdoc split is the first use; nothing in
// the package knows about .ch: destinations are named by the caller, with a
// '*' standing for the source's base name.
//
// Lines are read through linereader (CR, LF and CRLF alike) and the outputs are
// always CRLF; bytes are Windows-1252 and pass through untouched unless the
// source declares an /*{{encoding: utf-8}}*/ directive, in which case both
// projections are transcoded to 1252. Writes are idempotent (identical bytes
// are not rewritten) and never silently replace a different existing file:
// that needs Bak (keep <dst>.bak) or Force.
package split

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/linereader"
)

// Options controls a run. At least one of Code / Doc must be set. A
// destination is a path, or a pattern whose every '*' is replaced by the
// source's base name (name without extension); a destination without '*' is
// one file and is only valid for a single source.
type Options struct {
	Code  string // destination of the code projection (empty = do not produce it)
	Doc   string // destination of the doc projection (empty = do not produce it)
	Bak   bool   // an existing, different destination is copied to <dst>.bak before being overwritten
	Force bool   // an existing, different destination is overwritten with no copy
	Check bool   // compare against the existing destinations and report drift instead of writing
	Warn  func(string)
}

// Result is what a run produced for one source file.
type Result struct {
	Src        string // the source read
	CodeDst    string // the code projection path written/compared (empty if not requested)
	DocDst     string // the doc projection path written/compared (empty if not requested)
	DocLines   int    // number of doc-block lines the source carried
	CodeDrift  bool   // Check: the code projection differs from the existing file
	DocDrift   bool   // Check: the doc projection differs from the existing file
	CodeWrote  bool   // the code projection was actually written (skipped when already identical)
	DocWrote   bool   // the doc projection was actually written (skipped when already identical)
	CodeBacked bool   // the previous code projection was kept as <dst>.bak
	DocBacked  bool   // the previous doc projection was kept as <dst>.bak
}

// cp1252High maps bytes 0x80-0x9F to their Windows-1252 code points (the rest
// of the high half coincides with Latin-1); the same table scandoc uses.
var cp1252High = [32]rune{
	0x20AC, 0x0081, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x008D, 0x017D, 0x008F,
	0x0090, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x009D, 0x017E, 0x0178,
}

// rune2cp1252 is the reverse of cp1252High: the 0x80-0x9F code points.
var rune2cp1252 = func() map[rune]byte {
	m := make(map[rune]byte, 32)
	for i, r := range cp1252High {
		m[r] = byte(0x80 + i)
	}
	return m
}()

// encode1252 encodes a UTF-8 string to Windows-1252 bytes. A rune that has no
// Windows-1252 representation is a hard error (it should never appear in doc
// content). ASCII and Latin-1 (0xA0-0xFF) map to themselves; the 0x80-0x9F
// specials go through the reverse table.
func encode1252(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x80 || (r >= 0xA0 && r <= 0xFF):
			out = append(out, byte(r))
		default:
			b, ok := rune2cp1252[r]
			if !ok {
				return nil, fmt.Errorf("character %q (U+%04X) has no Windows-1252 mapping", r, r)
			}
			out = append(out, b)
		}
	}
	return out, nil
}

// detectEncoding scans for an /*{{encoding: <name>}}*/ directive in the source
// and returns its lowercased value (e.g. "utf-8"), or "" if none. The marker
// is a normal /*{{ }}*/ block so Strip removes it from the .ch and Extract
// skips it (isEncodingMarker).
func detectEncoding(data []byte) string {
	lines, _, _ := splitLines(data)
	for _, ln := range lines {
		if v, ok := encodingMarker(ln); ok {
			return v
		}
	}
	return ""
}

// splitLines reads src line by line with linereader, so CR, LF and CRLF are
// all line ends (as everywhere else in the tool) and a stray LF or CR can never
// glue two lines together. Each line comes back without its terminator (a
// private copy). endsWithEol reports whether the last line had a terminator, so
// a caller re-joining with CRLF can restore the trailing one faithfully;
// nonCRLF counts the lines that did not end in CRLF - the .ch is an Xbase++
// source and must be CRLF, so outputs are always joined with CRLF whatever the
// input carried. maxLineSize is len(src)+1: no line can exceed the whole input,
// so "line too long" cannot happen and io.EOF is the only way the loop ends.
func splitLines(src []byte) (lines [][]byte, endsWithEol bool, nonCRLF int) {
	lr := linereader.NewLineReader(bytes.NewReader(src), 0, uint64(len(src))+1)
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			break
		}
		lines = append(lines, bytes.Clone(raw))
		switch lr.LastEolType {
		case linereader.EolCrLf:
			endsWithEol = true
		case linereader.EolLf, linereader.EolCr:
			endsWithEol = true
			nonCRLF++
		default: // EolEof: the last line carried no terminator
			endsWithEol = false
		}
	}
	return lines, endsWithEol, nonCRLF
}

// encodingMarker returns the value of an /*{{encoding: X}}*/ marker on line, if
// the line is one.
func encodingMarker(line []byte) (string, bool) {
	i := bytes.Index(line, []byte("/*{{encoding:"))
	if i < 0 {
		return "", false
	}
	rest := line[i+len("/*{{encoding:"):]
	if j := bytes.Index(rest, []byte("}}*/")); j >= 0 {
		return strings.ToLower(strings.TrimSpace(string(rest[:j]))), true
	}
	return "", false
}

// isDoc reports whether a line opens a doc block.
func isDoc(line []byte) bool { return bytes.Contains(line, []byte("/*{{")) }

// isDocEnd reports whether a line closes a doc block.
func isDocEnd(line []byte) bool { return bytes.Contains(line, []byte("}}*/")) }

// Strip returns src without its /*{{ ... }}*/ doc blocks (the .ch projection).
// A doc block occupies whole lines, from the "/*{{" line to the "}}*/" line
// (inclusive); ordinary /* */ comments (not opened with "/*{{") are kept.
// Lines are read with linereader (any of CR, LF, CRLF ends a line) and always
// re-joined with CRLF; the trailing CRLF is restored only when the source ended
// in a terminator, so a CRLF-clean source round-trips byte for byte.
func Strip(src []byte) (out []byte, docLines int) {
	lines, endsWithEol, _ := splitLines(src)
	kept := make([][]byte, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if isDoc(lines[i]) {
			for i < len(lines) && !isDocEnd(lines[i]) {
				i++
				docLines++
			}
			docLines++ // the closing line (or a single-line marker)
			continue
		}
		kept = append(kept, lines[i])
	}
	out = bytes.Join(kept, []byte("\r\n"))
	if endsWithEol && len(kept) > 0 {
		out = append(out, '\r', '\n')
	}
	return out, docLines
}

// Code is the clean projection of a source as bytes: Strip, then the
// transcoding its /*{{encoding: ...}}*/ directive asks for (utf-8 sources
// come out as Windows-1252). docLines is the number of lines removed. What
// `-code` writes and what a release artefact packs.
func Code(src []byte) (out []byte, docLines int, err error) {
	enc := detectEncoding(src)
	gen, n := Strip(src)
	switch enc {
	case "", "1252", "windows-1252", "cp1252", "ascii":
		return gen, n, nil
	case "utf-8", "utf8":
		out, err = encode1252(string(gen))
		return out, n, err
	}
	return nil, 0, fmt.Errorf("unsupported encoding %q (only utf-8 or 1252)", enc)
}

// Extract returns only the /*{{ ... }}*/ doc blocks of src (the .chdoc
// projection), in order, each block kept verbatim. Non-doc lines are dropped,
// except between begin-code and end-code: those source lines are what the
// pair documents, so the projection keeps them or the code block would be
// empty.
func Extract(src []byte) []byte {
	lines, _, _ := splitLines(src)
	kept := make([][]byte, 0, len(lines))
	code := false // inside begin-code ... end-code
	for i := 0; i < len(lines); i++ {
		if isDoc(lines[i]) {
			_, isEnc := encodingMarker(lines[i]) // the encoding directive is not doc
			var block [][]byte
			for i < len(lines) && !isDocEnd(lines[i]) {
				block = append(block, lines[i])
				i++
			}
			if i < len(lines) {
				block = append(block, lines[i]) // the closing line
			}
			if !isEnc {
				kept = append(kept, block...)
			}
			if len(block) > 0 {
				if bytes.Contains(block[0], []byte("{{begin-code")) {
					code = true
				} else if bytes.Contains(block[0], []byte("{{end-code")) {
					code = false
				}
			}
		} else if code {
			kept = append(kept, lines[i])
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return append(bytes.Join(kept, []byte("\r\n")), '\r', '\n')
}

// destPath builds <dir>/<base-without-.chsrc><ext>.
// expandDst turns a destination pattern into the path for srcPath: every '*'
// becomes the source's base name (no extension); a pattern without '*' is
// returned as is.
func expandDst(pattern, srcPath string) string {
	name := filepath.Base(srcPath)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	// "*.*" keeps the source's own extension (mixed sources into one folder)
	pattern = strings.ReplaceAll(pattern, "*.*", name)
	return filepath.Clean(strings.ReplaceAll(pattern, "*", base))
}

// writeOrCheck compares gen against the existing dst and either reports the
// difference (check mode) or writes gen only when it actually differs
// (idempotent: an unchanged output is left untouched, so its mtime does not
// move and downstream builds are not disturbed).
// writeOut writes a projection under the overwrite policy: an identical
// destination is left alone (idempotent, not an overwrite); a different
// existing one is kept as <dst>.bak with Bak, replaced outright with Force,
// and refused otherwise. With Check nothing is written and diff reports
// whether the destination would change.
func writeOut(dst string, gen []byte, o Options) (diff, wrote, backed bool, err error) {
	old, rerr := os.ReadFile(dst)
	exists := rerr == nil
	if rerr != nil && !os.IsNotExist(rerr) {
		return false, false, false, rerr
	}
	same := exists && bytes.Equal(old, gen)
	if o.Check {
		return !same, false, false, nil // missing or differing output would change
	}
	if same {
		return false, false, false, nil // idempotent: nothing to write
	}
	if !exists {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return true, false, false, err
		}
	}
	if exists {
		switch {
		case o.Bak:
			if err := os.WriteFile(dst+".bak", old, 0666); err != nil {
				return true, false, false, fmt.Errorf("%s: cannot keep the previous version: %w", dst, err)
			}
			backed = true
		case o.Force:
		default:
			return true, false, false, fmt.Errorf("%s exists with different content: use -bak to keep a copy or -force to overwrite", dst)
		}
	}
	return true, true, backed, os.WriteFile(dst, gen, 0666)
}

// SplitFile produces the requested projections for one .chsrc.
func SplitFile(srcPath string, o Options) (Result, error) {
	r := Result{Src: srcPath}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return r, err
	}
	// The .ch is an Xbase++ source and must be CRLF. Any input line ending is
	// accepted (the outputs are normalized to CRLF), but a source that was not
	// CRLF-clean is worth knowing about.
	if _, _, nonCRLF := splitLines(data); nonCRLF > 0 && o.Warn != nil {
		o.Warn(fmt.Sprintf("%s: %d line(s) not CRLF-terminated; outputs normalized to CRLF", srcPath, nonCRLF))
	}
	// An optional /*{{encoding: utf-8}}*/ directive makes chsplit transcode the
	// generated outputs to Windows-1252 (the .ch must be 1252 for Alaska/MSVC).
	enc := detectEncoding(data)
	utf8src := false
	switch enc {
	case "", "1252", "windows-1252", "cp1252", "ascii":
		// default: byte pass-through
	case "utf-8", "utf8":
		utf8src = true
	default:
		return r, fmt.Errorf("%s: unsupported encoding %q (only utf-8 or 1252)", srcPath, enc)
	}
	to1252 := func(gen []byte) ([]byte, error) {
		if !utf8src {
			return gen, nil
		}
		return encode1252(string(gen))
	}
	if o.Code != "" {
		gen, n, err := Code(data)
		if err != nil {
			return r, fmt.Errorf("%s -> code: %w", srcPath, err)
		}
		r.DocLines = n
		r.CodeDst = expandDst(o.Code, srcPath)
		if r.CodeDrift, r.CodeWrote, r.CodeBacked, err = writeOut(r.CodeDst, gen, o); err != nil {
			return r, err
		}
	}
	if o.Doc != "" {
		gen := Extract(data)
		if gen, err = to1252(gen); err != nil {
			return r, fmt.Errorf("%s -> doc: %w", srcPath, err)
		}
		r.DocDst = expandDst(o.Doc, srcPath)
		if r.DocDrift, r.DocWrote, r.DocBacked, err = writeOut(r.DocDst, gen, o); err != nil {
			return r, err
		}
	}
	return r, nil
}

// Sources resolves an -in argument (a single .chsrc file, a directory of them,
// or a glob mask like "ch/src/*.chsrc") to the list of .chsrc files.
func Sources(in string) ([]string, error) {
	if st, err := os.Stat(in); err == nil {
		if st.IsDir() {
			return globDir(in)
		}
		return []string{in}, nil // a plain file, whatever its extension
	}
	// not a plain path: treat as a glob mask
	matches, err := filepath.Glob(in)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && !st.IsDir() {
			out = append(out, m)
		}
	}
	return out, nil
}

// globDir returns every *.chsrc in dir (not recursive).
func globDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".ch", ".prg", ".chsrc", ".c", ".cpp", ".h", ".hpp": // the authoring sources
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}

// Run resolves in and splits every source. It returns the per-file results.
func Run(in string, o Options) ([]Result, error) {
	if o.Code == "" && o.Doc == "" {
		return nil, fmt.Errorf("nothing to do: give -code and/or -doc")
	}
	srcs, err := Sources(in)
	if err != nil {
		return nil, err
	}
	if len(srcs) == 0 && o.Warn != nil {
		o.Warn(fmt.Sprintf("no source files matched %q", in))
	}
	if len(srcs) > 1 {
		// several sources need a '*' in every destination, or they would all
		// land on the same file
		for _, d := range []string{o.Code, o.Doc} {
			if d != "" && !strings.Contains(d, "*") {
				return nil, fmt.Errorf("%d sources match %q but destination %q has no '*' for the source name", len(srcs), in, d)
			}
		}
	}
	var out []Result
	for _, s := range srcs {
		res, err := SplitFile(s, o)
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}
