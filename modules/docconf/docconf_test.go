package docconf

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
