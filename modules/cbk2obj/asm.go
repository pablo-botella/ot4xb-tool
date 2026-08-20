// asm.go renders the code model as FASM source ("format MS COFF"), the
// text the legacy xppcbk handed to FASM.EXE: no blank lines, no comments,
// CRLF line ends. It is the debugging view of the object obj.go writes and
// the reference for its verification (assemble it, compare the objects).

package cbk2obj

import (
	"bufio"
	"io"
)

// writeAsm writes the FASM source of u.
func writeAsm(w io.Writer, u *unit) error {
	bw := bufio.NewWriter(w)
	line := func(s string) {
		bw.WriteString(s)
		bw.WriteString("\r\n")
	}
	line("format MS COFF")
	for _, x := range u.externs {
		line("extrn " + x)
	}
	line("section '.text' code readable executable")
	for _, b := range u.blocks {
		if b.public {
			line("public  " + b.label)
		}
	}
	for _, b := range u.blocks {
		line(b.label + ":")
		for _, in := range b.code {
			line(in.text)
		}
	}
	line("section '.data' data readable writeable")
	for _, d := range u.data {
		line(d.text)
	}
	return bw.Flush()
}
