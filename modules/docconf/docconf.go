// Package docconf reads a .doc-tool file: the configuration of the
// documentation of a project. The file declares the topic kinds the sources
// may use (name, identity case rules, header aliases, tag), the books the
// pages are filed in (each with its indexes, made of sections that filter by
// kind and category), the general index, and the sources and database the
// docs step compiles. Without a file, Default() reproduces the built-in
// configuration of ot4xb (the nine Draft 4 kinds, four books).
package docconf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pablo-botella/ot4xb-tool/modules/srcdoc"
)

// Kind is one topic kind.
type Kind struct {
	Name            string   `json:"name"`
	Book            string   `json:"book"`             // the book of every topic of the kind; "" = the topic says (field "book:")
	BookDefault     string   `json:"book_default"`     // when Book is "" and the topic says nothing
	CaseSensitive   bool     `json:"case_sensitive"`   // identity compared as written
	CaseIndex       string   `json:"case_index"`       // "upper" | "lower": how a case-insensitive identity is keyed
	Tag             string   `json:"tag"`              // shown after the link in an index listing several kinds
	Aliases         []string `json:"aliases"`          // other header labels ("class-name" for class)
	MultiDefinition bool     `json:"multi_definition"` // informational: the symbol may be documented in several places
	Prototype       bool     `json:"prototype"`        // informational: the identity carries the parameter types
	Indexed         *bool    `json:"indexed"`          // false: never listed (note-id); nil = true
}

// IsIndexed reports whether the kind's topics are listed in the indexes.
func (k *Kind) IsIndexed() bool { return k.Indexed == nil || *k.Indexed }

// Filter is one entry of a section's include or exclude: a kind ("*" = any)
// and a comma list of category masks ("*" = any category, or none).
type Filter struct {
	Kind     string `json:"kind"`
	Category string `json:"category"`
}

// Section is one part of an index page.
type Section struct {
	Title   string   `json:"title"`
	Book    string   `json:"book"` // take the pages from this book instead of the index's
	Include []Filter `json:"include"`
	Exclude []Filter `json:"exclude"`
	By      string   `json:"by"`   // "name" (default) | "category"
	Slug    string   `json:"slug"` // when set, the section is a page of its own and the index links to it
}

// Index is one index page of a book.
type Index struct {
	Slug     string    `json:"slug"`
	Title    string    `json:"title"`
	Sections []Section `json:"sections"`
}

// Book is a set of pages: the topics of its kinds, the topics that name it
// in their "book:" field, and everything of the books it includes.
type Book struct {
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Prefix   string   `json:"prefix"` // of its category page files
	Includes []string `json:"includes"`
	Indexes  []Index  `json:"indexes"`
}

// GeneralSection is one part of the general index: a list of books.
type GeneralSection struct {
	Title string   `json:"title"`
	Books []string `json:"books"`
}

// General is the general index.
type General struct {
	Slug     string           `json:"slug"`
	Title    string           `json:"title"`
	Sections []GeneralSection `json:"sections"`
}

// Config is a whole .doc-tool file.
type Config struct {
	Books   []Book            `json:"books"`
	Index   General           `json:"index"`
	Kinds   []Kind            `json:"kinds"`
	Folders map[string]string `json:"folders"`
	Src     []string          `json:"src"`
	DB      string            `json:"db"`

	// Path and Dir locate the file; "" for Default().
	Path string `json:"-"`
	Dir  string `json:"-"`

	kinds map[string]*Kind // by name and by alias
	books map[string]*Book
}

// Default is the built-in configuration: the nine kinds of Draft 4 with the
// rules ot4xb has always used, and four books (c, cpp, xbase, other) whose
// index lists the pages by category and by name.
func Default() *Config {
	no := false
	c := &Config{
		Books: []Book{
			{Name: "c", Title: "C API", Prefix: "c-", Indexes: []Index{stdIndex("index-c", "C API")}},
			{Name: "cpp", Title: "C++ API", Prefix: "cpp-", Indexes: []Index{stdIndex("index-cpp", "C++ API")}},
			{Name: "xbase", Title: "Xbase++", Prefix: "", Indexes: []Index{stdIndex("index-xbase", "Xbase++")}},
			{Name: "other", Title: "Other", Prefix: "other-", Indexes: []Index{stdIndex("index-other", "Other")}},
		},
		Index: General{Slug: "index", Title: "Index", Sections: []GeneralSection{{Books: []string{"c", "cpp", "xbase", "other"}}}},
		Kinds: []Kind{
			{Name: srcdoc.KindFunction, Book: "xbase", CaseIndex: "upper", Tag: "function"},
			{Name: srcdoc.KindInternalFunction, Book: "xbase", CaseIndex: "upper", Tag: "internal"},
			{Name: srcdoc.KindClass, Book: "xbase", CaseIndex: "upper", Tag: "class", Aliases: []string{"class-name"}},
			{Name: srcdoc.KindCFunction, Book: "c", CaseSensitive: true, Tag: "function", MultiDefinition: true},
			{Name: srcdoc.KindDebugCFunction, Book: "c", CaseSensitive: true, Tag: "debug", MultiDefinition: true},
			{Name: srcdoc.KindCppFunction, Book: "cpp", CaseSensitive: true, Tag: "function", MultiDefinition: true, Prototype: true},
			{Name: srcdoc.KindCppClass, Book: "cpp", CaseSensitive: true, Tag: "class", MultiDefinition: true},
			{Name: srcdoc.KindTopic, BookDefault: "other", CaseIndex: "lower", Tag: "topic"},
			{Name: srcdoc.KindNote, CaseIndex: "lower", Tag: "note", Indexed: &no},
		},
	}
	if err := c.check(); err != nil {
		panic(err)
	}
	return c
}

func stdIndex(slug, title string) Index {
	all := []Filter{{Kind: "*", Category: "*"}}
	return Index{Slug: slug, Title: title, Sections: []Section{
		{Title: "By category", Include: all, By: "category"},
		{Title: "Alphabetic", Include: all, By: "name", Slug: slug + "-alpha"},
	}}
}

// Load reads and checks a .doc-tool file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	c.Path = abs
	c.Dir = filepath.Dir(abs)
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// check validates the configuration and builds the lookup maps.
func (c *Config) check() error {
	c.kinds = map[string]*Kind{}
	c.books = map[string]*Book{}
	if len(c.Kinds) == 0 {
		return fmt.Errorf("no kinds")
	}
	for i := range c.Books {
		b := &c.Books[i]
		if !nameRe.MatchString(b.Name) {
			return fmt.Errorf("book %q: bad name", b.Name)
		}
		if c.books[b.Name] != nil {
			return fmt.Errorf("book %q declared twice", b.Name)
		}
		c.books[b.Name] = b
		if b.Title == "" {
			b.Title = b.Name
		}
	}
	for i := range c.Kinds {
		k := &c.Kinds[i]
		if !nameRe.MatchString(k.Name) {
			return fmt.Errorf("kind %q: bad name", k.Name)
		}
		if c.kinds[k.Name] != nil {
			return fmt.Errorf("kind %q declared twice", k.Name)
		}
		if strings.HasPrefix(k.Name, "begin-") || strings.HasPrefix(k.Name, "end-") || k.Name == "slug" || k.Name == "tg" {
			return fmt.Errorf("kind %q: reserved name", k.Name)
		}
		c.kinds[k.Name] = k
		for _, a := range k.Aliases {
			if !nameRe.MatchString(a) {
				return fmt.Errorf("kind %q: bad alias %q", k.Name, a)
			}
			if c.kinds[a] != nil {
				return fmt.Errorf("kind %q: alias %q is already a kind or alias", k.Name, a)
			}
			c.kinds[a] = k
		}
		if k.CaseSensitive {
			k.CaseIndex = ""
		} else if k.CaseIndex != "upper" && k.CaseIndex != "lower" {
			return fmt.Errorf("kind %q: case_sensitive false needs case_index \"upper\" or \"lower\"", k.Name)
		}
		if k.Book != "" && c.books[k.Book] == nil {
			return fmt.Errorf("kind %q: book %q does not exist", k.Name, k.Book)
		}
		if k.BookDefault != "" && c.books[k.BookDefault] == nil {
			return fmt.Errorf("kind %q: book_default %q does not exist", k.Name, k.BookDefault)
		}
		if k.Tag == "" {
			k.Tag = k.Name
		}
	}
	for i := range c.Books {
		b := &c.Books[i]
		for _, in := range b.Includes {
			if c.books[in] == nil {
				return fmt.Errorf("book %q includes %q, which does not exist", b.Name, in)
			}
		}
		if c.includesCycle(b.Name, b.Name, map[string]bool{}) {
			return fmt.Errorf("book %q includes itself (through its includes)", b.Name)
		}
		for j := range b.Indexes {
			ix := &b.Indexes[j]
			if ix.Slug == "" {
				return fmt.Errorf("book %q: index %d without slug", b.Name, j+1)
			}
			if ix.Title == "" {
				ix.Title = b.Title
			}
			for s := range ix.Sections {
				sec := &ix.Sections[s]
				if sec.Book != "" && c.books[sec.Book] == nil {
					return fmt.Errorf("book %q, index %s, section %q: book %q does not exist", b.Name, ix.Slug, sec.Title, sec.Book)
				}
				if sec.By == "" {
					sec.By = "name"
				}
				if sec.By != "name" && sec.By != "category" {
					return fmt.Errorf("book %q, index %s, section %q: by must be \"name\" or \"category\"", b.Name, ix.Slug, sec.Title)
				}
				if len(sec.Include) == 0 {
					sec.Include = []Filter{{Kind: "*", Category: "*"}}
				}
				for _, f := range append(append([]Filter{}, sec.Include...), sec.Exclude...) {
					if f.Kind != "*" && f.Kind != "" && c.kinds[f.Kind] == nil {
						return fmt.Errorf("book %q, index %s, section %q: kind %q does not exist", b.Name, ix.Slug, sec.Title, f.Kind)
					}
				}
			}
		}
	}
	if c.Index.Slug == "" {
		c.Index.Slug = "index"
	}
	if c.Index.Title == "" {
		c.Index.Title = "Index"
	}
	if len(c.Index.Sections) == 0 {
		var all []string
		for _, b := range c.Books {
			all = append(all, b.Name)
		}
		c.Index.Sections = []GeneralSection{{Books: all}}
	}
	for _, s := range c.Index.Sections {
		for _, b := range s.Books {
			if c.books[b] == nil {
				return fmt.Errorf("index section %q: book %q does not exist", s.Title, b)
			}
		}
	}
	return nil
}

func (c *Config) includesCycle(start, cur string, seen map[string]bool) bool {
	for _, in := range c.books[cur].Includes {
		if in == start {
			return true
		}
		if !seen[in] {
			seen[in] = true
			if c.includesCycle(start, in, seen) {
				return true
			}
		}
	}
	return false
}

// Kind returns the kind of a name or alias, nil when unknown.
func (c *Config) Kind(name string) *Kind { return c.kinds[name] }

// Book returns a book by name, nil when unknown.
func (c *Config) Book(name string) *Book { return c.books[name] }

// KindNames lists the kind names in declaration order.
func (c *Config) KindNames() []string {
	var out []string
	for _, k := range c.Kinds {
		out = append(out, k.Name)
	}
	return out
}

// KindTable builds the scanner's view of the kinds.
func (c *Config) KindTable() *srcdoc.KindTable {
	t := srcdoc.NewKindTable()
	for _, k := range c.Kinds {
		t.Add(k.Name, k.CaseIndex, k.Aliases...)
	}
	return t
}

// BooksOf returns the books a topic of the given kind belongs to, given the
// books it named itself (its "book:" field): the kind's fixed book plus the
// named ones, or the named ones, or the kind's default. Empty when the topic
// must name a book and did not.
func (c *Config) BooksOf(kind string, named []string) []string {
	k := c.kinds[kind]
	if k == nil {
		return nil
	}
	var out []string
	add := func(b string) {
		for _, have := range out {
			if have == b {
				return
			}
		}
		out = append(out, b)
	}
	if k.Book != "" {
		add(k.Book)
	}
	for _, b := range named {
		add(b)
	}
	if len(out) == 0 && k.BookDefault != "" {
		add(k.BookDefault)
	}
	return out
}

// Contains reports whether book b lists the pages of book of (itself, or
// through its includes).
func (c *Config) Contains(b, of string) bool {
	if b == of {
		return true
	}
	bk := c.books[b]
	if bk == nil {
		return false
	}
	for _, in := range bk.Includes {
		if c.Contains(in, of) {
			return true
		}
	}
	return false
}

// MatchCategory reports whether one of the categories matches the comma
// list of masks ("*" alone: always true; a mask may hold '*' wildcards; the
// comparison ignores case).
func MatchCategory(masks string, cats []string) bool {
	masks = strings.TrimSpace(masks)
	if masks == "" || masks == "*" {
		return true
	}
	for _, m := range strings.Split(masks, ",") {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		re := maskRe(m)
		for _, c := range cats {
			if re.MatchString(strings.ToLower(strings.TrimSpace(c))) {
				return true
			}
		}
	}
	return false
}

func maskRe(mask string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, part := range strings.Split(mask, "*") {
		if b.Len() > 1 {
			b.WriteString(".*")
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// Expand replaces the <name> macros of a value with the folders of the file
// (relative to its directory) and returns the result; a path is made
// absolute against the file's directory.
func (c *Config) Expand(v string) (string, error) {
	out := v
	for i := 0; i < 10 && strings.Contains(out, "<"); i++ {
		changed := false
		for name, val := range c.Folders {
			tag := "<" + name + ">"
			if strings.Contains(strings.ToLower(out), strings.ToLower(tag)) {
				re := regexp.MustCompile("(?i)" + regexp.QuoteMeta(tag))
				out = re.ReplaceAllLiteralString(out, val)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if strings.Contains(out, "<") && strings.Contains(out, ">") {
		return "", fmt.Errorf("%s: unknown <macro>", v)
	}
	if c.Dir != "" && !filepath.IsAbs(out) {
		out = filepath.Join(c.Dir, out)
	}
	return out, nil
}

// Sources expands the src list of the file.
func (c *Config) Sources() ([]string, error) {
	var out []string
	for _, s := range c.Src {
		e, err := c.Expand(s)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// DBPath expands the db entry of the file ("" when none).
func (c *Config) DBPath() (string, error) {
	if c.DB == "" {
		return "", nil
	}
	return c.Expand(c.DB)
}
