package doctool

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `{
	"books": [
		{ "name": "c", "title": "C API", "prefix": "c-",
		  "indexes": [ { "slug": "index-c", "title": "C API", "sections": [
		    { "title": "Functions", "include": [ { "kind": "c-function", "category": "*" } ], "by": "name" } ] } ] },
		{ "name": "cpp", "title": "C/C++", "prefix": "cpp-", "includes": [ "c" ] },
		{ "name": "xbase", "title": "Xbase++", "prefix": "" }
	],
	"index": { "slug": "index", "title": "Reference", "sections": [ { "title": "Manuals", "books": [ "xbase", "cpp" ] } ] },
	"kinds": [
		{ "name": "function", "book": "xbase", "case_sensitive": false, "case_index": "upper", "tag": "function" },
		{ "name": "class", "book": "xbase", "case_sensitive": false, "case_index": "upper", "tag": "class", "aliases": [ "class-name" ] },
		{ "name": "c-function", "book": "c", "case_sensitive": true, "tag": "function" },
		{ "name": "topic", "book": null, "book_default": "xbase", "case_sensitive": false, "case_index": "lower", "tag": "topic" },
		{ "name": "note-id", "book": null, "case_sensitive": false, "case_index": "lower", "tag": "note", "indexed": false }
	],
	"folders": { "src": "./source", "out": "./out" },
	"src": [ "<src>/*.cpp", "<src>/ch/*.ch" ],
	"db": "<out>/x.db"
}`

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.doc-tool")
	os.WriteFile(p, []byte(sample), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind("class-name") == nil || c.Kind("class-name").Name != "class" {
		t.Fatal("alias")
	}
	kt := c.KindTable()
	if kt.HeaderKind("class-name") != "class" || kt.Key("function", "aBc") != "ABC" || kt.Key("c-function", "aBc") != "aBc" || kt.Key("topic", "X") != "x" {
		t.Fatal("kind table")
	}
	if got := c.BooksOf("topic", nil); len(got) != 1 || got[0] != "xbase" {
		t.Fatalf("topic default book: %v", got)
	}
	if got := c.BooksOf("topic", []string{"c", "cpp"}); len(got) != 2 {
		t.Fatalf("topic named books: %v", got)
	}
	if got := c.BooksOf("note-id", nil); len(got) != 0 {
		t.Fatalf("note books: %v", got)
	}
	if !c.Contains("cpp", "c") || c.Contains("c", "cpp") || !c.Contains("c", "c") {
		t.Fatal("includes")
	}
	srcs, err := c.Sources()
	if err != nil || len(srcs) != 2 || srcs[0] != filepath.Join(dir, "source", "*.cpp") {
		t.Fatalf("sources: %v %v", srcs, err)
	}
	if db, _ := c.DBPath(); db != filepath.Join(dir, "out", "x.db") {
		t.Fatalf("db: %s", db)
	}
	if !MatchCategory("*", nil) || MatchCategory("c-api*", nil) || !MatchCategory("c-api*,other", []string{"C-API/tlist"}) || MatchCategory("container*", []string{"c-api/container"}) {
		t.Fatal("masks")
	}
	if len(c.Book("xbase").Indexes) != 0 || c.Index.Sections[0].Books[1] != "cpp" {
		t.Fatal("defaults")
	}
	bad := []string{
		`{"books":[{"name":"c"}],"kinds":[{"name":"f","book":"nope","case_sensitive":true}]}`,
		`{"books":[{"name":"c","includes":["c"]}],"kinds":[{"name":"f","book":"c","case_sensitive":true}]}`,
		`{"books":[{"name":"c"}],"kinds":[{"name":"f","book":"c","case_sensitive":false}]}`,
		`{"books":[{"name":"c","indexes":[{"slug":"i","sections":[{"include":[{"kind":"zz"}]}]}]}],"kinds":[{"name":"f","book":"c","case_sensitive":true}]}`,
	}
	for i, b := range bad {
		os.WriteFile(p, []byte(b), 0o644)
		if _, err := Load(p); err == nil {
			t.Errorf("bad %d accepted", i)
		}
	}
	if Default().Kind("class-name") == nil {
		t.Fatal("default")
	}
}

// TestUserCompanion checks the personal ".user" file replaces the members it
// declares: the paths of the machine that collects (db, folders, src) live
// there, out of the tracked .doc-tool.
func TestUserCompanion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.doc-tool")
	os.WriteFile(p, []byte(sample), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	db, _ := c.DBPath()
	if db != filepath.Join(dir, "out", "x.db") {
		t.Fatalf("without .user: %s", db)
	}
	os.WriteFile(p+".user", []byte(`{"db": "<out>/mine.db", "folders": {"out": "../elsewhere"}}`), 0o644)
	c, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if db, _ = c.DBPath(); db != filepath.Join(filepath.Dir(dir), "elsewhere", "mine.db") {
		t.Fatalf("with .user: %s", db)
	}
	// the shared blocks are untouched by the .user
	if c.Kind("class-name") == nil {
		t.Fatal("the .user must not disturb the kinds")
	}
}

// TestBlocksRoundTrip is the whole point of the cfg table: a .doc-tool that
// declares only local paths loads when the shared blocks come from elsewhere.
func TestBlocksRoundTrip(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, "full.doc-tool")
	os.WriteFile(full, []byte(sample), 0o644)
	src, err := Load(full)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := src.Blocks()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range SharedBlocks {
		if blocks[b] == "" {
			t.Fatalf("block %q is empty", b)
		}
	}
	if len(blocks) != 3 {
		t.Fatalf("only the shared blocks travel: %v", blocks)
	}

	local := filepath.Join(dir, "local.doc-tool")
	os.WriteFile(local, []byte(`{"folders": {"out": "./o"}, "db": "<out>/d.db", "src": ["a.cpp"]}`), 0o644)
	if _, err := Load(local); err == nil {
		t.Fatal("a .doc-tool without kinds cannot load on its own")
	}
	get := func(b string) ([]byte, bool, error) {
		v, ok := blocks[b]
		return []byte(v), ok, nil
	}
	c, err := LoadWith(local, get)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind("class-name") == nil || c.Kind("class-name").Name != "class" {
		t.Fatal("kinds did not come from the blocks")
	}
	if got := c.BooksOf("topic", nil); len(got) != 1 || got[0] != "xbase" {
		t.Fatalf("books did not come from the blocks: %v", got)
	}
	if c.Index.Title != "Reference" {
		t.Fatalf("index did not come from the blocks: %q", c.Index.Title)
	}
	// the local paths stay local: they are not in the blocks
	if db, _ := c.DBPath(); db != filepath.Join(dir, "o", "d.db") {
		t.Fatalf("local db lost: %s", db)
	}
	// what the file declares wins over what the database offers
	if _, err := LoadWith(full, func(string) ([]byte, bool, error) {
		return []byte(`[]`), true, nil
	}); err != nil {
		t.Fatalf("the file must win over get: %v", err)
	}
	// with no file at all, everything comes from the blocks
	if _, err := LoadWith("", get); err != nil {
		t.Fatalf("blocks alone must be enough: %v", err)
	}
}

// TestPeekDB reads the database of a .doc-tool that cannot be validated yet -
// the blocks it is missing live in that very database.
func TestPeekDB(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "local.doc-tool")
	os.WriteFile(p, []byte(`{"folders": {"out": "./o"}, "db": "<out>/d.db"}`), 0o644)
	got, err := PeekDB(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "o", "d.db") {
		t.Fatalf("got %s", got)
	}
	// the .user wins, and an absolute path in it is taken as it is. Built at
	// run time: a literal like "D:/x" is absolute on Windows and relative
	// everywhere else, which would resolve against the folder of the file.
	abs := filepath.Join(t.TempDir(), "elsewhere", "other.db")
	os.WriteFile(p+".user", []byte(`{"db": "`+filepath.ToSlash(abs)+`"}`), 0o644)
	if got, err = PeekDB(p); err != nil || filepath.ToSlash(got) != filepath.ToSlash(abs) {
		t.Fatalf("the .user must win: %s (%v)", got, err)
	}

	// and a relative one in the .user resolves against the same folder, since
	// both files live side by side
	os.WriteFile(p+".user", []byte(`{"db": "../beside/other.db"}`), 0o644)
	if got, err = PeekDB(p); err != nil || got != filepath.Join(filepath.Dir(dir), "beside", "other.db") {
		t.Fatalf("relative in the .user: %s (%v)", got, err)
	}
}
