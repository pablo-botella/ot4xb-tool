package scandoc

import "fmt"

// Severity of a diagnostic: lint findings are warnings, grammar violations
// (unbalanced scopes, malformed markers, dangling includes) are errors.
type Severity int

const (
	Warning Severity = iota
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warning"
}

// Diag is one problem found while scanning or resolving; the scan itself
// never aborts on them.
type Diag struct {
	Severity Severity
	Line     int
	Msg      string
}

func (d Diag) String() string {
	return fmt.Sprintf("line %d: %s: %s", d.Line, d.Severity, d.Msg)
}

// Errors returns how many of the file's diagnostics are errors.
func (f *File) Errors() int {
	n := 0
	for _, d := range f.Diags {
		if d.Severity == Error {
			n++
		}
	}
	return n
}
