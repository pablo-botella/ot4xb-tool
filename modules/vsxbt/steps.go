// steps.go executes the steps of an entry. A step whose name is one of the
// tool modules — vbuild, xbmac2h, def2lib20, artefacts — runs that tool with
// its config; any other step is a composite executed by its config keys, in
// this order: "cleanup.before" (delete files), "xbmac2h", "copy".
package vsxbt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/artefacts"
	"github.com/pablo-botella/ot4xb-tool/modules/def2lib20"
	"github.com/pablo-botella/ot4xb-tool/modules/vbuild"
	"github.com/pablo-botella/ot4xb-tool/modules/xbmac2h"
)

func (t *Tool) runStep(e *entry, name string, cfg json.RawMessage, o Options) error {
	switch strings.ToLower(name) {
	case "vbuild":
		return t.stepVbuild(e, cfg, o)
	case "xbmac2h":
		return t.stepXbmac2h(e, cfg, o)
	case "def2lib20":
		return t.stepDef2lib20(e, cfg, o)
	case "artefacts":
		return t.stepArtefacts(e, cfg, o)
	case "srcsplit":
		return t.stepSrcsplit(e, cfg, o)
	case "docs":
		return t.stepDocs(e, cfg, o)
	}
	// composite step: recognised keys in a fixed order
	var m map[string]json.RawMessage
	if err := json.Unmarshal(cfg, &m); err != nil {
		return err
	}
	known := 0
	if raw, ok := lookup(m, "cleanup.before"); ok {
		known++
		if err := t.stepCleanup(e, raw, o); err != nil {
			return err
		}
	}
	if raw, ok := lookup(m, "xbmac2h"); ok {
		known++
		if err := t.stepXbmac2h(e, raw, o); err != nil {
			return err
		}
	}
	if raw, ok := lookup(m, "copy"); ok {
		known++
		if err := t.stepCopy(e, raw, o); err != nil {
			return err
		}
	}
	if known == 0 {
		return fmt.Errorf("nothing to do (no vbuild/xbmac2h/def2lib20/artefacts step name, no cleanup.before/xbmac2h/copy keys)")
	}
	return nil
}

// ---------------------------------------------------------------- vbuild

func (t *Tool) stepVbuild(e *entry, cfg json.RawMessage, o Options) error {
	var c struct {
		Flags string `json:"flags"`
	}
	if len(cfg) > 0 {
		if err := json.Unmarshal(cfg, &c); err != nil {
			return err
		}
	}
	if t.VersionInfo == "" {
		return fmt.Errorf("vbuild step needs \"versioninfo\" in the tool file")
	}
	opts := vbuild.Options{
		Print: func(line string) { o.Say("vbuild: " + line) },
	}
	flags, err := t.expand(e, c.Flags)
	if err != nil {
		return err
	}
	for i, fields := 0, strings.Fields(flags); i < len(fields); i++ {
		f := fields[i]
		switch {
		case f == "-inc" && i+1 < len(fields):
			i++
			comp, err := vbuild.ParseComponent(fields[i])
			if err != nil {
				return err
			}
			opts.Inc = comp
		case strings.HasPrefix(f, "-inc="):
			comp, err := vbuild.ParseComponent(f[len("-inc="):])
			if err != nil {
				return err
			}
			opts.Inc = comp
		case f == "-eolrn":
			opts.Eol = vbuild.OutCrLf
		case f == "-eolr":
			opts.Eol = vbuild.OutCr
		case f == "-eoln":
			opts.Eol = vbuild.OutLf
		case f == "-eols":
			opts.Eol = vbuild.OutSave
		default:
			return fmt.Errorf("vbuild step: unknown flag %q", f)
		}
	}
	res, err := vbuild.Run(t.abs(t.VersionInfo), opts)
	if err != nil {
		return err
	}
	t.ver = nil // the version may have changed
	for _, f := range res.Files {
		o.Say("vbuild: " + f)
	}
	return nil
}

// ---------------------------------------------------------------- xbmac2h

func (t *Tool) stepXbmac2h(e *entry, cfg json.RawMessage, o Options) error {
	var c struct {
		Src string `json:"src"`
		Lib string `json:"lib"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return err
	}
	if c.Src == "" {
		return fmt.Errorf("xbmac2h: \"src\" missing")
	}
	src, err := t.expandPath(e, c.Src)
	if err != nil {
		return err
	}
	lib, err := t.expand(e, c.Lib)
	if err != nil {
		return err
	}
	paths, err := xbmac2h.Generate(src, xbmac2h.Options{
		LibName: lib,
		Warn:    func(msg string) { o.Warn("xbmac2h: " + msg) },
	})
	if err != nil {
		return err
	}
	for _, p := range paths {
		o.Say("xbmac2h: " + p)
	}
	return nil
}

// ---------------------------------------------------------------- def2lib20

func (t *Tool) stepDef2lib20(e *entry, cfg json.RawMessage, o Options) error {
	var c struct {
		In     string `json:"in"`
		Out    string `json:"out"`
		Monkey bool   `json:"monkey"`
		Prefix string `json:"prefix"`
		DLL    string `json:"dll"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return err
	}
	if c.In == "" {
		return fmt.Errorf("def2lib20: \"in\" missing")
	}
	in, err := t.expandPath(e, c.In)
	if err != nil {
		return err
	}
	out := ""
	if c.Out != "" {
		if out, err = t.expandPath(e, c.Out); err != nil {
			return err
		}
	}
	n, err := def2lib20.BuildFile(in, out, def2lib20.Options{
		Monkey: c.Monkey,
		Prefix: c.Prefix,
		DLL:    c.DLL,
		Warn:   func(msg string) { o.Warn("def2lib20: " + msg) },
	})
	if err != nil {
		return err
	}
	if out == "" {
		out = strings.TrimSuffix(in, filepath.Ext(in)) + ".lib"
	}
	o.Say(fmt.Sprintf("def2lib20: %d imports -> %s", n, out))
	return nil
}

// ---------------------------------------------------------------- artefacts

func (t *Tool) stepArtefacts(e *entry, cfg json.RawMessage, o Options) error {
	var list []struct {
		Type        string `json:"type"`
		ZipFilename string `json:"zip_filename"`
		Content     []struct {
			In               []string `json:"in"`
			Out              string   `json:"out"`
			CleanDocComments bool     `json:"clean_doc_comments"`
		} `json:"content"`
	}
	if err := json.Unmarshal(cfg, &list); err != nil {
		return err
	}
	for _, a := range list {
		if !strings.EqualFold(a.Type, "zip") {
			return fmt.Errorf("artefacts: unknown type %q", a.Type)
		}
		if a.ZipFilename == "" {
			return fmt.Errorf("artefacts: \"zip_filename\" missing")
		}
		zp, err := t.expandPath(e, a.ZipFilename)
		if err != nil {
			return err
		}
		var contents []artefacts.Content
		for _, c := range a.Content {
			var in []string
			for _, p := range c.In {
				ep, err := t.expandPath(e, p)
				if err != nil {
					return err
				}
				in = append(in, ep)
			}
			contents = append(contents, artefacts.Content{In: in, Out: c.Out, Clean: c.CleanDocComments})
		}
		names, err := artefacts.Zip(zp, contents, artefacts.Options{
			Warn: func(msg string) { o.Warn("artefacts: " + msg) },
		})
		if err != nil {
			return err
		}
		o.Say(fmt.Sprintf("artefacts: %s (%d files)", zp, len(names)))
	}
	return nil
}

// ---------------------------------------------------------------- cleanup / copy

func (t *Tool) stepCleanup(e *entry, cfg json.RawMessage, o Options) error {
	var c struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return err
	}
	for _, f := range c.Files {
		p, err := t.expandPath(e, f)
		if err != nil {
			return err
		}
		err = os.Remove(p)
		switch {
		case err == nil:
			o.Say("del: " + p)
		case os.IsNotExist(err):
			// nothing to delete
		default:
			return err
		}
	}
	return nil
}

func (t *Tool) stepCopy(e *entry, cfg json.RawMessage, o Options) error {
	var items []struct {
		In     []string `json:"in"`
		Out    string   `json:"out"`
		Create *bool    `json:"create"`
	}
	if err := json.Unmarshal(cfg, &items); err != nil {
		return err
	}
	for _, it := range items {
		if it.Out == "" {
			return fmt.Errorf("copy: \"out\" missing")
		}
		dst, err := t.expandPath(e, it.Out)
		if err != nil {
			return err
		}
		create := it.Create == nil || *it.Create
		if fi, err := os.Stat(dst); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			if !create {
				o.Say("copy: " + dst + " does not exist, skipped")
				continue
			}
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		} else if !fi.IsDir() {
			return fmt.Errorf("copy: %s is not a folder", dst)
		}
		for _, pat := range it.In {
			p, err := t.expandPath(e, pat)
			if err != nil {
				return err
			}
			matches, err := artefacts.Glob(p)
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				o.Warn("copy: " + p + ": no files match")
				continue
			}
			for _, m := range matches {
				target := filepath.Join(dst, filepath.Base(m))
				if err := copyFile(m, target); err != nil {
					return err
				}
				o.Say("copy: " + m + " -> " + target)
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return err
	}
	if fi, err := os.Stat(src); err == nil {
		_ = os.Chtimes(dst, fi.ModTime(), fi.ModTime())
	}
	return nil
}
