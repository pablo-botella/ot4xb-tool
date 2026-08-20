// script.go is the producer side of cbk2obj: it parses a callback script
// (.cbk) into a Script, the structure the generators consume.
//
// The file is read line by line with github.com/pablo-botella/linereader
// (CR, LF and CRLF alike), as bytes (ANSI, no transcoding). Parsing is a
// hand-written scan over the bytes of each line; no regular expressions.
// Errors do not stop the parse: every faulty line yields a Diag with its
// line number and the legacy message, and parsing goes on, as xppcbk did.

package cbk2obj

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pablo-botella/linereader"
)

// LegacyVersion is the version of the legacy xppcbk whose scripts this
// package accepts: "XPPCBK VERSION v" lines with v above it are errors.
const LegacyVersion = "001.000.017"

// Kind is the C type of a callback parameter or result.
type Kind int

const (
	Void   Kind = iota // VOID (results only)
	Byte               // BYTE, CHAR
	Word               // WORD, SHORT, INT16
	DWord              // DWORD, LONG, ULONG, INT, UINT, INT32, LRESULT, LPARAM, POINTER32, POINTER, HANDLE, LPSTR
	QWord              // QWORD, LONGLONG, ULONGLONG, INT64
	Bool               // BOOL
	Double             // DOUBLE
	Float              // FLOAT
)

var kindNames = [...]string{"VOID", "BYTE", "WORD", "DWORD", "QWORD", "BOOL", "DOUBLE", "FLOAT"}

// String returns the canonical type name.
func (k Kind) String() string {
	if k >= 0 && int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Size is the number of bytes the value takes on the stack: 8 for QWORD
// and DOUBLE, 4 for everything else.
func (k Kind) Size() int {
	if k == QWord || k == Double {
		return 8
	}
	return 4
}

// IsInt reports whether the kind is an integer handled through _conPutNL
// (BYTE, WORD, DWORD).
func (k Kind) IsInt() bool { return k == Byte || k == Word || k == DWord }

// KindOf maps a type name (any case) to its Kind.
func KindOf(name string) (Kind, bool) {
	switch strings.ToUpper(name) {
	case "VOID":
		return Void, true
	case "BYTE", "CHAR":
		return Byte, true
	case "WORD", "SHORT", "INT16":
		return Word, true
	case "DWORD", "LONG", "ULONG", "INT", "UINT", "INT32", "LRESULT", "LPARAM",
		"POINTER32", "POINTER", "HANDLE", "LPSTR":
		return DWord, true
	case "QWORD", "LONGLONG", "ULONGLONG", "INT64":
		return QWord, true
	case "BOOL":
		return Bool, true
	case "DOUBLE":
		return Double, true
	case "FLOAT":
		return Float, true
	}
	return 0, false
}

// CallConv is the calling convention of the thunk.
type CallConv int

const (
	StdCall CallConv = iota // the callee pops its arguments (ret N)
	CDecl                   // the caller pops (ret 0)
)

// String returns "stdcall" or "cdecl".
func (c CallConv) String() string {
	if c == CDecl {
		return "cdecl"
	}
	return "stdcall"
}

// Callback is one callback of the script.
type Callback struct {
	Name   string   // PRG function name as written (case kept)
	Ret    Kind     // result type
	Params []Kind   // parameter types in declaration order
	Conv   CallConv // calling convention of the thunk
	Line   int      // 1-based line of the CALLBACK / BEGIN CALLBACK command
}

// Script is a parsed callback script.
type Script struct {
	Callbacks []Callback
	// Version is the argument of the last "XPPCBK VERSION" line ("" if none).
	Version string
}

// Diag is one script error; the legacy tool printed it as
// "line: <n>  error: <msg>".
type Diag struct {
	Line int
	Msg  string
}

// String formats the diagnostic the way xppcbk printed it.
func (d Diag) String() string { return fmt.Sprintf("line: %d  error: %s", d.Line, d.Msg) }

// Template is a predefined callback signature of the CALLBACK command.
type Template struct {
	Ret    Kind
	Params []Kind
}

var (
	tplWnd  = Template{DWord, []Kind{DWord, DWord, DWord, DWord}} // hWnd, nMsg, wParam, lParam
	tplHook = Template{DWord, []Kind{DWord, DWord, DWord}}        // nCode, wParam, lParam
	tplEnum = Template{Bool, []Kind{DWord, DWord}}                // hWnd, lParam
)

// Templates are the predefined signatures, by upper-case template name.
var Templates = map[string]Template{
	"WNDPROC":              tplWnd,
	"DIALOGPROC":           tplWnd,
	"MSGBOXCALLBACK":       {Void, []Kind{DWord}}, // HELPINFO*
	"CCHOOKPROC":           tplWnd,
	"CFHOOKPROC":           tplWnd,
	"FRHOOKPROC":           tplWnd,
	"OFNHOOKPROC":          tplWnd,
	"OFNHOOKPROCOLDSTYLE":  tplWnd,
	"PAGEPAINTHOOK":        tplWnd,
	"PAGESETUPHOOK":        tplWnd,
	"PRINTHOOKPROC":        tplWnd,
	"SETUPHOOKPROC":        tplWnd,
	"SENDASYNCPROC":        tplWnd, // hWnd, nMsg, dwData, lResult
	"TIMERPROC":            tplWnd, // hWnd, nMsg, idEvent, dwTime
	"ENUMCHILDPROC":        tplEnum,
	"ENUMTHREADWNDPROC":    tplEnum,
	"ENUMWINDOWSPROC":      tplEnum,
	"CALLWNDPROC":          tplHook,
	"CALLWNDRETPROC":       tplHook,
	"CBTPROC":              tplHook,
	"DEBUGPROC":            tplHook,
	"FOREGROUNDIDLEPROC":   tplHook,
	"GETMSGPROC":           tplHook,
	"JOURNALPLAYBACKPROC":  tplHook,
	"JOURNALRECORDPROC":    tplHook,
	"KEYBOARDPROC":         tplHook,
	"LOWLEVELKEYBOARDPROC": tplHook,
	"LOWLEVELMOUSEPROC":    tplHook,
	"MESSAGEPROC":          tplHook,
	"MOUSEPROC":            tplHook,
	"SHELLPROC":            tplHook,
	"SYSMSGPROC":           tplHook,
}

// ParseFile reads and parses a .cbk file.
func ParseFile(path string) (*Script, []Diag, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer fh.Close()
	return Parse(fh)
}

// Parse parses a callback script from r. Script errors come back as Diags
// (the Script then holds whatever was understood); the error is for I/O only.
func Parse(r io.Reader) (*Script, []Diag, error) {
	lr := linereader.NewLineReader(r, 0, 0)
	p := &parser{script: &Script{}}
	n := 0
	for {
		raw, err := lr.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, nil, err
		}
		n++
		p.line(n, raw)
	}
	if p.open != nil {
		p.errorf(n, "Unclosed control structures")
	}
	return p.script, p.diags, nil
}

// parser holds the state of one parse: the default calling convention and
// the callback a BEGIN CALLBACK left open.
type parser struct {
	script *Script
	diags  []Diag
	conv   CallConv
	open   *Callback // callback being defined, nil outside BEGIN/END
}

func (p *parser) errorf(line int, format string, a ...any) {
	p.diags = append(p.diags, Diag{Line: line, Msg: fmt.Sprintf(format, a...)})
}

// line handles one script line: strip the comment, split into tokens,
// dispatch on the upper-cased command and sub-command.
func (p *parser) line(n int, raw []byte) {
	tok := tokens(raw)
	if len(tok) == 0 {
		return
	}
	cmd := strings.ToUpper(tok[0])
	sub := ""
	if len(tok) > 1 {
		sub = strings.ToUpper(tok[1])
	}
	switch {
	case cmd == "USING" && sub == "STDCALL", cmd == "__STDCALL":
		p.conv = StdCall
	case cmd == "USING" && sub == "CDECL", cmd == "__CDECL":
		p.conv = CDecl
	case cmd == "CALLBACK" && sub != "":
		t, ok := Templates[sub]
		if !ok {
			p.errorf(n, "unknow template")
			return
		}
		if len(tok) < 3 {
			p.errorf(n, "Bad Syntax")
			return
		}
		if p.begin(n, tok[2], t.Ret) {
			p.open.Params = append(p.open.Params, t.Params...)
			p.end(n, StdCall)
		}
	case cmd == "BEGIN" && sub == "CALLBACK":
		// BEGIN CALLBACK <name> <word...> <TYPE>: at least one word between
		// the name and the type (RETURNS, AS, ...), the type is the last token.
		if len(tok) < 5 {
			p.errorf(n, "Bad Syntax")
			return
		}
		typ := tok[len(tok)-1]
		k, ok := KindOf(typ)
		if !ok {
			p.errorf(n, "invalid type %s", strings.ToUpper(typ))
			return
		}
		p.begin(n, tok[2], k)
	case cmd == "END" && sub == "CALLBACK":
		p.end(n, p.conv)
		p.conv = StdCall // the legacy reset: a USING covers one callback
	case cmd == "PARAM":
		if len(tok) < 2 {
			p.errorf(n, "Bad Syntax")
			return
		}
		k, ok := KindOf(tok[1])
		if !ok || k == Void {
			p.errorf(n, "invalid type %s", sub)
			return
		}
		if p.open == nil {
			p.errorf(n, "PARAM defined outside CALLBACK")
			return
		}
		p.open.Params = append(p.open.Params, k)
	case cmd == "XPPCBK" && sub == "VERSION":
		if len(tok) < 3 {
			p.errorf(n, "Bad Syntax")
			return
		}
		v := strings.ToUpper(tok[2])
		p.script.Version = v
		if v > LegacyVersion {
			p.errorf(n, "THIS SCRIPT REQUIRE XPPCBK VERSION >= %s", v)
		}
	default:
		p.errorf(n, "unknow command")
	}
}

// begin opens a callback; false when it could not be opened (error issued).
func (p *parser) begin(n int, name string, ret Kind) bool {
	if p.open != nil {
		p.errorf(n, "Unclosed control structures")
		return false
	}
	if !validName(name) {
		p.errorf(n, "invalid function name %s", name)
		return false
	}
	for _, cb := range p.script.Callbacks {
		if strings.EqualFold(cb.Name, name) {
			p.errorf(n, "Dupe function name %s", name)
			return false
		}
	}
	p.open = &Callback{Name: name, Ret: ret, Line: n}
	return true
}

// end closes the open callback with conv.
func (p *parser) end(n int, conv CallConv) {
	if p.open == nil {
		p.errorf(n, "END CALLBACK no match BEGIN CALLBACK")
		return
	}
	p.open.Conv = conv
	p.script.Callbacks = append(p.script.Callbacks, *p.open)
	p.open = nil
}

// tokens drops the "//" comment and splits the line at blanks and tabs.
func tokens(raw []byte) []string {
	var out []string
	start := -1
	for i := 0; i <= len(raw); i++ {
		end := i == len(raw) || (raw[i] == '/' && i+1 < len(raw) && raw[i+1] == '/')
		if end || raw[i] == ' ' || raw[i] == '\t' {
			if start >= 0 {
				out = append(out, string(raw[start:i]))
				start = -1
			}
			if end {
				break
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return out
}

// validName accepts a PRG function name: a letter or underscore, then
// letters, digits and underscores.
func validName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
