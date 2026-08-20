package vsxbt

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLenientJSON(t *testing.T) {
	in := "{\r\n // comment\r\n \"a\": [1, 2,], // tail\r\n \"b\": \"x//y\", \"c\": {\"d\": 1,},\r\n}"
	want := `{"a":[1,2],"b":"x//y","c":{"d":1}}`
	var v1, v2 any
	if err := jsonUnmarshal(lenientJSON([]byte(in)), &v1); err != nil {
		t.Fatalf("lenient parse: %v", err)
	}
	if err := jsonUnmarshal([]byte(want), &v2); err != nil {
		t.Fatal(err)
	}
	if fmtAny(v1) != fmtAny(v2) {
		t.Errorf("lenient = %v, want %v", v1, v2)
	}
}

func TestNormSlash(t *testing.T) {
	cases := map[string]string{
		`./patata//frita/`: "./patata/frita/",
		`.\patata\\frita`:  "./patata/frita",
		`C:\pli\ot4xb`:     "C:/pli/ot4xb",
		`//server/share/x`: "//server/share/x",
		`a/b`:              "a/b",
	}
	for in, want := range cases {
		if got := normSlash(in); got != want {
			t.Errorf("normSlash(%q) = %q, want %q", in, got, want)
		}
	}
}

// project builds a mini ot4xb-like tree with a tool file and returns
// (root, toolPath).
func project(t *testing.T, toolJSON string) (string, string) {
	t.Helper()
	root := t.TempDir()
	write(t, root, "source/ot4xb.VersionInfo",
		"[ot4xb.dll]  Version:{1,7,14,0}\r\n"+
			"\r\n"+
			"{$<File:>$[version.h]$}\r\n"+
			"#define V \"{$<FILEVERSION(000.000.000.000)>$}\"\r\n")
	write(t, root, "source/ot4xb.xbmac", "_XPP_REG_FUN_( APPINSTANCE )\r\n_XPP_REG_FUN_( OT4XB )\r\n")
	write(t, root, "source/notes.txt", "notes")
	write(t, root, "Release/ot4xb.dll", "dll-bytes")
	toolPath := write(t, root, "ot4xb.ot4xb-tool", toolJSON)
	return root, toolPath
}

const toolJSON = `{
	// the version script, relative to this file
	"versioninfo": "./source/ot4xb.VersionInfo",
	"pre.release": {
		"folders": {
			"src": "./source/",
		},
		"steps": [ "vbuild", "codegen" ],
		"vbuild": { "flags": "" },
		"codegen": {
			"cleanup.before": { "files": [ "<src>/Release/ot4xb.obj", "<src>/ot4xb.def" ] },
			"xbmac2h": { "src": "<src>//ot4xb.xbmac" },
		},
	},
	"pre.debug": { "template": "pre.release" },
	"post.release": {
		"vars": { "bo": "ot4xb_<v.maj:03>_<v.min:03>_<v.hbuild:03>_<v.lbuild:03>" },
		"folders": {
			"src": "./source",
			"dst": "./Release",
			"deploy": "<dst>/builds/_<bo>"
		},
		"steps": [ "def2lib20", "artefacts" ],
		"def2lib20": { "in": "<src>/ot4xb.def", "out": "<dst>/ot4xb.lib", "monkey": true },
		"artefacts": [
			{
				"type": "zip",
				"zip_filename": "<deploy>/_<bo>.zip",
				"content": [
					{ "in": [ "<dst>/*.dll", "<dst>/*.lib", "<src>/*.txt" ], "out": "/" }
				]
			}
		]
	}
}`

func TestRunPreRelease(t *testing.T) {
	root, toolPath := project(t, toolJSON)
	// a stale ot4xb.def that cleanup.before must delete before xbmac2h remakes it
	write(t, root, "source/ot4xb.def", "stale")
	tool, err := Load(toolPath)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	if err := tool.Run("pre.release", Options{Say: func(m string) { log = append(log, m) }}); err != nil {
		t.Fatal(err)
	}
	// vbuild generated version.h next to the script
	vh, err := os.ReadFile(filepath.Join(root, "source", "version.h"))
	if err != nil || !strings.Contains(string(vh), `"001.007.014.000"`) {
		t.Errorf("version.h = %q, %v", vh, err)
	}
	// xbmac2h regenerated the def (LIBRARY from the file base name)
	def, err := os.ReadFile(filepath.Join(root, "source", "ot4xb.def"))
	if err != nil || !strings.HasPrefix(string(def), "LIBRARY ot4xb\r\n") || !strings.Contains(string(def), "APPINSTANCE =  _APPINSTANCE") {
		t.Errorf("ot4xb.def = %q, %v", def, err)
	}
	// version script untouched (no -inc)
	vi, _ := os.ReadFile(filepath.Join(root, "source", "ot4xb.VersionInfo"))
	if !strings.HasPrefix(string(vi), "[ot4xb.dll]  Version:{1,7,14,0}") {
		t.Errorf("versioninfo touched: %q", vi[:40])
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "del: ") || !strings.Contains(joined, "xbmac2h: ") {
		t.Errorf("log = %v", log)
	}
	// the template alias runs the same
	if err := tool.Run("pre.debug", Options{}); err != nil {
		t.Errorf("pre.debug: %v", err)
	}
}

func TestRunPostRelease(t *testing.T) {
	root, toolPath := project(t, toolJSON)
	tool, err := Load(toolPath)
	if err != nil {
		t.Fatal(err)
	}
	// pre.release first so ot4xb.def exists
	if err := tool.Run("pre.release", Options{}); err != nil {
		t.Fatal(err)
	}
	if err := tool.Run("post.release", Options{}); err != nil {
		t.Fatal(err)
	}
	// the lib landed in Release
	if _, err := os.Stat(filepath.Join(root, "Release", "ot4xb.lib")); err != nil {
		t.Fatalf("ot4xb.lib missing: %v", err)
	}
	// the zip landed in Release/builds/_ot4xb_001_007_014_000/
	zp := filepath.Join(root, "Release", "builds", "_ot4xb_001_007_014_000", "_ot4xb_001_007_014_000.zip")
	zr, err := zip.OpenReader(zp)
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	got := strings.Join(names, "|")
	for _, want := range []string{"ot4xb.dll", "ot4xb.lib", "notes.txt"} {
		if !strings.Contains(got, want) {
			t.Errorf("zip misses %s: %v", want, names)
		}
	}
}

func TestUserCompanion(t *testing.T) {
	root, toolPath := project(t, toolJSON)
	pli := filepath.Join(root, "pli") // exists -> create:false copies
	os.MkdirAll(pli, 0o755)
	write(t, root, "ot4xb.ot4xb-tool.user", `{
		"post.release": {
			"folders": { "pli": "`+strings.ReplaceAll(pli, "\\", "/")+`" },
			"steps": [ "deploy" ],
			"deploy": {
				"copy": [
					{ "in": [ "<dst>/ot4xb.dll" ], "out": "<pli>", "create": false },
					{ "in": [ "<dst>/ot4xb.dll" ], "out": "<pli>/cppinc", "create": false }
				]
			}
		},
		"post.debug": { "template": "post.release" }
	}`)
	tool, err := Load(toolPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := tool.Run("pre.release", Options{}); err != nil {
		t.Fatal(err)
	}
	var log []string
	if err := tool.Run("post.release", Options{Say: func(m string) { log = append(log, m) }}); err != nil {
		t.Fatal(err)
	}
	// the .user deploy copied the dll (folder exists) and skipped cppinc (create:false, missing)
	if _, err := os.Stat(filepath.Join(pli, "ot4xb.dll")); err != nil {
		t.Errorf("dll not deployed to pli: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pli, "cppinc")); !os.IsNotExist(err) {
		t.Errorf("cppinc must not be created")
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "user: ") || !strings.Contains(joined, "skipped") {
		t.Errorf("log = %v", log)
	}
}

func TestErrors(t *testing.T) {
	_, toolPath := project(t, toolJSON)
	tool, err := Load(toolPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := tool.Run("nope", Options{}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown entry: %v", err)
	}
	// unknown macro is loud
	_, tp2 := project(t, `{"e": {"steps":["s"], "s": {"cleanup.before": {"files": ["<nope>/x"]}}}}`)
	tool2, err := Load(tp2)
	if err != nil {
		t.Fatal(err)
	}
	if err := tool2.Run("e", Options{}); err == nil || !strings.Contains(err.Error(), "unknown macro") {
		t.Errorf("unknown macro: %v", err)
	}
	// template cycle
	_, tp3 := project(t, `{"a": {"template": "b"}, "b": {"template": "a"}}`)
	tool3, err := Load(tp3)
	if err != nil {
		t.Fatal(err)
	}
	if err := tool3.Run("a", Options{}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("template cycle: %v", err)
	}
}

func TestVersionMacroWidths(t *testing.T) {
	_, toolPath := project(t, toolJSON)
	tool, err := Load(toolPath)
	if err != nil {
		t.Fatal(err)
	}
	e := &entry{names: map[string]string{}}
	for spec, want := range map[string]string{
		"<v.maj>":       "1",
		"<v.min:03>":    "007",
		"<v.hbuild:3>":  " 14",
		"<v.build>":     "3584",
		"<v.lbuild:02>": "00",
	} {
		got, err := tool.expand(e, spec)
		if err != nil || got != want {
			t.Errorf("expand(%q) = %q, %v; want %q", spec, got, err, want)
		}
	}
	if _, err := tool.expand(e, "<v.patch>"); err == nil {
		t.Errorf("unknown component must fail")
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
func fmtAny(v any) string                 { return fmt.Sprintf("%v", v) }
