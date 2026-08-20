package cbk2obj

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildFile(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile("testdata/sample.cbk")
	if err != nil {
		t.Fatal(err)
	}
	cbk := filepath.Join(dir, "cb.cbk")
	if err := os.WriteFile(cbk, src, 0o644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	os.Chtimes(cbk, mtime, mtime)

	res, err := BuildFile(cbk, "", Options{Asm: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Obj != filepath.Join(dir, "cb.obj") || res.Asm != filepath.Join(dir, "cb.asm") || res.Callbacks != 6 || len(res.Diags) != 0 {
		t.Errorf("result %+v", res)
	}
	obj, err := os.ReadFile(res.Obj)
	if err != nil {
		t.Fatal(err)
	}
	if ts := binary.LittleEndian.Uint32(obj[4:]); ts != uint32(mtime.Unix()) {
		t.Errorf("timestamp %d, want %d", ts, mtime.Unix())
	}
	asm, err := os.ReadFile(res.Asm)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile("testdata/sample.asm")
	diffText(t, "asm file", asm, want)

	// explicit object path, no asm
	out := filepath.Join(dir, "sub", "x.obj")
	os.MkdirAll(filepath.Dir(out), 0o755)
	res, err = BuildFile(cbk, out, Options{Timestamp: 7})
	if err != nil || res.Obj != out || res.Asm != "" {
		t.Errorf("%v %+v", err, res)
	}
	if obj, _ := os.ReadFile(out); binary.LittleEndian.Uint32(obj[4:]) != 7 {
		t.Error("explicit timestamp not written")
	}

	// script errors: diags, no object
	bad := filepath.Join(dir, "bad.cbk")
	os.WriteFile(bad, []byte("CALLBACK WNDPROC A\r\nPARAM DWORD\r\nFOO\r\n"), 0o644)
	res, err = BuildFile(bad, "", Options{})
	if !errors.Is(err, ErrScript) || len(res.Diags) != 2 || res.Diags[0].Line != 2 || res.Diags[1].Msg != "unknow command" {
		t.Errorf("bad script: %v %+v", err, res)
	}
	if err.Error() != "cbk2obj: 2 script errors. No code generated" {
		t.Errorf("error text %q", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "bad.obj")); statErr == nil {
		t.Error("object written despite errors")
	}

	// no callbacks
	empty := filepath.Join(dir, "empty.cbk")
	os.WriteFile(empty, []byte("// nothing\r\nUSING CDECL\r\n"), 0o644)
	if _, err := BuildFile(empty, "", Options{}); !errors.Is(err, ErrNoCallbacks) {
		t.Errorf("empty: %v", err)
	}

	if _, err := BuildFile(filepath.Join(dir, "missing.cbk"), "", Options{}); err == nil {
		t.Error("missing file accepted")
	}
}
