// Package vsxbt is the build-step runner of ot4xb-tool: it loads a project
// tool file (<project>.ot4xb-tool, JSON) and executes one of its entries
// ("pre.release", "post.debug", ...), each a sequence of steps backed by the
// tool modules — vbuild, xbmac2h, def2lib20, artefacts — plus the file
// primitives cleanup.before and copy.
//
// The tool file (see the ot4xb repository for a real one):
//
//	{
//	  "versioninfo": "./source/ot4xb.VersionInfo",
//	  "pre.release": {
//	    "folders": { "src": "./source" },
//	    "steps": [ "vbuild", "codegen" ],
//	    "vbuild": { "flags": "" },
//	    "codegen": {
//	      "cleanup.before": { "files": [ "<src>/Release/ot4xb.obj" ] },
//	      "xbmac2h": { "src": "<src>/ot4xb.xbmac" }
//	    }
//	  },
//	  "pre.debug": { "template": "pre.release" },
//	  ...
//	}
//
// Rules of the file:
//   - JSON is read leniently: // line comments and trailing commas are fine.
//   - Every relative path resolves against the folder of the tool file,
//     whatever the current directory is.
//   - "folders" and "vars" of an entry are one namespace, referenced as
//     <name> inside any value (case-insensitive, values may reference each
//     other). <v.maj>, <v.min>, <v.hbuild>, <v.lbuild> and <v.build> render
//     the version of the "versioninfo" script; an optional width follows a
//     colon: <v.maj:03> = %03d, <v.maj:3> = %3d.
//   - Paths are normalised after expansion: "\" and "/" are the same,
//     doubled separators at macro junctions collapse (src=./patata/ +
//     <src>/frita -> ./patata/frita); Windows separators are used on disk.
//   - "template": "other" starts the entry as a copy of the other one;
//     folders and vars merge by name (the entry wins), everything else the
//     entry declares replaces the template's value.
//   - A companion file "<toolfile>.user" (never tracked, personal) may hold
//     the same entries; after the main entry runs, the .user entry of the
//     same name runs with the main entry's folders and vars as base.
package vsxbt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Tool is a loaded .ot4xb-tool file.
type Tool struct {
	// Path is the tool file; Dir anchors every relative path.
	Path string
	Dir  string
	// VersionInfo is the version script path as written ("" when absent).
	VersionInfo string

	entries map[string]json.RawMessage
	// version cache (see version.go)
	ver      *toolVersion
	userTool *Tool
}

// Options controls Run.
type Options struct {
	// Say receives one line per action done (files written, zips built...).
	Say func(msg string)
	// Warn receives non-fatal notices from the modules.
	Warn func(msg string)
}

// Load reads a tool file and, when it exists, its "<file>.user" companion.
func Load(path string) (*Tool, error) {
	t, err := loadFile(path)
	if err != nil {
		return nil, err
	}
	userPath := path + ".user"
	if _, err := os.Stat(userPath); err == nil {
		u, err := loadFile(userPath)
		if err != nil {
			return nil, err
		}
		t.userTool = u
	}
	return t, nil
}

func loadFile(path string) (*Tool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(lenientJSON(raw), &top); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	t := &Tool{Path: path, Dir: filepath.Dir(abs), entries: map[string]json.RawMessage{}}
	for k, v := range top {
		if strings.EqualFold(k, "versioninfo") {
			if err := json.Unmarshal(v, &t.VersionInfo); err != nil {
				return nil, fmt.Errorf("%s: versioninfo: %w", path, err)
			}
			continue
		}
		t.entries[strings.ToLower(k)] = v
	}
	return t, nil
}

// Entries lists the entry names of the file (lower-cased, sorted order not
// guaranteed).
func (t *Tool) Entries() []string {
	var out []string
	for k := range t.entries {
		out = append(out, k)
	}
	return out
}

// entry is a resolved entry: folders+vars namespace, ordered steps and the
// raw config of each step.
type entry struct {
	names map[string]string // folders + vars, lower-cased names, raw values
	steps []string
	cfg   map[string]json.RawMessage // step name (lower) -> config
}

// resolveEntry returns the entry after applying its template chain.
func (t *Tool) resolveEntry(name string, seen map[string]bool) (*entry, error) {
	raw, ok := t.entries[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("%s: entry %q not found", t.Path, name)
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	if seen[strings.ToLower(name)] {
		return nil, fmt.Errorf("%s: template cycle at %q", t.Path, name)
	}
	seen[strings.ToLower(name)] = true

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: entry %q: %w", t.Path, name, err)
	}
	e := &entry{names: map[string]string{}, cfg: map[string]json.RawMessage{}}
	if tmplRaw, ok := lookup(m, "template"); ok {
		var tmpl string
		if err := json.Unmarshal(tmplRaw, &tmpl); err != nil {
			return nil, fmt.Errorf("%s: entry %q: template: %w", t.Path, name, err)
		}
		base, err := t.resolveEntry(tmpl, seen)
		if err != nil {
			return nil, err
		}
		*e = *base
		// deep-copy the maps the base shares
		e.names = copyMap(base.names)
		e.cfg = copyRawMap(base.cfg)
	}
	for k, v := range m {
		lk := strings.ToLower(k)
		switch lk {
		case "template":
			// done
		case "folders", "vars":
			var kv map[string]string
			if err := json.Unmarshal(v, &kv); err != nil {
				return nil, fmt.Errorf("%s: entry %q: %s: %w", t.Path, name, k, err)
			}
			for n, val := range kv {
				e.names[strings.ToLower(n)] = val
			}
		case "steps":
			var ss []string
			if err := json.Unmarshal(v, &ss); err != nil {
				return nil, fmt.Errorf("%s: entry %q: steps: %w", t.Path, name, err)
			}
			e.steps = ss
		default:
			e.cfg[lk] = v
		}
	}
	return e, nil
}

// Run executes one entry of the tool file and, when a .user companion has
// the same entry, that one right after (inheriting the entry's folders and
// vars).
func (t *Tool) Run(entryName string, o Options) error {
	if o.Say == nil {
		o.Say = func(string) {}
	}
	if o.Warn == nil {
		o.Warn = func(string) {}
	}
	e, mainErr := t.resolveEntry(entryName, nil)
	var notFound bool
	if mainErr != nil {
		if t.userTool == nil {
			return mainErr
		}
		notFound = true
	}
	if !notFound {
		if err := t.runEntry(e, entryName, o); err != nil {
			return err
		}
	}
	if t.userTool == nil {
		return nil
	}
	ue, err := t.userTool.resolveEntry(entryName, nil)
	if err != nil {
		if notFound {
			return mainErr // neither file has the entry
		}
		return nil // the .user simply does not extend this entry
	}
	// the .user entry inherits the main entry's names as base
	if !notFound {
		merged := copyMap(e.names)
		for k, v := range ue.names {
			merged[k] = v
		}
		ue.names = merged
	}
	// version macros of the .user resolve against the main versioninfo when
	// the .user does not declare one
	if t.userTool.VersionInfo == "" {
		t.userTool.VersionInfo = t.VersionInfo
		t.userTool.Dir = t.Dir
	}
	o.Say(fmt.Sprintf("user: %s", t.userTool.Path))
	return t.userTool.runEntry(ue, entryName, o)
}

func (t *Tool) runEntry(e *entry, name string, o Options) error {
	if len(e.steps) == 0 {
		return fmt.Errorf("%s: entry %q has no steps", t.Path, name)
	}
	for _, step := range e.steps {
		cfg, ok := e.cfg[strings.ToLower(step)]
		if !ok {
			return fmt.Errorf("%s: entry %q: step %q has no configuration", t.Path, name, step)
		}
		if err := t.runStep(e, step, cfg, o); err != nil {
			return fmt.Errorf("entry %q, step %q: %w", name, step, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- helpers

func lookup(m map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// lenientJSON strips // line comments (outside strings) and trailing commas
// so hand-edited tool files parse.
func lenientJSON(in []byte) []byte {
	var out bytes.Buffer
	inStr := false
	esc := false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inStr {
			out.WriteByte(c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
			out.WriteByte(c)
		case '/':
			if i+1 < len(in) && in[i+1] == '/' {
				for i < len(in) && in[i] != '\n' {
					i++
				}
				if i < len(in) {
					out.WriteByte('\n')
				}
			} else {
				out.WriteByte(c)
			}
		case ',':
			// trailing comma: look ahead past blanks and comments for } or ]
			j := i + 1
			for j < len(in) {
				if in[j] == ' ' || in[j] == '\t' || in[j] == '\r' || in[j] == '\n' {
					j++
					continue
				}
				if in[j] == '/' && j+1 < len(in) && in[j+1] == '/' {
					for j < len(in) && in[j] != '\n' {
						j++
					}
					continue
				}
				break
			}
			if j < len(in) && (in[j] == '}' || in[j] == ']') {
				continue // drop the comma
			}
			out.WriteByte(c)
		default:
			out.WriteByte(c)
		}
	}
	return out.Bytes()
}

var errNoVersionInfo = errors.New("vsxbt: <v.*> used but the tool file has no \"versioninfo\"")
