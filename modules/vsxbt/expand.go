// expand.go resolves the <name> macros of the tool file values — folders,
// vars and the <v.*> version components — and normalises paths the agreed
// way: work with "/", collapse doubled separators at macro junctions, hand
// "\" to the disk.
package vsxbt

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/vbuild"
)

const maxExpandDepth = 16

// expand replaces every <name> of value using the entry namespace and the
// version components. Unknown names are an error (loud beats silent).
func (t *Tool) expand(e *entry, value string) (string, error) {
	return t.expandDepth(e, value, 0)
}

func (t *Tool) expandDepth(e *entry, value string, depth int) (string, error) {
	if depth > maxExpandDepth {
		return "", fmt.Errorf("vsxbt: macro expansion too deep (cycle?) in %q", value)
	}
	var out strings.Builder
	i, n := 0, len(value)
	for i < n {
		c := value[i]
		if c != '<' {
			out.WriteByte(c)
			i++
			continue
		}
		end := strings.IndexByte(value[i+1:], '>')
		if end < 0 {
			out.WriteString(value[i:])
			break
		}
		name := value[i+1 : i+1+end]
		rep, ok, err := t.resolveName(e, name, depth)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("vsxbt: unknown macro <%s> in %q", name, value)
		}
		// collapse separators at the junctions: drop a trailing separator of
		// the replacement when the text continues with one, and vice versa
		rep = strings.ReplaceAll(rep, "\\", "/")
		if strings.HasSuffix(rep, "/") && i+1+end+1 < n && (value[i+1+end+1] == '/' || value[i+1+end+1] == '\\') {
			rep = strings.TrimRight(rep, "/")
		}
		out.WriteString(rep)
		i += end + 2
	}
	return out.String(), nil
}

// resolveName resolves one macro name: v.* version components or a
// folders/vars entry (case-insensitive).
func (t *Tool) resolveName(e *entry, name string, depth int) (string, bool, error) {
	if len(name) > 2 && (name[0] == 'v' || name[0] == 'V') && name[1] == '.' {
		s, err := t.versionComponent(name[2:])
		if err != nil {
			return "", false, err
		}
		return s, true, nil
	}
	raw, ok := e.names[strings.ToLower(name)]
	if !ok {
		return "", false, nil
	}
	s, err := t.expandDepth(e, raw, depth+1)
	if err != nil {
		return "", false, err
	}
	return s, true, nil
}

// versionComponent renders "maj", "min:03", "build:5", ... from the
// versioninfo header.
func (t *Tool) versionComponent(spec string) (string, error) {
	name, width := spec, ""
	if k := strings.IndexByte(spec, ':'); k >= 0 {
		name, width = spec[:k], spec[k+1:]
	}
	v, err := t.version()
	if err != nil {
		return "", err
	}
	var val int
	switch strings.ToLower(name) {
	case "maj":
		val = int(v[0])
	case "min":
		val = int(v[1])
	case "hbuild":
		val = int(v[2])
	case "lbuild":
		val = int(v[3])
	case "build":
		val = int(v.Build())
	default:
		return "", fmt.Errorf("vsxbt: unknown version component <v.%s>", spec)
	}
	if width == "" {
		return fmt.Sprintf("%d", val), nil
	}
	w := 0
	zero := strings.HasPrefix(width, "0")
	for _, c := range width {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("vsxbt: bad width in <v.%s>", spec)
		}
		w = w*10 + int(c-'0')
	}
	if zero {
		return fmt.Sprintf("%0*d", w, val), nil
	}
	return fmt.Sprintf("%*d", w, val), nil
}

// toolVersion caches the versioninfo header.
type toolVersion struct {
	v vbuild.Version
}

// version reads (and caches) the version of the versioninfo script. A
// vbuild step invalidates the cache.
func (t *Tool) version() (vbuild.Version, error) {
	if t.ver != nil {
		return t.ver.v, nil
	}
	if t.VersionInfo == "" {
		return vbuild.Version{}, errNoVersionInfo
	}
	s, err := vbuild.ParseFile(t.abs(t.VersionInfo))
	if err != nil {
		return vbuild.Version{}, err
	}
	t.ver = &toolVersion{v: s.Header.Version}
	return t.ver.v, nil
}

// abs resolves a tool-file-relative path and hands it to Windows: "/" and
// "\" both accepted, doubled separators collapsed, "\" on disk.
func (t *Tool) abs(p string) string {
	s := normSlash(p)
	if !filepath.IsAbs(filepath.FromSlash(s)) {
		s = normSlash(filepath.ToSlash(t.Dir) + "/" + s)
	}
	return filepath.Clean(filepath.FromSlash(s))
}

// expandPath expands macros and resolves the result against the tool dir.
func (t *Tool) expandPath(e *entry, value string) (string, error) {
	s, err := t.expand(e, value)
	if err != nil {
		return "", err
	}
	return t.abs(s), nil
}

// normSlash turns every "\" into "/" and collapses doubled separators
// (keeping a leading "//" for UNC paths).
func normSlash(p string) string {
	s := strings.ReplaceAll(p, "\\", "/")
	unc := strings.HasPrefix(s, "//")
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	if unc {
		s = "/" + s
	}
	return s
}
