package gen

import (
	"fmt"
	"strings"
	"testing"
)

// dump renders a parse as one line per block, for the expectations.
func dump(blocks []Block) string {
	var b strings.Builder
	for _, bl := range blocks {
		switch v := bl.(type) {
		case *Heading:
			fmt.Fprintf(&b, "h%d:%s\n", v.Level, dumpInlines(v.Inlines))
		case *Paragraph:
			fmt.Fprintf(&b, "p:%s\n", dumpInlines(v.Inlines))
		case *List:
			for i, it := range v.Items {
				fmt.Fprintf(&b, "li:%s\n", dumpInlines(it))
				if i < len(v.Subs) && v.Subs[i] != nil {
					for _, s := range v.Subs[i].Items {
						fmt.Fprintf(&b, "  li:%s\n", dumpInlines(s))
					}
				}
			}
		case *CodeBlock:
			fmt.Fprintf(&b, "code(%s):%q\n", v.Lang, v.Text)
		case *RawMarkdown:
			fmt.Fprintf(&b, "md:%q\n", v.Text)
		}
	}
	return b.String()
}

func dumpInlines(in []Inline) string {
	var b strings.Builder
	for _, i := range in {
		switch v := i.(type) {
		case *Text:
			b.WriteString(v.Text)
		case *Strong:
			fmt.Fprintf(&b, "<b>%s</b>", v.Text)
		case *Code:
			fmt.Fprintf(&b, "<c>%s</c>", v.Text)
		case *Link:
			fmt.Fprintf(&b, "<a %s>%s</a>", v.Target, v.Text)
		case *URL:
			fmt.Fprintf(&b, "<url %s>%s</url>", v.URL, v.Text)
		case *Call:
			fmt.Fprintf(&b, "<call %s|%s>", v.Name, v.Arg)
		}
	}
	return b.String()
}

func TestParseText(t *testing.T) {
	cases := []struct{ in, want string }{
		// the subset
		{"# Title\n\ntext", "h1:Title\np:text\n"},
		{"##### deep", "h5:deep\n"},
		{"one\r\ntwo\r\n\r\nthree", "p:one two\np:three\n"},
		{"- a\n- b\n  continued\n\nafter", "li:a\nli:b continued\np:after\n"},
		{"para\n- item", "p:para\nli:item\n"},
		{"```xbase\n  code **not** `raw`\n```\nx", "code(xbase):\"  code **not** `raw`\"\np:x\n"},
		{"**bold** and `code` and ~tilde~ end", "p:<b>bold</b> and <c>code</c> and ~tilde~ end\n"},
		{"each '~' is replaced, ~7 digits, ~nBits", "p:each '~' is replaced, ~7 digits, ~nBits\n"},
		{"see {{ilink: <function foo> foo}} now", "p:see <call ilink|<function foo> foo> now\n"},
		{"{{include-note-id: x}}", "p:<call include-note-id|x>\n"},
		// a bracket is the author's: a URL, never a page reference
		{"- [Foo](function-foo.md) - does foo\n- [Bar](b.md#x)", "li:<url function-foo.md>Foo</url> - does foo\nli:<url b.md#x>Bar</url>\n"},
		{"[MS](https://x.test/p.html) and [mail](mailto:pb@x.test)", "p:<url https://x.test/p.html>MS</url> and <url mailto:pb@x.test>mail</url>\n"},
		{"an array [1,2] and a [bracket] alone", "p:an array [1,2] and a [bracket] alone\n"},
		// zone 2
		{"before\n{{begin-md}}\n| a | b |\n|---|---|\n{{end-md}}\nafter", "p:before\nmd:\"| a | b |\\n|---|---|\"\np:after\n"},
		{"{{begin-md: raw}}\n  kept\n{{end-md}}", "md:\"  kept\"\n"},
		// nothing else is special: identifiers, placeholders, paths, pipes
		{"_OT4XB_MAP_WAPIST_FUNC_ and *x* and <cMethod> and \\\\server\\share and a | b", "p:_OT4XB_MAP_WAPIST_FUNC_ and *x* and <cMethod> and \\\\server\\share and a | b\n"},
		{"a `char**` pointer and LPSTR*", "p:a <c>char**</c> pointer and LPSTR*\n"},
		{"a `|` in code and `<x>` too", "p:a <c>|</c> in code and <c><x></c> too\n"},
		{"", ""},
	}
	for _, c := range cases {
		got, issues := ParseText(c.in)
		if d := dump(got); d != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.in, d, c.want)
		}
		if len(issues) != 0 {
			t.Errorf("%q: issues %v", c.in, issues)
		}
	}
}

func TestParseTextNest(t *testing.T) {
	in := "- [Book](b.md)\n  - [Index](i.md)\n  - [Sec](s.md)\n    wrapped\n- [Other](o.md)\n"
	got, issues := parseText(in, true)
	want := "li:<url b.md>Book</url>\n  li:<url i.md>Index</url>\n  li:<url s.md>Sec</url> wrapped\nli:<url o.md>Other</url>\n"
	if d := dump(got); d != want || len(issues) != 0 {
		t.Errorf("nest:\n got %q\nwant %q\nissues %v", d, want, issues)
	}
}

func TestParseTextIssues(t *testing.T) {
	cases := []struct {
		in   string
		line int
		msg  string
	}{
		{"###### six", 1, "heading"},
		{"#nospace", 1, "heading"},
		{"ok\n\n**open", 3, "never closed"},
		{"a `tick", 1, "never closed"},
		{"a {{call", 1, "never closed"},
		{"```\nnever closed", 1, "never closed"},
		{"{{begin-md}}\nno end", 1, "never closed"},
		{"- a\n  - nested", 2, "nested"},
		{"x\n{{begin-md}}\ntable\n{{end-md}} trailing", 4, "after {{end-md}}"},
	}
	for _, c := range cases {
		_, issues := ParseText(c.in)
		if len(issues) == 0 {
			t.Errorf("%q: no issue, want %q", c.in, c.msg)
			continue
		}
		if issues[0].Line != c.line || !strings.Contains(issues[0].Msg, c.msg) {
			t.Errorf("%q: got %v, want line %d %q", c.in, issues[0], c.line, c.msg)
		}
	}
}
