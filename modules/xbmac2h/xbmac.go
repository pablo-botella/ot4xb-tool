// xbmac.go is the producer side of xbmac2h: it parses the function
// registration list of an ot4xb-style C/C++ DLL (.xbmac file) into a File,
// the structure the generators consume.
//
// The file is read line by line with github.com/pablo-botella/linereader
// (CR, LF and CRLF), as bytes (ANSI). Each line is one of:
//
//	_XPP_REG_FUN_( name )      plain Xbase++ function, C symbol NAME
//	_XPP_REG_WMAC( name )      macro-style wrapper, C symbol wapimc_NAME
//	_XPP_REG_WST_( name )      structure wrapper, C symbol wapist_NAME
//	_XPP_REG_WAPI( name )      Win32 API wrapper, C symbol wapi_NAME
//	_CDECL_EXPORT_( name )     plain C function the DLL exports under its own
//	                           name (cdecl), for C/C++ and Xbase++ clients alike
//
// blanks are free, "//" starts a comment to the end of the line. Blank
// lines and comment-only lines are kept as Blank/Comment entries, anything
// else is an Unknown entry. Names of the four registration commands are
// upper-cased, as the legacy tool did; a _CDECL_EXPORT_ name keeps its case
// (it is a C symbol).

package xbmac2h

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pablo-botella/linereader"
)

// Kind classifies a line of the .xbmac file.
type Kind int

const (
	Blank       Kind = iota // empty line (after comment removal it was empty but the line had no comment)
	Comment                 // a line holding only a comment
	Unknown                 // anything the parser does not understand
	Fun                     // _XPP_REG_FUN_
	Wmac                    // _XPP_REG_WMAC
	Wst                     // _XPP_REG_WST_
	Wapi                    // _XPP_REG_WAPI
	CdeclExport             // _CDECL_EXPORT_
)

// String returns the kind name.
func (k Kind) String() string {
	switch k {
	case Blank:
		return "Blank"
	case Comment:
		return "Comment"
	case Unknown:
		return "Unknown"
	case Fun:
		return "Fun"
	case Wmac:
		return "Wmac"
	case Wst:
		return "Wst"
	case Wapi:
		return "Wapi"
	case CdeclExport:
		return "CdeclExport"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// IsCommand reports whether the kind is one of the four registration
// commands (the Xbase++ functions: prototype, function-list row and
// decorated .def entries). CdeclExport is not one of them.
func (k Kind) IsCommand() bool { return k >= Fun && k <= Wapi }

// Prefix is the C symbol prefix the legacy tool used for each command.
func (k Kind) Prefix() string {
	switch k {
	case Wmac:
		return "wapimc_"
	case Wst:
		return "wapist_"
	case Wapi:
		return "wapi_"
	}
	return ""
}

// Line is one parsed line.
type Line struct {
	N    int    // 1-based line number
	Kind Kind   // classification
	Name string // command argument: upper-cased for the commands, as written for CdeclExport
	// Comment is the text after "//" without the slashes, trimmed ("" if none).
	Comment string
	// Text is the original line without its terminator (useful for Unknown).
	Text string
}

// Symbol returns the C symbol for a command line (prefix + upper-cased name).
func (l Line) Symbol() string { return l.Kind.Prefix() + l.Name }

// File is a parsed .xbmac file.
type File struct {
	Lines []Line
}

// Commands returns the command lines only, in file order.
func (f *File) Commands() []Line {
	var out []Line
	for _, l := range f.Lines {
		if l.Kind.IsCommand() {
			out = append(out, l)
		}
	}
	return out
}

// Unknowns returns the lines the parser did not understand.
func (f *File) Unknowns() []Line {
	var out []Line
	for _, l := range f.Lines {
		if l.Kind == Unknown {
			out = append(out, l)
		}
	}
	return out
}

// ParseFile reads and parses a .xbmac file.
func ParseFile(path string) (*File, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return Parse(fh)
}

// Parse parses a .xbmac from r. It never fails on content: unrecognised
// lines become Unknown entries; only I/O errors are returned.
func Parse(r io.Reader) (*File, error) {
	lr := linereader.NewLineReader(r, 0, 0)
	f := &File{}
	n := 0
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		n++
		f.Lines = append(f.Lines, parseLine(n, string(raw)))
	}
	return f, nil
}

// parseLine mirrors xbmac2h.prg ParseLine: strip "//" comment, remove all
// blanks, remove ")", split at "(", match the upper-cased command. The
// argument is upper-cased for the four registration commands and kept as
// written for _CDECL_EXPORT_.
func parseLine(n int, text string) Line {
	l := Line{N: n, Text: text}
	code := text
	if i := strings.Index(code, "//"); i >= 0 {
		l.Comment = strings.TrimSpace(code[i+2:])
		code = code[:i]
	}
	code = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, code)
	if code == "" {
		if strings.Contains(text, "//") {
			l.Kind = Comment
		} else {
			l.Kind = Blank
		}
		return l
	}
	code = strings.ReplaceAll(code, ")", "")
	i := strings.IndexByte(code, '(')
	if i < 0 {
		l.Kind = Unknown
		return l
	}
	cmd, arg := strings.ToUpper(code[:i]), code[i+1:]
	switch cmd {
	case "_XPP_REG_FUN_":
		l.Kind = Fun
	case "_XPP_REG_WMAC":
		l.Kind = Wmac
	case "_XPP_REG_WST_":
		l.Kind = Wst
	case "_XPP_REG_WAPI":
		l.Kind = Wapi
	case "_CDECL_EXPORT_":
		l.Kind = CdeclExport
		l.Name = arg
		return l
	default:
		l.Kind = Unknown
		return l
	}
	l.Name = strings.ToUpper(arg)
	return l
}
