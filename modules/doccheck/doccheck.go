// Package doccheck cross-checks the in-source documentation (parsed by
// scandoc) against the DLL's registration list (parsed by xbmac2h): every
// Xbase++ function/structure that the .xbmac registers should be documented,
// and every documented function/structure/c-function should correspond to a
// registration. It answers the release question "is the public surface
// documented?" and catches stale or misspelled doc.
//
// Kind mapping (verified against the real ot4xb tree):
//   - _XPP_REG_FUN_ and _XPP_REG_WMAC -> an Xbase++ function, documented as
//     function: or internal-function: (case-insensitive).
//   - _XPP_REG_WST_ -> a GWST structure, documented as structure: or (the
//     not-yet-migrated form) class-name: (case-insensitive).
//   - _CDECL_EXPORT_ -> a plain C export, documented as c-function:
//     (case-SENSITIVE - a C symbol).
package doccheck

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/scandoc"
	"github.com/pablo-botella/ot4xb-tool/modules/xbmac2h"
)

// Miss is one cross-check finding: a name present on one side and missing on
// the other.
type Miss struct {
	Name  string // the registered or documented name, as written
	Group string // "function", "structure" or "c-function"
	Where string // the .xbmac SRC hint, or the doc file:line
}

// Report is the outcome of a cross-check.
type Report struct {
	RegisteredUndocumented     []Miss // in the .xbmac, no matching doc entity
	DocumentedUnregistered     []Miss // a documented entity with no matching registration
	NRegFun, NRegStruct, NRegC int    // registration counts, for the summary
	NDocFun, NDocStruct, NDocC int    // documented-entity counts
}

// normXbase folds an Xbase++ identity for comparison (case-insensitive, blanks
// removed); C symbols keep their case, so normC only strips blanks.
func normXbase(s string) string { return strings.ToLower(stripBlanks(s)) }
func normC(s string) string     { return stripBlanks(s) }

func stripBlanks(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// Check runs the cross-check over the scanned sources and the parsed .xbmac.
func Check(files []*scandoc.File, mac *xbmac2h.File) Report {
	// Documented sides, keyed for lookup; keep a display value for reporting.
	docFun := map[string]string{}    // Xbase++ functions (function + internal-function)
	docStruct := map[string]string{} // structures + class-name
	docC := map[string]string{}      // c-functions (case-sensitive)
	var rep Report

	for _, f := range files {
		for i := range f.Entities {
			e := &f.Entities[i]
			if e.Ident == "" {
				continue
			}
			where := fmt.Sprintf("%s:%d", f.Path, e.StartLine)
			switch e.Kind {
			case scandoc.KindFunction, scandoc.KindInternalFunction:
				docFun[normXbase(e.Ident)] = where
				rep.NDocFun++
			case scandoc.KindStructure, scandoc.KindClass:
				docStruct[normXbase(e.Ident)] = where
				rep.NDocStruct++
			case scandoc.KindCFunction:
				docC[normC(e.Ident)] = where
				rep.NDocC++
			}
		}
	}

	// Registered side; a registration is matched, or it is undocumented.
	regFun := map[string]bool{}
	for _, l := range mac.Commands() {
		src := l.Comment // the "SRC: file.cpp" hint
		switch l.Kind {
		case xbmac2h.Fun, xbmac2h.Wmac:
			rep.NRegFun++
			k := normXbase(l.Name)
			regFun[k] = true
			// A registered function is documented as function:/internal-function:,
			// OR as a class/structure - in Xbase++ a class NAME is itself the
			// registered constructor function (e.g. _LARGE_INTEGER_).
			_, ok := docFun[k]
			if !ok {
				_, ok = docStruct[k]
			}
			if !ok {
				rep.RegisteredUndocumented = append(rep.RegisteredUndocumented, Miss{l.Name, "function", src})
			}
		case xbmac2h.Wst:
			rep.NRegStruct++
			if _, ok := docStruct[normXbase(l.Name)]; !ok {
				rep.RegisteredUndocumented = append(rep.RegisteredUndocumented, Miss{l.Name, "structure", src})
			}
		}
	}
	// _CDECL_EXPORT_ lines are not "commands"; walk the raw lines for them.
	for _, l := range mac.Lines {
		if l.Kind == xbmac2h.CdeclExport {
			rep.NRegC++
			if _, ok := docC[normC(l.Name)]; !ok {
				rep.RegisteredUndocumented = append(rep.RegisteredUndocumented, Miss{l.Name, "c-function", l.Comment})
			}
		}
	}

	// Reverse: a documented FUNCTION with no FUN/WMAC registration is suspect
	// (not exported, or a misspelled name). Structures and c-functions are NOT
	// checked in reverse: they reach the DLL through other registries (the WAPIST
	// map, the .def / import library), not the .xbmac, so comparing them here
	// would be the wrong comparison and pure noise.
	for key, where := range docFun {
		if !regFun[key] {
			rep.DocumentedUnregistered = append(rep.DocumentedUnregistered, Miss{key, "function", where})
		}
	}
	sortMisses(rep.RegisteredUndocumented)
	sortMisses(rep.DocumentedUnregistered)
	return rep
}

func sortMisses(m []Miss) {
	sort.Slice(m, func(i, j int) bool {
		if m[i].Group != m[j].Group {
			return m[i].Group < m[j].Group
		}
		return strings.ToLower(m[i].Name) < strings.ToLower(m[j].Name)
	})
}
