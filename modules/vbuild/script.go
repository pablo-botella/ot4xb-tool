// script.go is the producer side of vbuild: it reads a version script
// (.VersionInfo) into a Script — the version header plus the body lines —
// using github.com/pablo-botella/linereader. Every line parser here is a
// hand-written FSM over bytes: no regular expressions.
//
// A version script is:
//
//	[Product]  Version:{major,minor,hbuild,lbuild}
//	body: free text with {$<...>$} macros and {$<File:>$[path]$} markers
//
// The header line is matched byte by byte: optional blanks, '[', product
// (any text, the closing bracket is the LAST ']' before "Version:"),
// optional blanks, "Version:" (any case), '{', four decimal components in
// 0..255 separated by ',', '}', end of line. The product may carry the
// suffix ":noinc" (any case) which freezes the version.
package vbuild

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/pablo-botella/linereader"
)

// Version is the four-component version; index 0 = major, 1 = minor,
// 2 = hbuild, 3 = lbuild.
type Version [4]uint8

// Build returns the 16-bit build number: hbuild*256 + lbuild.
func (v Version) Build() uint16 { return uint16(v[2])<<8 | uint16(v[3]) }

// String renders "major,minor,hbuild,lbuild" with no padding.
func (v Version) String() string {
	return fmt.Sprintf("%d,%d,%d,%d", v[0], v[1], v[2], v[3])
}

// Component selects what Inc increments.
type Component int

const (
	// NoInc leaves the version unchanged (the vbuild default).
	NoInc Component = iota
	// Major increments major and resets minor, hbuild and lbuild.
	Major
	// Minor increments minor and resets hbuild and lbuild.
	Minor
	// HBuild increments hbuild and resets lbuild.
	HBuild
	// LBuild increments lbuild, carrying into hbuild, minor and major.
	// Build is the same increment (the build is the 16-bit number).
	LBuild
)

// ParseComponent maps the -inc argument names (major, minor, hbuild,
// lbuild, build; any case) to a Component.
func ParseComponent(name string) (Component, error) {
	switch lower(name) {
	case "major":
		return Major, nil
	case "minor":
		return Minor, nil
	case "hbuild":
		return HBuild, nil
	case "lbuild", "build":
		return LBuild, nil
	}
	return NoInc, fmt.Errorf("vbuild: unknown component %q (major, minor, hbuild, lbuild, build)", name)
}

// ErrOverflow is returned when an increment would carry past major = 255.
var ErrOverflow = errors.New("vbuild: version overflow (major past 255)")

// Inc returns the version after incrementing c. NoInc returns v unchanged.
func (v Version) Inc(c Component) (Version, error) {
	switch c {
	case NoInc:
		return v, nil
	case Major:
		if v[0] == 255 {
			return v, ErrOverflow
		}
		return Version{v[0] + 1, 0, 0, 0}, nil
	case Minor:
		if v[1] == 255 {
			return v.Inc(Major)
		}
		return Version{v[0], v[1] + 1, 0, 0}, nil
	case HBuild:
		if v[2] == 255 {
			return v.Inc(Minor)
		}
		return Version{v[0], v[1], v[2] + 1, 0}, nil
	case LBuild:
		if v[3] == 255 {
			return v.Inc(HBuild)
		}
		return Version{v[0], v[1], v[2], v[3] + 1}, nil
	}
	return v, fmt.Errorf("vbuild: unknown component %d", int(c))
}

// Header is the parsed first line of a version script.
type Header struct {
	// Product is the text between the brackets, ":noinc" suffix excluded.
	Product string
	Version Version
	// NoInc is set when the product carried the ":noinc" suffix.
	NoInc bool
}

// Format renders the normalised header line, terminator excluded:
// "[Product]  Version:{a,b,c,d}" — two blanks, capital V, no padding.
// A NoInc header keeps its ":noinc" suffix.
func (h Header) Format() string {
	p := h.Product
	if h.NoInc {
		p += ":noinc"
	}
	return "[" + p + "]  Version:{" + h.Version.String() + "}"
}

// ErrNoHeader is returned when the first line is not a version header.
var ErrNoHeader = errors.New("vbuild: first line is not a version header [Product]  Version:{a,b,c,d}")

// ErrRange is returned when a version component is not in 0..255.
var ErrRange = errors.New("vbuild: version component out of range 0..255")

// ParseHeader parses one header line (no terminator) with a byte FSM.
func ParseHeader(line []byte) (Header, error) {
	var h Header
	i, n := 0, len(line)
	for i < n && isBlank(line[i]) {
		i++
	}
	if i >= n || line[i] != '[' {
		return h, ErrNoHeader
	}
	i++
	prodStart := i
	// The product ends at the LAST ']' that is followed (after optional
	// blanks) by "Version:"; scan for every ']' candidate.
	prodEnd := -1
	verStart := -1
	for j := i; j < n; j++ {
		if line[j] != ']' {
			continue
		}
		k := j + 1
		for k < n && isBlank(line[k]) {
			k++
		}
		if hasFold(line[k:], "version:") {
			prodEnd = j
			verStart = k + len("version:")
		}
	}
	if prodEnd < 0 {
		return h, ErrNoHeader
	}
	product := string(line[prodStart:prodEnd])
	if len(product) == 0 {
		return h, ErrNoHeader
	}
	// ":noinc" suffix, any case
	if len(product) >= 6 && lower(product[len(product)-6:]) == ":noinc" {
		h.NoInc = true
		product = product[:len(product)-6]
	}
	h.Product = product
	i = verStart
	if i >= n || line[i] != '{' {
		return h, ErrNoHeader
	}
	i++
	for comp := 0; comp < 4; comp++ {
		val, digits := 0, 0
		for i < n && line[i] >= '0' && line[i] <= '9' {
			val = val*10 + int(line[i]-'0')
			if val > 255 {
				return Header{}, fmt.Errorf("%w: component %d", ErrRange, comp+1)
			}
			digits++
			i++
		}
		if digits == 0 {
			return Header{}, ErrNoHeader
		}
		h.Version[comp] = uint8(val)
		if comp < 3 {
			if i >= n || line[i] != ',' {
				return Header{}, ErrNoHeader
			}
			i++
		}
	}
	if i >= n || line[i] != '}' {
		return Header{}, ErrNoHeader
	}
	i++
	if i != n {
		return Header{}, ErrNoHeader
	}
	return h, nil
}

// Eol identifies a line terminator of the input script.
type Eol int

const (
	EolNone Eol = iota // last line without terminator
	EolCrLf
	EolCr
	EolLf
)

// Bytes returns the terminator bytes ("" for EolNone).
func (e Eol) Bytes() []byte {
	switch e {
	case EolCrLf:
		return []byte("\r\n")
	case EolCr:
		return []byte("\r")
	case EolLf:
		return []byte("\n")
	}
	return nil
}

// Line is one line of the script body with the terminator it had.
type Line struct {
	Text []byte
	Eol  Eol
}

// Script is a parsed version script.
type Script struct {
	Header Header
	// HeaderEol is the terminator the header line had in the file.
	HeaderEol Eol
	// Body holds the lines after the header, in order.
	Body []Line
	// Path is the file the script was read from ("" for Parse).
	Path string
}

// ParseFile reads and parses a version script.
func ParseFile(path string) (*Script, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	s, err := Parse(fh)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s.Path = path
	return s, nil
}

// Parse reads a script from r: the first line must be a header, the rest is
// the body, each line keeping its own terminator.
func Parse(r io.Reader) (*Script, error) {
	lr := linereader.NewLineReader(r, 0, 0)
	first, err := lr.ReadLine()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrNoHeader
		}
		return nil, err
	}
	h, err := ParseHeader(first)
	if err != nil {
		return nil, err
	}
	s := &Script{Header: h, HeaderEol: eolOf(lr.LastEolType)}
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		s.Body = append(s.Body, Line{Text: append([]byte(nil), raw...), Eol: eolOf(lr.LastEolType)})
	}
	return s, nil
}

func eolOf(t linereader.EolType) Eol {
	switch t {
	case linereader.EolCrLf:
		return EolCrLf
	case linereader.EolCr:
		return EolCr
	case linereader.EolLf:
		return EolLf
	}
	return EolNone
}

// ---------------------------------------------------------------- byte helpers

func isBlank(b byte) bool { return b == ' ' || b == '\t' }

// lower lower-cases ASCII letters of s.
func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// hasFold reports whether b starts with the ASCII string s, case-insensitively
// (s must be given in lower case).
func hasFold(b []byte, s string) bool {
	if len(b) < len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		if c != s[i] {
			return false
		}
	}
	return true
}
