package docdb

import (
	"path/filepath"
	"testing"
)

func open(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func count(t *testing.T, d *DB, table string) int64 {
	t.Helper()
	var n int64
	if err := d.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNormalizeSrc(t *testing.T) {
	for in, want := range map[string]string{
		`src\TBinFile.cpp`:   "src/TBinFile.cpp",
		"./src//a.cpp/":      "src/a.cpp",
		"ch/src/ot4xb.chsrc": "ch/src/ot4xb.chsrc",
	} {
		got, err := NormalizeSrc(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeSrc(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "src/a b.cpp", "src/ñ.cpp", "c:/x.cpp"} {
		if _, err := NormalizeSrc(bad); err == nil {
			t.Fatalf("NormalizeSrc(%q): expected an error", bad)
		}
	}
}

func TestPackPos(t *testing.T) {
	p := PackPos(3, 17)
	if f, s := UnpackPos(p); f != 3 || s != 17 {
		t.Fatalf("pack/unpack = %d/%d", f, s)
	}
	// ordering: any segment of file 2 sorts before any segment of file 3
	if PackPos(2, PosLowMask) >= PackPos(3, 0) {
		t.Fatal("file order must dominate")
	}
}

func TestSourceGetOrCreate(t *testing.T) {
	d := open(t)
	id1, pos1, created, err := d.Source("src/A.cpp")
	if err != nil || !created || pos1 != 1 {
		t.Fatalf("first: id=%d pos=%d created=%v err=%v", id1, pos1, created, err)
	}
	id2, pos2, created, _ := d.Source("src/b.cpp")
	if !created || pos2 != 2 || id2 == id1 {
		t.Fatalf("second: id=%d pos=%d created=%v", id2, pos2, created)
	}
	// same file, different case and separator: the SAME source, same pos
	id3, pos3, created, _ := d.Source(`SRC\a.CPP`)
	if created || id3 != id1 || pos3 != pos1 {
		t.Fatalf("case-insensitive lookup failed: id=%d pos=%d created=%v", id3, pos3, created)
	}
}

func TestTopicSegmentsReplace(t *testing.T) {
	d := open(t)
	src, fpos, _, _ := d.Source("src/a.cpp")
	other, opos, _, _ := d.Source("src/b.cpp")

	// topic created on first sight, reused after
	tp, err := d.Topic("function", "FOO", "Foo")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := d.Topic("function", "FOO", "foo"); again != tp {
		t.Fatalf("topic not reused: %d vs %d", again, tp)
	}
	var ident string
	d.QueryRow(`SELECT ident FROM topics WHERE idtopic = ?`, tp).Scan(&ident)
	if ident != "Foo" {
		t.Fatalf("ident must keep the first spelling, got %q", ident)
	}

	// two sources contribute segments to the same topic (REOPEN / multi-source)
	s1, _ := d.AddSegment(tp, src, PackPos(fpos, 1), 10, []byte("/*{{function: Foo}}*/"))
	s2, _ := d.AddSegment(tp, other, PackPos(opos, 1), 5, []byte("/*{{function: Foo | desc: more}}*/"))
	if err := d.AddReference(s1, src, tp, "note", "shared-one", "include"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddIssue(src, s1, 10, "warning", "todo", "todo field"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddCategory(src, tp, "Container/Dictionary"); err != nil {
		t.Fatal(err)
	}
	_ = s2
	if count(t, d, "segments") != 2 || count(t, d, "refs") != 1 || count(t, d, "issues") != 1 || count(t, d, "topic_category") != 1 {
		t.Fatal("unexpected row counts after inserts")
	}
	var cat string
	d.QueryRow(`SELECT category FROM topic_category`).Scan(&cat)
	if cat != "container/dictionary" {
		t.Fatalf("category not lowercased: %q", cat)
	}

	// ordering of a topic's segments follows (file pos, pos in file) with a plain ORDER BY
	rows, err := d.QueryAll(`SELECT idsrc FROM segments WHERE idtopic = ? ORDER BY pos`, tp)
	if err != nil {
		t.Fatal(err)
	}
	var order []int64
	for _, r := range rows {
		order = append(order, r[0].(int64))
	}
	if len(order) != 2 || order[0] != src || order[1] != other {
		t.Fatalf("segment order = %v, want [%d %d]", order, src, other)
	}

	// resolution is a-posteriori: raw is there from compile, resolved comes later
	if err := d.SetResolved(s2, []byte("resolved content")); err != nil {
		t.Fatal(err)
	}
	var flag int64
	d.QueryRow(`SELECT is_resolved FROM segments WHERE idseg = ?`, s2).Scan(&flag)
	if flag != 1 {
		t.Fatalf("is_resolved = %d after SetResolved", flag)
	}

	// rescanning src: everything it contributed goes, the other source's stays,
	// the source row and the topic survive - and every resolved flag is cleared
	// (references cross sources)
	if err := d.ReplaceSource(src); err != nil {
		t.Fatal(err)
	}
	d.QueryRow(`SELECT is_resolved FROM segments WHERE idseg = ?`, s2).Scan(&flag)
	if flag != 0 {
		t.Fatalf("is_resolved = %d after another source was replaced, want 0", flag)
	}
	if count(t, d, "segments") != 1 || count(t, d, "refs") != 0 || count(t, d, "issues") != 0 || count(t, d, "topic_category") != 0 {
		t.Fatal("ReplaceSource did not delete exactly the source's rows")
	}
	if count(t, d, "sources") != 2 || count(t, d, "topics") != 1 {
		t.Fatal("ReplaceSource must keep sources and topics")
	}
	id, pos, created, _ := d.Source("src/a.cpp")
	if created || id != src || pos != fpos {
		t.Fatal("source must keep its id and pos across a rescan")
	}

	// the other source's last contribution removed -> the topic becomes an orphan
	d.ReplaceSource(other)
	if n, _ := d.PruneOrphanTopics(); n != 1 || count(t, d, "topics") != 0 {
		t.Fatalf("prune removed %d, topics left %d", n, count(t, d, "topics"))
	}
}

func TestMetaAndFTS5(t *testing.T) {
	d := open(t)
	var v string
	d.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	if v != SchemaVersion {
		t.Fatalf("schema_version = %q", v)
	}
	if err := d.Meta("project", "ot4xb"); err != nil {
		t.Fatal(err)
	}
	// informative: the doc_fts lookup needs FTS5; report what this build ships
	t.Logf("FTS5 available: %v", d.HasFTS5())
}
