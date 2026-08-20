// Command ot4xb-tool is the single binary of the ot4xb tool set; each tool is
// a subcommand backed by a package under modules/.
//
//	ot4xb-tool [-q] [-tool file.ot4xb-tool] -bs entry            run a build-step entry
//	ot4xb-tool [-q] vbuild [-inc major|minor|hbuild|lbuild|build] [-eolrn|-eolr|-eoln|-eols] file.VersionInfo
//	ot4xb-tool [-q] xbmac2h [-lib NAME] file.xbmac
//	ot4xb-tool [-q] def2lib20 [-monkey] [-o out.lib] [-dll name.dll] [-prefix _] [-ts seconds] file.def
//	ot4xb-tool [-q] cbk2obj [-asm] [-o out.obj] [-ts seconds] file.cbk
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/cbk2obj"
	"github.com/pablo-botella/ot4xb-tool/modules/def2lib20"
	"github.com/pablo-botella/ot4xb-tool/modules/vbuild"
	"github.com/pablo-botella/ot4xb-tool/modules/vsxbt"
	"github.com/pablo-botella/ot4xb-tool/modules/xbmac2h"
)

// quiet is the global -q: suppress normal output (errors still go to stderr).
var quiet bool

// say prints unless -q.
func say(format string, a ...any) {
	if !quiet {
		fmt.Printf(format, a...)
	}
}

func main() {
	args := os.Args[1:]
	for len(args) > 0 && args[0] == "-q" {
		quiet = true
		args = args[1:]
	}
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	var err error
	switch args[0] {
	case "vbuild":
		err = runVbuild(args[1:])
	case "xbmac2h":
		err = runXbmac2h(args[1:])
	case "def2lib20":
		err = runDef2lib20(args[1:])
	case "cbk2obj":
		err = runCbk2obj(args[1:])
	case "-tool", "-bs":
		err = runBuildStep(args)
	case "-h", "--help", "help", "/?":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "ot4xb-tool: unknown command %q\n\n", args[0])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ot4xb-tool:", err)
		os.Exit(1)
	}
}

func runVbuild(args []string) error {
	fs := flag.NewFlagSet("vbuild", flag.ContinueOnError)
	inc := fs.String("inc", "", "component to increment: major, minor, hbuild, lbuild, build (default: none)")
	eolrn := fs.Bool("eolrn", false, "output lines end in CRLF (default)")
	eolr := fs.Bool("eolr", false, "output lines end in CR")
	eoln := fs.Bool("eoln", false, "output lines end in LF")
	eols := fs.Bool("eols", false, "output lines keep the terminator they had in the script")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ot4xb-tool [-q] vbuild [-inc major|minor|hbuild|lbuild|build] [-eolrn|-eolr|-eoln|-eols] file.VersionInfo")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("one version script expected")
	}
	o := vbuild.Options{
		Print: func(line string) { say("%s\n", line) },
	}
	if *inc != "" {
		c, err := vbuild.ParseComponent(*inc)
		if err != nil {
			return err
		}
		o.Inc = c
	}
	n := 0
	for _, b := range []bool{*eolrn, *eolr, *eoln, *eols} {
		if b {
			n++
		}
	}
	if n > 1 {
		return fmt.Errorf("choose one of -eolrn, -eolr, -eoln, -eols")
	}
	switch {
	case *eolr:
		o.Eol = vbuild.OutCr
	case *eoln:
		o.Eol = vbuild.OutLf
	case *eols:
		o.Eol = vbuild.OutSave
	default:
		o.Eol = vbuild.OutCrLf
	}
	res, err := vbuild.Run(fs.Arg(0), o)
	if err != nil {
		return err
	}
	for _, f := range res.Files {
		say("vbuild: %s\n", f)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ot4xb-tool [-q] <command> [options]")
	fmt.Fprintln(os.Stderr, "       ot4xb-tool [-q] [-tool file.ot4xb-tool] -bs entry")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  -bs entry   run a build-step entry of the project tool file (default: the single *.ot4xb-tool of the folder)")
	fmt.Fprintln(os.Stderr, "  vbuild      process a version script: increment the version and generate its files")
	fmt.Fprintln(os.Stderr, "  xbmac2h     generate <base>_xbexports.hpp, <base>_xbfunclist.hpp, <base>Cpp.def and <base>.def from a .xbmac list")
	fmt.Fprintln(os.Stderr, "  def2lib20   build an x86 COFF import library (.lib, long format, ALINK compatible) from a .def")
	fmt.Fprintln(os.Stderr, "  cbk2obj     compile an Xbase++ callback script (.cbk) into a linkable x86 COFF object (.obj)")
}

// runBuildStep handles "ot4xb-tool [-tool file] -bs entry" (flags in any
// order before/after each other).
func runBuildStep(args []string) error {
	var toolPath, entry string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-tool":
			if i+1 >= len(args) {
				return fmt.Errorf("-tool needs a file")
			}
			i++
			toolPath = args[i]
		case "-bs":
			if i+1 >= len(args) {
				return fmt.Errorf("-bs needs an entry name")
			}
			i++
			entry = args[i]
		default:
			return fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	if entry == "" {
		return fmt.Errorf("-bs entry missing")
	}
	if toolPath == "" {
		found, err := findToolFile(".")
		if err != nil {
			return err
		}
		toolPath = found
	}
	t, err := vsxbt.Load(toolPath)
	if err != nil {
		return err
	}
	say("%s -bs %s\n", toolPath, entry)
	return t.Run(entry, vsxbt.Options{
		Say:  func(m string) { say("  %s\n", m) },
		Warn: func(m string) { fmt.Fprintln(os.Stderr, "  warning:", m) },
	})
}

// findToolFile looks for the single *.ot4xb-tool of dir (the .user
// companions do not count).
func findToolFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".ot4xb-tool") {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no *.ot4xb-tool file here; use -tool")
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("several *.ot4xb-tool files here (%s); use -tool", strings.Join(found, ", "))
}

func runXbmac2h(args []string) error {
	fs := flag.NewFlagSet("xbmac2h", flag.ContinueOnError)
	lib := fs.String("lib", "", "LIBRARY name for the .def files (default: the file name without extension)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ot4xb-tool xbmac2h [-lib NAME] file.xbmac")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("one .xbmac file expected")
	}
	paths, err := xbmac2h.Generate(fs.Arg(0), xbmac2h.Options{
		LibName: *lib,
		Warn:    func(msg string) { fmt.Fprintln(os.Stderr, "xbmac2h: warning:", msg) },
	})
	if err != nil {
		return err
	}
	for _, p := range paths {
		say("xbmac2h: %s\n", p)
	}
	return nil
}

func runDef2lib20(args []string) error {
	fs := flag.NewFlagSet("def2lib20", flag.ContinueOnError)
	out := fs.String("o", "", "output .lib (default: the .def name with .lib)")
	dll := fs.String("dll", "", "DLL file name (default: LIBRARY name + .dll)")
	prefix := fs.String("prefix", "", "prefix for linker symbols (e.g. _ for cdecl C exports)")
	ts := fs.Uint("ts", 0, "timestamp written in the headers (default: .def modification time)")
	monkey := fs.Bool("monkey", false, "imitate Alaska aimplib: <dll>_IMPORT_DESCRIPTOR / NULL_IMPORT_DESCRIPTOR / <dll>_NULL_THUNK_DATA names and hint 0xFFFF (shares the terminator with the Xbase++ runtime libraries)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ot4xb-tool def2lib20 [-monkey] [-o out.lib] [-dll name.dll] [-prefix _] [-ts seconds] file.def")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("one .def file expected")
	}
	opts := def2lib20.Options{
		DLL:       *dll,
		Prefix:    *prefix,
		Timestamp: uint32(*ts),
		Monkey:    *monkey,
		Warn:      func(msg string) { fmt.Fprintln(os.Stderr, "def2lib20: warning:", msg) },
	}
	n, err := def2lib20.BuildFile(fs.Arg(0), *out, opts)
	if err != nil {
		return err
	}
	libPath := *out
	if libPath == "" {
		libPath = fs.Arg(0)[:len(fs.Arg(0))-len(ext(fs.Arg(0)))] + ".lib"
	}
	say("def2lib20: %d imports -> %s\n", n, libPath)
	return nil
}

func runCbk2obj(args []string) error {
	fs := flag.NewFlagSet("cbk2obj", flag.ContinueOnError)
	out := fs.String("o", "", "output .obj (default: the .cbk name with .obj)")
	asm := fs.Bool("asm", false, "also write the equivalent FASM source (.asm) next to the object")
	ts := fs.Uint("ts", 0, "timestamp written in the COFF header (default: .cbk modification time)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ot4xb-tool [-q] cbk2obj [-asm] [-o out.obj] [-ts seconds] file.cbk")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("one .cbk file expected")
	}
	res, err := cbk2obj.BuildFile(fs.Arg(0), *out, cbk2obj.Options{Timestamp: uint32(*ts), Asm: *asm})
	for _, d := range res.Diags {
		fmt.Fprintln(os.Stderr, d)
	}
	if err != nil {
		return err
	}
	say("cbk2obj: %d callbacks -> %s\n", res.Callbacks, res.Obj)
	if res.Asm != "" {
		say("cbk2obj: %s\n", res.Asm)
	}
	return nil
}

func ext(p string) string {
	for i := len(p) - 1; i >= 0 && p[i] != '/' && p[i] != '\\'; i-- {
		if p[i] == '.' {
			return p[i:]
		}
	}
	return ""
}
