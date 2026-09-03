package docgen

import (
	"strings"
	"testing"
)

func TestSlugOf(t *testing.T) {
	for _, c := range []struct{ kind, key, want string }{
		{"function", "ARRAY2PPMARSHALL", "function-array2ppmarshall"},
		{"class", "_LARGE_INTEGER_", "class-_large_integer_"},
		{"method", "LARGE_INTEGER:NEW64", "method-large_integer.new64"},
		{"cpp-function", "json_ns::serialize(XppParamList)", "cpp-function-json_ns.serialize-xppparamlist"},
		{"command", "BEGIN DYNAMIC CLASS", "command-begin-dynamic-class"},
		{"note", "con-get-long-ex", "note-con-get-long-ex"},
		{"c-function", "_conGetLong", "c-function-_congetlong"},
	} {
		if got := slugOf(c.kind, c.key); got != c.want {
			t.Fatalf("slugOf(%s, %q) = %q, want %q", c.kind, c.key, got, c.want)
		}
	}
	// a huge key is cut and hashed, and stays a file name
	long := slugOf("function", strings.Repeat("A", 300))
	if len(long) > len("function-")+maxStem || !ValidSlug(strings.TrimPrefix(long, "function-")) {
		t.Fatalf("long slug = %q", long)
	}
}

func TestSlugsCollisionsAndExplicit(t *testing.T) {
	topics := []Topic{
		{1, "c-function", "Foo", "Foo"},    // collides with 2 once lower-cased
		{2, "c-function", "FOO", "FOO"},    // (Windows would merge Foo.md / FOO.md)
		{3, "function", "BAR", "Bar"},      // alone
		{4, "function", "BAZ", "Baz"},      // explicit slug "nice"
		{5, "class", "NICE", "Nice"},       // computed slug class-nice, no clash with "nice"
		{6, "function", "QUX", "Qux"},      // explicit "shared"
		{7, "function", "QUUX", "Quux"},    // explicit "shared" too -> both suffixed + warning
		{8, "function", "BAD", "Bad"},      // explicit with a space -> invalid, computed instead
		{9, "structure", "TAKEN", "Taken"}, // computed "structure-taken"...
		{10, "function", "X", "X"},         // ...explicit "structure-taken" wins, 9 is suffixed
	}
	explicit := map[int64]string{4: "Nice", 6: "shared", 7: "shared", 8: "not a slug", 10: "structure-taken"}
	slugs, warns := Slugs(topics, explicit)

	if slugs[1] == slugs[2] || !strings.HasPrefix(slugs[1], "c-function-foo-") || !strings.HasPrefix(slugs[2], "c-function-foo-") {
		t.Fatalf("case collision not suffixed: %q %q", slugs[1], slugs[2])
	}
	if slugs[3] != "function-bar" {
		t.Fatalf("plain slug = %q", slugs[3])
	}
	if slugs[4] != "nice" {
		t.Fatalf("explicit slug lower-cased = %q", slugs[4])
	}
	if slugs[5] != "class-nice" {
		t.Fatalf("no false clash: %q", slugs[5])
	}
	if slugs[6] == slugs[7] || !strings.HasPrefix(slugs[6], "shared-") || !strings.HasPrefix(slugs[7], "shared-") {
		t.Fatalf("duplicate explicit slugs must both be suffixed: %q %q", slugs[6], slugs[7])
	}
	if slugs[8] != "function-bad" {
		t.Fatalf("invalid explicit must fall back: %q", slugs[8])
	}
	if slugs[10] != "structure-taken" || slugs[9] == "structure-taken" || !strings.HasPrefix(slugs[9], "structure-taken-") {
		t.Fatalf("explicit must win over computed: %q / %q", slugs[10], slugs[9])
	}
	// every stem unique (case-insensitively) and a valid file name
	seen := map[string]bool{}
	for id, s := range slugs {
		if !ValidSlug(s) || seen[strings.ToLower(s)] {
			t.Fatalf("slug %q for %d invalid or duplicated", s, id)
		}
		seen[strings.ToLower(s)] = true
	}
	if len(warns) != 3 { // invalid explicit (Bad) + one per topic sharing "shared" (Qux, Quux)
		t.Fatalf("warnings = %v", warns)
	}
}
