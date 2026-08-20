// macros.go expands the {$<...>$} macros of a version script body. The
// scanner is a byte FSM: it looks for "{$<", reads the macro name up to the
// matching ">$}" (or ":)>$}" for the masked FILEVERSION form) and replaces
// known macros, leaving unknown ones untouched. Macro names are
// case-insensitive; spelling is otherwise exact.
package vbuild

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Context carries the values the macros render.
type Context struct {
	// Version is the version after the increment.
	Version Version
	// Now is the run's timestamp (local clock); one value for every macro.
	Now time.Time
	// UUID is the run's uuid: 32 lower-case hex digits, no dashes. Empty
	// means "generate one" (NewUUID) at first use.
	UUID string
	// Folder is the value of {$<fld>$}: the name of the folder that
	// contains the script.
	Folder string
}

// NewUUID returns a fresh random uuid rendered the vbuild way: 32 lower-case
// hex digits, no dashes.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0F) | 0x40 // version 4
	b[8] = (b[8] & 0x3F) | 0x80 // variant 10
	return hex.EncodeToString(b[:]), nil
}

// Expand replaces every macro of body and returns the result. Unknown
// macros stay as written.
func Expand(body []byte, ctx *Context) []byte {
	var out bytes.Buffer
	i, n := 0, len(body)
	for i < n {
		j := indexFrom(body, i, "{$<")
		if j < 0 {
			out.Write(body[i:])
			break
		}
		out.Write(body[i:j])
		rep, consumed := expandAt(body[j:], ctx)
		if consumed == 0 {
			out.WriteString("{$<")
			i = j + 3
			continue
		}
		out.Write(rep)
		i = j + consumed
	}
	return out.Bytes()
}

// expandAt tries to expand one macro at the start of b (b starts with "{$<").
// It returns the replacement and the number of input bytes consumed; 0 means
// "not a macro, keep the text".
func expandAt(b []byte, ctx *Context) ([]byte, int) {
	inner := b[3:]
	// masked FILEVERSION: {$<FILEVERSION(: mask :)>$}
	if hasFold(inner, "fileversion(:") {
		mask := inner[len("fileversion(:"):]
		end := bytes.Index(mask, []byte(":)>$}"))
		if end < 0 {
			return nil, 0
		}
		return expandMask(mask[:end], ctx.Version), 3 + len("fileversion(:") + end + len(":)>$}")
	}
	end := bytes.Index(inner, []byte(">$}"))
	if end < 0 {
		return nil, 0
	}
	name := inner[:end]
	total := 3 + end + 3
	v := ctx.Version
	var s string
	switch {
	case eqFold(name, "FILEVERSION(,,,)"):
		s = fmt.Sprintf("%d,%d,%d,%d", v[0], v[1], v[2], v[3])
	case eqFold(name, "FILEVERSION(...)"):
		s = fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
	case eqFold(name, "FILEVERSION(000,000,000,000)"):
		s = fmt.Sprintf("%03d,%03d,%03d,%03d", v[0], v[1], v[2], v[3])
	case eqFold(name, "FILEVERSION(000.000.000.000)"):
		s = fmt.Sprintf("%03d.%03d.%03d.%03d", v[0], v[1], v[2], v[3])
	case eqFold(name, "FILEVERSION(_._._._)"):
		s = fmt.Sprintf("%03d_%03d_%03d_%03d", v[0], v[1], v[2], v[3])
	case eqFold(name, "LOCALTIME(YYYY)"):
		s = ctx.Now.Format("2006")
	case eqFold(name, "LOCALTIME(YYYYMMDD)"):
		s = ctx.Now.Format("20060102")
	case eqFold(name, "LOCALTIME(14)"):
		s = ctx.Now.Format("20060102150405")
	case eqFold(name, "LOCALTIME(19)"):
		s = ctx.Now.Format("2006-01-02 15:04:05")
	case eqFold(name, "LOCALTIME(19+z)"):
		s = ctx.Now.Format("2006-01-02 15:04:05 -0700")
	case eqFold(name, "SYSTEMTIME(14)"):
		s = ctx.Now.UTC().Format("20060102150405")
	case eqFold(name, "SYSTEMTIME(19)"):
		s = ctx.Now.UTC().Format("2006-01-02 15:04:05")
	case eqFold(name, "UUID"):
		if ctx.UUID == "" {
			u, err := NewUUID()
			if err != nil {
				return nil, 0
			}
			ctx.UUID = u
		}
		s = ctx.UUID
	case eqFold(name, "fld"):
		s = ctx.Folder
	default:
		return nil, 0
	}
	return []byte(s), total
}

// expandMask renders a FILEVERSION mask: free text in which <maj>, <min>,
// <hbuild>, <lbuild> and <build> — optionally with a width, <maj(03)> /
// <maj(3)> — are replaced; everything else, a non-component <word>
// included, is copied as is.
func expandMask(mask []byte, v Version) []byte {
	var out bytes.Buffer
	i, n := 0, len(mask)
	for i < n {
		if mask[i] != '<' {
			out.WriteByte(mask[i])
			i++
			continue
		}
		val, consumed := maskComponent(mask[i:], v)
		if consumed == 0 {
			out.WriteByte('<')
			i++
			continue
		}
		out.WriteString(val)
		i += consumed
	}
	return out.Bytes()
}

// maskComponent parses one <component[(width)]> at the start of b and
// renders it; consumed 0 = not a component.
func maskComponent(b []byte, v Version) (string, int) {
	i := 1 // skip '<'
	start := i
	for i < len(b) && isAlpha(b[i]) {
		i++
	}
	name := lower(string(b[start:i]))
	var val int
	switch name {
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
		return "", 0
	}
	format := "%d"
	if i < len(b) && b[i] == '(' {
		j := i + 1
		zero := false
		if j < len(b) && b[j] == '0' {
			zero = true
			j++
		}
		width := 0
		digits := 0
		for j < len(b) && b[j] >= '0' && b[j] <= '9' {
			width = width*10 + int(b[j]-'0')
			digits++
			j++
		}
		if digits == 0 || j >= len(b) || b[j] != ')' {
			return "", 0
		}
		if zero {
			format = fmt.Sprintf("%%0%dd", width)
		} else {
			format = fmt.Sprintf("%%%dd", width)
		}
		i = j + 1
	}
	if i >= len(b) || b[i] != '>' {
		return "", 0
	}
	return fmt.Sprintf(format, val), i + 1
}

func indexFrom(b []byte, from int, s string) int {
	k := bytes.Index(b[from:], []byte(s))
	if k < 0 {
		return -1
	}
	return from + k
}

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// eqFold reports whether a equals b ASCII-case-insensitively.
func eqFold(a []byte, b string) bool {
	return len(a) == len(b) && hasFold(a, lower(b))
}
