package cbk2obj

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options controls Build and BuildFile.
type Options struct {
	// Timestamp is written into the COFF file header (seconds since 1970).
	// Zero means "the .cbk modification time" in BuildFile and zero in Build.
	Timestamp uint32
	// Asm makes BuildFile also write the FASM source next to the object.
	Asm bool
}

// ErrScript is wrapped by BuildFile when the script has errors; the Result
// carries them.
var ErrScript = errors.New("script errors")

// ErrNoCallbacks is returned when the script defines no callback.
var ErrNoCallbacks = errors.New("cbk2obj: no callbacks to compile")

// Build compiles s and returns the COFF object and the equivalent FASM
// source.
func Build(s *Script, o Options) (obj, asm []byte, err error) {
	if len(s.Callbacks) == 0 {
		return nil, nil, ErrNoCallbacks
	}
	u := generate(s, genOptions{})
	var ab bytes.Buffer
	if err := writeAsm(&ab, u); err != nil {
		return nil, nil, err
	}
	ob, err := object(u, o.Timestamp)
	if err != nil {
		return nil, nil, err
	}
	return ob, ab.Bytes(), nil
}

// Result reports what BuildFile did.
type Result struct {
	Obj       string // object written
	Asm       string // FASM source written ("" unless Options.Asm)
	Callbacks int    // callbacks compiled
	Diags     []Diag // script errors (with ErrScript)
}

// BuildFile parses cbkPath and writes the object to objPath (default: the
// .cbk path with the extension replaced by .obj); with o.Asm the FASM
// source goes next to the object with the same base name. Script errors
// come back in Result.Diags with an error wrapping ErrScript.
func BuildFile(cbkPath, objPath string, o Options) (Result, error) {
	s, diags, err := ParseFile(cbkPath)
	if err != nil {
		return Result{}, err
	}
	if len(diags) > 0 {
		return Result{Diags: diags}, fmt.Errorf("cbk2obj: %d %w. No code generated", len(diags), ErrScript)
	}
	if o.Timestamp == 0 {
		if fi, err := os.Stat(cbkPath); err == nil {
			o.Timestamp = uint32(fi.ModTime().Unix())
		}
	}
	obj, asm, err := Build(s, o)
	if err != nil {
		return Result{}, err
	}
	if objPath == "" {
		objPath = strings.TrimSuffix(cbkPath, filepath.Ext(cbkPath)) + ".obj"
	}
	if err := os.WriteFile(objPath, obj, 0o644); err != nil {
		return Result{}, err
	}
	res := Result{Obj: objPath, Callbacks: len(s.Callbacks)}
	if o.Asm {
		res.Asm = strings.TrimSuffix(objPath, filepath.Ext(objPath)) + ".asm"
		if err := os.WriteFile(res.Asm, asm, 0o644); err != nil {
			return res, err
		}
	}
	return res, nil
}
