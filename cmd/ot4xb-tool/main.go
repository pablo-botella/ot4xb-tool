// Command ot4xb-tool is the single binary of the ot4xb tool set; each tool is
// a subcommand backed by a package under modules/.
//
//	ot4xb-tool [-q] [-tool file.ot4xb-tool] -bs entry            run a build-step entry
//	ot4xb-tool [-q] vbuild [-inc major|minor|hbuild|lbuild|build] [-eolrn|-eolr|-eoln|-eols] file.VersionInfo
//	ot4xb-tool [-q] xbmac2h [-lib NAME] file.xbmac
//	ot4xb-tool [-q] def2lib20 [-monkey] [-o out.lib] [-dll name.dll] [-prefix _] [-ts seconds] file.def
//	ot4xb-tool [-q] cbk2obj [-asm] [-o out.obj] [-ts seconds] file.cbk
//	ot4xb-tool [-q] scandoc -src path [-fields] [-tags] [-tagsout file]
//	ot4xb-tool [-q] srcsplit -src path|glob [-code dst] [-doc dst] [-bak|-force] [-check]
//	ot4xb-tool [-q] doccheck -src dir -xbmac file.xbmac [-full]
//	ot4xb-tool [-q] compile -root dir -db file.db -src path|glob|dir [-src ...]
//	ot4xb-tool [-q] resolve -db file.db
//	ot4xb-tool [-q] gendoc -db file.db -out dir
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/cbk2obj"
	"github.com/pablo-botella/ot4xb-tool/modules/def2lib20"
	"github.com/pablo-botella/ot4xb-tool/modules/doccheck"
	"github.com/pablo-botella/ot4xb-tool/modules/doccompile"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/docgen"
	"github.com/pablo-botella/ot4xb-tool/modules/docresolve"
	"github.com/pablo-botella/ot4xb-tool/modules/scandoc"
	"github.com/pablo-botella/ot4xb-tool/modules/srcsplit"
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
	case "scandoc":
		err = runScandoc(args[1:])
	case "srcsplit":
		err = runSrcsplit(args[1:])
	case "doccheck":
		err = runDoccheck(args[1:])
	case "compile":
		err = runCompile(args[1:])
	case "resolve":
		err = runResolve(args[1:])
	case "gendoc":
		err = runGendoc(args[1:])
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
	fmt.Fprintln(os.Stderr, "  scandoc     scan C/C++ sources for /*{{ }}*/ documentation markers: print the model and diagnostics")
	fmt.Fprintln(os.Stderr, "  srcsplit    split an authoring source into its code projection (no /*{{ }}*/ doc) and its doc projection")
	fmt.Fprintln(os.Stderr, "  doccheck    cross-check the documented surface against the .xbmac registration list")
	fmt.Fprintln(os.Stderr, "  compile     compile documented sources into the intermediate SQLite database (any number of passes)")
	fmt.Fprintln(os.Stderr, "  resolve     the a-posteriori step over that database: broken references, include cycles, duplicates")
	fmt.Fprintln(os.Stderr, "  gendoc      generate the reference Markdown from that database: one file per topic (slugs) plus an index")
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

// scandocUsage prints the scandoc parameter summary (all parameters are
// named; positionals end up being a pain).
func scandocUsage() {
	fmt.Fprintln(os.Stderr, "usage: ot4xb-tool [-q] scandoc -src path [-fields] [-tags [file]]")
	fmt.Fprintln(os.Stderr, "  -src path     file or directory to scan")
	fmt.Fprintln(os.Stderr, "  -fields       print every field of every entity")
	fmt.Fprintln(os.Stderr, "  -tags [file]  tag inventory (debug): alone it prints, with a file it writes every")
	fmt.Fprintln(os.Stderr, "                occurrence there, one per line (ASCII + CRLF)")
}

func runScandoc(args []string) error {
	// Hand-parsed (like -tool/-bs): -tags takes an OPTIONAL value, which
	// the flag package cannot express. One syntax only: -name value.
	var path, tagsOut string
	var fields, tags bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-src":
			if i+1 >= len(args) {
				return fmt.Errorf("-src needs a path")
			}
			i++
			path = args[i]
		case "-fields":
			fields = true
		case "-tags":
			tags = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				tagsOut = args[i]
			}
		case "-h", "--help", "/?":
			scandocUsage()
			return nil
		default:
			scandocUsage()
			return fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	if path == "" {
		scandocUsage()
		return fmt.Errorf("-src path missing")
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	var files []*scandoc.File
	if st.IsDir() {
		files, err = scandoc.ScanDir(path)
	} else {
		var f *scandoc.File
		f, err = scandoc.Scan(path)
		files = []*scandoc.File{f}
	}
	if err != nil {
		return err
	}
	// tag inventory (per first word of the field name, the vocabulary key);
	// occurrences carry every single appearance for the -tags file, which
	// exists to analyze the tags in detail separately
	type tagInfo struct {
		name    string
		count   int
		unknown bool
		file    string
		line    int
	}
	inventory := make(map[string]*tagInfo)
	var occurrences []string
	nErr := 0
	// Project-level resolution: cross-file includes, ilink targets, note cycles
	// and duplicate identities across the whole scan (a single file is a project
	// of one).
	scandoc.ResolveProject(files)
	for _, f := range files {
		if len(f.Entities)+len(f.Notes)+len(f.Diags) > 0 {
			say("scandoc: %s: %d entities, %d notes\n", f.Path, len(f.Entities), len(f.Notes))
		}
		for i := range f.Entities {
			e := &f.Entities[i]
			say("  %5d-%-5d %-12s %s\n", e.StartLine, e.EndLine, e.Kind, e.Ident)
			for _, fd := range e.Fields {
				if fields {
					say("        %5d  | %s: %s\n", fd.Line, fd.Name, fd.Value)
				}
				name := fd.Name
				if i := strings.IndexAny(name, " \t"); i >= 0 {
					name = name[:i]
				}
				key := strings.ToLower(name)
				ti := inventory[key]
				if ti == nil {
					ti = &tagInfo{name: name, unknown: fd.Unknown, file: f.Path, line: fd.Line}
					inventory[key] = ti
				}
				ti.count++
				if tagsOut != "" {
					status := "known"
					if fd.Unknown {
						status = "unknown"
					}
					occurrences = append(occurrences, fmt.Sprintf("%s\t%s\t%s:%d", status, name, f.Path, fd.Line))
				}
			}
			for _, id := range e.NoteRefs {
				say("        include-note-id: %s\n", id)
			}
			for ci := range e.Components {
				c := &e.Components[ci]
				// identity is Owner:Name; show the authored form when it differs.
				if c.Ident != c.Name {
					say("        %5d  %-16s %s:%s  (%s)\n", c.Line, c.Kind, e.Ident, c.Name, c.Ident)
				} else {
					say("        %5d  %-16s %s:%s\n", c.Line, c.Kind, e.Ident, c.Name)
				}
				if fields {
					for _, fd := range c.Fields {
						say("        %5d    | %s: %s\n", fd.Line, fd.Name, fd.Value)
					}
				}
			}
		}
		for i := range f.Notes {
			n := &f.Notes[i]
			title := n.Title
			if title != "" {
				title = " (" + title + ")"
			}
			say("  %5d-%-5d %-12s %s%s\n", n.StartLine, n.EndLine, "note", n.ID, title)
		}
		for _, d := range f.Diags {
			fmt.Fprintf(os.Stderr, "%s:%d: %s: %s\n", f.Path, d.Line, d.Severity, d.Msg)
		}
		nErr += f.Errors()
	}
	if tags && tagsOut == "" {
		var known, unknown []string
		for _, ti := range inventory {
			if ti.unknown {
				unknown = append(unknown, fmt.Sprintf("%s(%d, %s:%d)", ti.name, ti.count, ti.file, ti.line))
			} else {
				known = append(known, fmt.Sprintf("%s(%d)", ti.name, ti.count))
			}
		}
		sort.Strings(known)
		sort.Strings(unknown)
		say("known tags:   %s\n", strings.Join(known, " "))
		say("unknown tags: %s\n", strings.Join(unknown, " "))
	}
	if tagsOut != "" {
		// unknown first, then per tag and location: the file is for
		// analyzing the tags in detail separately
		sort.Slice(occurrences, func(i, j int) bool {
			ui, uj := strings.HasPrefix(occurrences[i], "unknown"), strings.HasPrefix(occurrences[j], "unknown")
			if ui != uj {
				return ui
			}
			return occurrences[i] < occurrences[j]
		})
		var sb strings.Builder
		for _, o := range occurrences {
			sb.WriteString(o + "\r\n")
		}
		if err := os.WriteFile(tagsOut, []byte(sb.String()), 0666); err != nil {
			return err
		}
		say("scandoc: %d tag occurrence(s) -> %s\n", len(occurrences), tagsOut)
	}
	if nErr > 0 {
		return fmt.Errorf("scandoc: %d error(s)", nErr)
	}
	return nil
}

// srcsplitUsage prints the srcsplit parameter summary.
func srcsplitUsage() {
	fmt.Fprintln(os.Stderr, "usage: ot4xb-tool [-q] srcsplit -src path|glob [-code dst] [-doc dst] [-bak|-force] [-check]")
	fmt.Fprintln(os.Stderr, "  -src path   a source file, a folder of them, or a glob mask (folder/mask)")
	fmt.Fprintln(os.Stderr, "  -code dst   write the code projection (the source without its /*{{ }}*/ doc blocks) to dst")
	fmt.Fprintln(os.Stderr, "  -doc dst    write the doc projection (only the /*{{ }}*/ blocks, verbatim) to dst")
	fmt.Fprintln(os.Stderr, "              dst may hold a '*' = the source name without extension (ch/*.ch); no '*' = one file,")
	fmt.Fprintln(os.Stderr, "              only for a single source. Extensions are never implied.")
	fmt.Fprintln(os.Stderr, "  -bak        an existing, different dst is kept as dst.bak before being overwritten")
	fmt.Fprintln(os.Stderr, "  -force      an existing, different dst is overwritten with no copy")
	fmt.Fprintln(os.Stderr, "              (without -bak or -force a different existing dst is never overwritten)")
	fmt.Fprintln(os.Stderr, "  -check      compare against the existing destinations and report drift, write nothing")
}

// runCompile is one compile pass of the documentation database: every -src
// (file, glob or directory) is scanned and written into -db under its path
// relative to -root. Passes are append-only per source and can be repeated;
// nothing is resolved here (that is a separate step).
func runCompile(args []string) error {
	var root, dbPath string
	var srcs []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-root":
			if i+1 >= len(args) {
				return fmt.Errorf("-root needs a directory")
			}
			i++
			root = args[i]
		case "-db":
			if i+1 >= len(args) {
				return fmt.Errorf("-db needs a file")
			}
			i++
			dbPath = args[i]
		case "-src":
			if i+1 >= len(args) {
				return fmt.Errorf("-src needs a path, glob or directory")
			}
			i++
			srcs = append(srcs, args[i])
		default:
			return fmt.Errorf("compile: unexpected argument %q", args[i])
		}
	}
	if root == "" || dbPath == "" || len(srcs) == 0 {
		return fmt.Errorf("usage: ot4xb-tool [-q] compile -root <projectdir> -db <file.db> -src <path|glob|dir> [-src ...]")
	}
	files, err := doccompile.ExpandSources(srcs)
	if err != nil {
		return err
	}
	n, err := doccompile.Compile(dbPath, root, files, func(s string) { say("%s\n", s) })
	if err != nil {
		return err
	}
	say("compile: %d source(s) -> %s\n", n, dbPath)
	return nil
}

// runResolve is the a-posteriori step over a compiled database: it verifies
// every reference against the topics and records the broken ones, include
// cycles and duplicate definitions as issues (code resolve/...). Re-runnable.
func runResolve(args []string) error {
	var dbPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-db":
			if i+1 >= len(args) {
				return fmt.Errorf("-db needs a file")
			}
			i++
			dbPath = args[i]
		default:
			return fmt.Errorf("resolve: unexpected argument %q", args[i])
		}
	}
	if dbPath == "" {
		return fmt.Errorf("usage: ot4xb-tool [-q] resolve -db <file.db>")
	}
	db, err := docdb.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	rep, err := docresolve.Resolve(db)
	if err != nil {
		return err
	}
	say("resolve: %d reference(s): %d skipped (unqualified), %d missing target, %d include cycle(s), %d duplicate definition(s)\n",
		rep.Refs, rep.Skipped, rep.Missing, rep.Cycles, rep.Duplicates)
	rows, err := db.QueryAll(`SELECT s.src, i.line, i.severity, i.message FROM issues i JOIN sources s USING(idsrc)
	                          WHERE i.code LIKE 'resolve/%' ORDER BY s.pos, i.line`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		line, _ := r[1].(int64) // NULL when the issue has no line
		fmt.Fprintf(os.Stderr, "%s:%d: %s: %s\n", r[0], line, r[2], r[3])
	}
	return nil
}

// runGendoc generates the reference Markdown from a compiled (and resolved)
// database: one file per topic, named by its slug, plus index.md.
func runGendoc(args []string) error {
	var dbPath, out string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-db":
			if i+1 >= len(args) {
				return fmt.Errorf("-db needs a file")
			}
			i++
			dbPath = args[i]
		case "-out":
			if i+1 >= len(args) {
				return fmt.Errorf("-out needs a directory")
			}
			i++
			out = args[i]
		default:
			return fmt.Errorf("gendoc: unexpected argument %q", args[i])
		}
	}
	if dbPath == "" || out == "" {
		return fmt.Errorf("usage: ot4xb-tool [-q] gendoc -db <file.db> -out <dir>")
	}
	_, err := docgen.Generate(dbPath, out, func(s string) {
		if strings.Contains(s, "warning:") {
			fmt.Fprintln(os.Stderr, s)
		} else {
			say("%s\n", s)
		}
	})
	return err
}

// runDoccheck cross-checks the documented surface (scandoc) against the DLL's
// registration list (xbmac2h): registered-but-undocumented (coverage gaps) and,
// with -full, documented-but-unregistered (stale or misspelled doc).
func runDoccheck(args []string) error {
	var src, mac string
	var full bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-src":
			if i+1 >= len(args) {
				return fmt.Errorf("-src needs a path")
			}
			i++
			src = args[i]
		case "-xbmac":
			if i+1 >= len(args) {
				return fmt.Errorf("-xbmac needs a path")
			}
			i++
			mac = args[i]
		case "-full":
			full = true
		default:
			return fmt.Errorf("doccheck: unexpected argument %q", args[i])
		}
	}
	if src == "" || mac == "" {
		return fmt.Errorf("usage: ot4xb-tool [-q] doccheck -src <sourcedir> -xbmac <file.xbmac> [-full]")
	}
	files, err := scandoc.ScanDir(src)
	if err != nil {
		return err
	}
	m, err := xbmac2h.ParseFile(mac)
	if err != nil {
		return err
	}
	rep := doccheck.Check(files, m)

	say("doccheck: registered %d functions, %d structures, %d c-exports; documented %d/%d/%d\n",
		rep.NRegFun, rep.NRegStruct, rep.NRegC, rep.NDocFun, rep.NDocStruct, rep.NDocC)
	say("doccheck: %d registered but UNDOCUMENTED, %d documented but unregistered\n",
		len(rep.RegisteredUndocumented), len(rep.DocumentedUnregistered))
	for _, miss := range rep.RegisteredUndocumented {
		w := miss.Where
		if w != "" {
			w = "  (" + w + ")"
		}
		say("  UNDOCUMENTED %-12s %s%s\n", miss.Group, miss.Name, w)
	}
	if full {
		for _, miss := range rep.DocumentedUnregistered {
			say("  UNREGISTERED %-12s %s  (%s)\n", miss.Group, miss.Name, miss.Where)
		}
	}
	return nil
}

func runSrcsplit(args []string) error {
	// Hand-parsed like scandoc: one syntax only, -name value.
	var src, code, doc string
	var bak, force, check bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-src":
			if i+1 >= len(args) {
				return fmt.Errorf("-src needs a path")
			}
			i++
			src = args[i]
		case "-code":
			if i+1 >= len(args) {
				return fmt.Errorf("-code needs a destination")
			}
			i++
			code = args[i]
		case "-doc":
			if i+1 >= len(args) {
				return fmt.Errorf("-doc needs a destination")
			}
			i++
			doc = args[i]
		case "-bak":
			bak = true
		case "-force":
			force = true
		case "-check":
			check = true
		case "-h", "--help", "/?":
			srcsplitUsage()
			return nil
		default:
			srcsplitUsage()
			return fmt.Errorf("unexpected argument %q", args[i])
		}
	}
	if src == "" {
		srcsplitUsage()
		return fmt.Errorf("-src path missing")
	}
	if code == "" && doc == "" {
		srcsplitUsage()
		return fmt.Errorf("give -code and/or -doc")
	}
	res, err := srcsplit.Run(src, srcsplit.Options{
		Code:  code,
		Doc:   doc,
		Bak:   bak,
		Force: force,
		Check: check,
		Warn:  func(m string) { fmt.Fprintln(os.Stderr, "srcsplit: warning:", m) },
	})
	if err != nil {
		return err
	}
	drift := 0
	report := func(dst string, wrote, backed bool) {
		switch {
		case wrote && backed:
			say("srcsplit: -> %s (previous kept as %s.bak)\n", dst, dst)
		case wrote:
			say("srcsplit: -> %s\n", dst)
		default:
			say("srcsplit: -> %s (unchanged)\n", dst)
		}
	}
	for _, r := range res {
		if check {
			if r.CodeDst != "" && r.CodeDrift {
				fmt.Fprintf(os.Stderr, "srcsplit: drift: %s would change\n", r.CodeDst)
				drift++
			}
			if r.DocDst != "" && r.DocDrift {
				fmt.Fprintf(os.Stderr, "srcsplit: drift: %s would change\n", r.DocDst)
				drift++
			}
			continue
		}
		say("srcsplit: %s\n", r.Src)
		if r.CodeDst != "" {
			report(r.CodeDst, r.CodeWrote, r.CodeBacked)
		}
		if r.DocDst != "" {
			report(r.DocDst, r.DocWrote, r.DocBacked)
		}
	}
	if drift > 0 {
		return fmt.Errorf("srcsplit: %d output(s) would change", drift)
	}
	if check {
		say("srcsplit: %d source(s) checked, no drift\n", len(res))
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
