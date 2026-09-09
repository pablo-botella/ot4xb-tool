// Package artefacts is the packager of ot4xb-tool: it materialises the
// "artefacts" entries of a .ot4xb-tool build step — today the "zip" type —
// and provides the Windows-style file matching the other steps share.
//
// A zip artefact is described by the target file name and a list of content
// items; each item has "in" (a list of file paths or glob patterns, wildcards
// in the last path element) and "out" (the folder inside the zip, "/" for the
// root). Matched files land flat in that folder under their base name.
//
// Everything is Windows-minded: "/" and "\" are interchangeable, matching is
// case-insensitive (*.h matches CRC32.HPP's sibling X.H, *.VersionInfo
// matches ot4xb.versioninfo), and the zip stores CRLF files as they are —
// bytes are never touched.
package artefacts

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/doctool/split"
)

// Content is one item of a zip artefact: the files that go into one folder
// of the archive.
type Content struct {
	// In lists file paths or glob patterns (wildcards in the base name only),
	// already resolved (no <folder> macros at this level).
	In []string
	// Out is the destination folder inside the zip: "/", "/include", ...
	Out string
	// Clean packs the clean projection of every source that carries /*{{ }}*/
	// documentation blocks (split.Code); files without them go as they are.
	Clean bool
}

// Options controls the packager.
type Options struct {
	// Warn, when not nil, receives non-fatal notices: a pattern that matched
	// nothing, a duplicate base name skipped.
	Warn func(msg string)
}

// Glob expands one path or pattern with Windows semantics: separators "/" or
// "\", case-insensitive matching, wildcards ('*', '?', '[...]') allowed in
// the last element only. A pattern with no wildcards returns the file itself
// when it exists (and nothing otherwise). Matches come back sorted by name,
// case-insensitively, for reproducible output.
func Glob(pattern string) ([]string, error) {
	p := filepath.FromSlash(pattern)
	dir, base := filepath.Split(p)
	if strings.ContainsAny(dir, "*?[") {
		return nil, fmt.Errorf("artefacts: wildcards allowed in the last path element only: %s", pattern)
	}
	if !strings.ContainsAny(base, "*?[") {
		if _, err := os.Stat(p); err != nil {
			return nil, nil
		}
		return []string{p}, nil
	}
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lbase := strings.ToLower(base)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ok, err := path.Match(lbase, strings.ToLower(e.Name()))
		if err != nil {
			return nil, fmt.Errorf("artefacts: bad pattern %q: %w", pattern, err)
		}
		if ok {
			out = append(out, filepath.Join(strings.TrimSuffix(dir, string(filepath.Separator)), e.Name()))
		}
	}
	sort.Slice(out, func(a, b int) bool {
		return strings.ToLower(out[a]) < strings.ToLower(out[b])
	})
	return out, nil
}

// Zip builds zipPath from the content items, creating the parent folders of
// the zip when needed. It returns the archive-relative names written, in
// order. Files are deflated; their modification times are kept. A pattern
// matching nothing warns and contributes nothing; two files mapping to the
// same name in the zip keep the first and warn about the rest.
func Zip(zipPath string, contents []Content, o Options) ([]string, error) {
	warn := o.Warn
	if warn == nil {
		warn = func(string) {}
	}
	type entry struct {
		src   string
		name  string // archive name, forward slashes, no leading slash
		clean bool
	}
	var entries []entry
	seen := map[string]string{} // lower archive name -> src
	for _, c := range contents {
		folder := strings.Trim(strings.ReplaceAll(c.Out, "\\", "/"), "/")
		for _, pat := range c.In {
			matches, err := Glob(pat)
			if err != nil {
				return nil, err
			}
			if len(matches) == 0 {
				warn(fmt.Sprintf("%s: no files match", pat))
				continue
			}
			for _, m := range matches {
				name := filepath.Base(m)
				if folder != "" {
					name = folder + "/" + name
				}
				key := strings.ToLower(name)
				if prev, dup := seen[key]; dup {
					warn(fmt.Sprintf("%s: duplicate zip entry %s (kept %s)", m, name, prev))
					continue
				}
				seen[key] = m
				entries = append(entries, entry{src: m, name: name, clean: c.Clean})
			}
		}
	}
	zp := filepath.FromSlash(zipPath)
	if dir := filepath.Dir(zp); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.Create(zp)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(f)
	var names []string
	for _, e := range entries {
		if err := addFile(zw, e.src, e.name, e.clean); err != nil {
			zw.Close()
			f.Close()
			os.Remove(zp)
			return nil, fmt.Errorf("%s: %w", e.src, err)
		}
		names = append(names, e.name)
	}
	if err := zw.Close(); err != nil {
		f.Close()
		os.Remove(zp)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(zp)
		return nil, err
	}
	return names, nil
}

func addFile(zw *zip.Writer, src, name string, clean bool) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if clean && bytes.Contains(data, []byte("/*{{")) {
		if data, _, err = split.Code(data); err != nil {
			return err
		}
	}
	hdr, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	hdr.Name = name
	hdr.Method = zip.Deflate
	hdr.UncompressedSize64 = uint64(len(data))
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
