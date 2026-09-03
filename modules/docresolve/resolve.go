// Package docresolve is the a-posteriori step of the documentation database:
// once every source has been compiled, it verifies the references against the
// topics and records what is broken as issues. It is re-runnable: its own
// issues (code "resolve/...") are dropped and recomputed every time.
//
// Rules (spec Draft 3, "resolve"):
//   - A reference target is looked up by (kind, key) with the key rule of the
//     target's family. class and structure are ONE family ("a todos los
//     efectos son clases"): a target written as either resolves to either.
//   - Missing target -> issue "resolve/missing" on the source that wrote the
//     reference. For navigation links this is the only check.
//   - include (transclusion): cycles are forbidden -> issue "resolve/cycle".
//     Repetitions are fine.
//   - see-also and calls are stored unqualified and are NOT resolved for now.
//   - A topic with several segments is legitimate for class / structure /
//     cpp-class (REOPEN) and an issue "resolve/duplicate" for any other kind.
//
// Content resolution (the `resolved` blob and its flag) is a later operation;
// this step only judges references and identities.
package docresolve

import (
	"fmt"

	"github.com/pablo-botella/ot4xb-tool/modules/doccompile"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
)

// Report summarizes one resolve run.
type Report struct {
	Refs       int // references examined
	Skipped    int // unqualified references left alone (see-also, calls)
	Missing    int // references whose target does not exist
	Cycles     int // include cycles found
	Duplicates int // extra segments on non-reopenable topics
}

type topicKey struct{ kind, key string }

type ref struct {
	idseg, idsrc, from int64
	toKind, toIdent    string
	refType            string
}

// family returns the kinds to try for a target written with kind k.
func family(k string) []string {
	switch k {
	case "class", "structure":
		return []string{"class", "structure"}
	}
	return []string{k}
}

func reopenable(kind string) bool {
	return kind == "class" || kind == "structure" || kind == "cpp-class"
}

// Resolve runs the step over an open database.
func Resolve(db *docdb.DB) (Report, error) {
	var rep Report
	if _, err := db.Exec(`DELETE FROM issues WHERE code LIKE 'resolve/%'`); err != nil {
		return rep, err
	}

	// the identity graph
	topics := map[topicKey]int64{}
	kindOf := map[int64]string{}
	identOf := map[int64]string{}
	rows, err := db.QueryAll(`SELECT idtopic, kind, key, ident FROM topics`)
	if err != nil {
		return rep, err
	}
	for _, r := range rows {
		id, kind, key, ident := r[0].(int64), r[1].(string), r[2].(string), r[3].(string)
		topics[topicKey{kind, key}] = id
		kindOf[id] = kind
		identOf[id] = ident
	}

	// segment lines, for citing
	lineOf := map[int64]int64{}
	if rows, err = db.QueryAll(`SELECT idseg, line FROM segments`); err != nil {
		return rep, err
	}
	for _, r := range rows {
		lineOf[r[0].(int64)] = r[1].(int64)
	}

	// the references
	var refs []ref
	if rows, err = db.QueryAll(`SELECT idseg, idsrc, idtopic_in, reftokind, reftoident, reftype FROM refs`); err != nil {
		return rep, err
	}
	for _, r := range rows {
		refs = append(refs, ref{r[0].(int64), r[1].(int64), r[2].(int64), r[3].(string), r[4].(string), r[5].(string)})
	}

	// lookup resolves a written target to its topic id (0 = none) trying the
	// kinds of its family with each kind's key rule.
	lookup := func(toKind, toIdent string) int64 {
		for _, k := range family(toKind) {
			if id, ok := topics[topicKey{k, doccompile.Key(k, toIdent)}]; ok {
				return id
			}
		}
		return 0
	}

	// 1. existence
	type edge struct {
		to int64
		r  ref
	}
	includes := map[int64][]edge{} // from topic -> resolved include edges (for cycles)
	for i := range refs {
		r := refs[i]
		rep.Refs++
		if r.toKind == "" || r.refType == "see-also" || r.refType == "calls" {
			rep.Skipped++
			continue
		}
		found := lookup(r.toKind, r.toIdent)
		if found == 0 {
			rep.Missing++
			msg := fmt.Sprintf("%s %s: target <%s %s> does not exist", r.refType, identOf[r.from], r.toKind, r.toIdent)
			if err := db.AddIssue(r.idsrc, r.idseg, lineOf[r.idseg], "error", "resolve/missing", msg); err != nil {
				return rep, err
			}
			continue
		}
		if r.refType == "include" {
			includes[r.from] = append(includes[r.from], edge{found, r})
		}
	}

	// 2. include cycles: DFS over topic -> included topics
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[int64]int{}
	var stack []int64
	var visit func(id int64) error
	visit = func(id int64) error {
		color[id] = gray
		stack = append(stack, id)
		for _, e := range includes[id] {
			to, r := e.to, e.r
			switch color[to] {
			case white:
				if err := visit(to); err != nil {
					return err
				}
			case gray:
				rep.Cycles++
				path := ""
				for _, s := range stack {
					path += identOf[s] + " -> "
				}
				msg := "include cycle: " + path + identOf[to]
				if err := db.AddIssue(r.idsrc, r.idseg, lineOf[r.idseg], "error", "resolve/cycle", msg); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return nil
	}
	for id := range includes {
		if color[id] == white {
			if err := visit(id); err != nil {
				return rep, err
			}
		}
	}

	// 3. duplicates: several segments on a non-reopenable topic
	if rows, err = db.QueryAll(`SELECT s.idtopic, s.idseg, s.idsrc, s.line FROM segments s ORDER BY s.idtopic, s.pos`); err != nil {
		return rep, err
	}
	type segRow struct{ tp, seg, src, line int64 }
	var dups []segRow
	var prevTopic int64 = -1
	for _, row := range rows {
		r := segRow{row[0].(int64), row[1].(int64), row[2].(int64), row[3].(int64)}
		if r.tp == prevTopic && !reopenable(kindOf[r.tp]) {
			dups = append(dups, r)
		}
		prevTopic = r.tp
	}
	for _, r := range dups {
		rep.Duplicates++
		msg := fmt.Sprintf("%s %s is defined again (only class/structure/cpp-class may reopen)", kindOf[r.tp], identOf[r.tp])
		if err := db.AddIssue(r.src, r.seg, r.line, "error", "resolve/duplicate", msg); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
