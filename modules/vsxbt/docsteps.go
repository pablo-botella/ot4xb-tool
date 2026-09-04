// docsteps.go: the documentation steps of a tool file - "srcsplit" (clean
// sources for a release) and "docs" (compile + resolve + gendoc). The JSON
// decides how and when the documentation is built: which entry runs them,
// with which sources, into which folder.
package vsxbt

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pablo-botella/ot4xb-tool/modules/doccompile"
	"github.com/pablo-botella/ot4xb-tool/modules/docdb"
	"github.com/pablo-botella/ot4xb-tool/modules/docgen"
	"github.com/pablo-botella/ot4xb-tool/modules/docresolve"
	"github.com/pablo-botella/ot4xb-tool/modules/srcsplit"
)

// stepSrcsplit runs srcsplit over "in" (a file, a glob or a directory) writing
// the code projection to "code" and/or the doc projection to "doc" (patterns
// with '*' = the source base name). "force" overwrites a different existing
// output, "bak" keeps a .bak of it; without either a different output is an
// error, as on the command line.
func (t *Tool) stepSrcsplit(e *entry, cfg json.RawMessage, o Options) error {
	// one job, or a list of jobs
	var jobs []json.RawMessage
	if err := json.Unmarshal(cfg, &jobs); err != nil {
		jobs = []json.RawMessage{cfg}
	}
	for _, j := range jobs {
		if err := t.srcsplitJob(e, j, o); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tool) srcsplitJob(e *entry, cfg json.RawMessage, o Options) error {
	var c struct {
		In    string `json:"in"`
		Code  string `json:"code"`
		Doc   string `json:"doc"`
		Force bool   `json:"force"`
		Bak   bool   `json:"bak"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return err
	}
	if c.In == "" {
		return fmt.Errorf("srcsplit: \"in\" missing")
	}
	if c.Code == "" && c.Doc == "" {
		return fmt.Errorf("srcsplit: give \"code\" and/or \"doc\"")
	}
	in, err := t.expandPath(e, c.In)
	if err != nil {
		return err
	}
	opt := srcsplit.Options{Force: c.Force, Bak: c.Bak, Warn: func(m string) { o.Warn("srcsplit: " + m) }}
	if c.Code != "" {
		if opt.Code, err = t.expandPath(e, c.Code); err != nil {
			return err
		}
	}
	if c.Doc != "" {
		if opt.Doc, err = t.expandPath(e, c.Doc); err != nil {
			return err
		}
	}
	res, err := srcsplit.Run(in, opt)
	if err != nil {
		return err
	}
	wrote := 0
	for _, r := range res {
		if r.CodeWrote || r.DocWrote {
			wrote++
		}
	}
	o.Say(fmt.Sprintf("srcsplit: %d source(s), %d output(s) written", len(res), wrote))
	return nil
}

// stepDocs builds the documentation: "src" (one path or a list: files, globs
// or directories, relative to the tool file) compiled into "db" under "root"
// (default: the tool dir), then resolved, then rendered into "out". With
// "clean" (default true) the database is rebuilt from scratch, so sources
// that no longer exist leave nothing behind. Broken references are reported
// as warnings; "strict" turns them into an error.
func (t *Tool) stepDocs(e *entry, cfg json.RawMessage, o Options) error {
	var c struct {
		Root   string          `json:"root"`
		DB     string          `json:"db"`
		Src    json.RawMessage `json:"src"`
		Out    string          `json:"out"`
		Clean  *bool           `json:"clean"`
		Strict bool            `json:"strict"`
	}
	if err := json.Unmarshal(cfg, &c); err != nil {
		return err
	}
	if c.DB == "" || c.Out == "" || len(c.Src) == 0 {
		return fmt.Errorf("docs: \"db\", \"src\" and \"out\" are required")
	}
	root := t.Dir
	var err error
	if c.Root != "" {
		if root, err = t.expandPath(e, c.Root); err != nil {
			return err
		}
	}
	db, err := t.expandPath(e, c.DB)
	if err != nil {
		return err
	}
	out, err := t.expandPath(e, c.Out)
	if err != nil {
		return err
	}
	var srcs []string
	var one string
	if json.Unmarshal(c.Src, &one) == nil {
		srcs = []string{one}
	} else if err := json.Unmarshal(c.Src, &srcs); err != nil {
		return fmt.Errorf("docs: \"src\" must be a string or a list of strings")
	}
	for i, s := range srcs {
		if srcs[i], err = t.expandPath(e, s); err != nil {
			return err
		}
	}
	if c.Clean == nil || *c.Clean {
		if err := os.Remove(db); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	files, err := doccompile.ExpandSources(srcs)
	if err != nil {
		return err
	}
	n, err := doccompile.Compile(db, root, files, nil)
	if err != nil {
		return err
	}
	h, err := docdb.Open(db)
	if err != nil {
		return err
	}
	rep, err := docresolve.Resolve(h)
	if err != nil {
		h.Close()
		return err
	}
	var issues []string
	rows, err := h.QueryAll(`SELECT COALESCE(s.src, ''), i.line, i.severity, i.message FROM issues i
	                         LEFT JOIN sources s USING(idsrc) WHERE i.severity = 'error' ORDER BY s.pos, i.line`)
	h.Close()
	if err != nil {
		return err
	}
	for _, r := range rows {
		issues = append(issues, fmt.Sprintf("%s:%d: %s", r[0].(string), r[1].(int64), r[3].(string)))
	}
	for _, is := range issues {
		o.Warn("docs: " + is)
	}
	pages, err := docgen.Generate(db, out, nil)
	if err != nil {
		return err
	}
	o.Say(fmt.Sprintf("docs: %d source(s) -> %s; %d reference(s), %d missing; %d page(s) -> %s",
		n, db, rep.Refs, rep.Missing, pages, out))
	if c.Strict && len(issues) > 0 {
		return fmt.Errorf("docs: %d error(s) in the documentation (strict)", len(issues))
	}
	return nil
}
