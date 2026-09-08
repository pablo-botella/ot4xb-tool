package docgen

import (
	"strings"
	"testing"
)

func TestFirstSentence(t *testing.T) {
	cases := map[string]string{
		"":                                    "",
		"A point.":                            "A point.",
		"Copies n bytes. Returns the count.":  "Copies n bytes.",
		"Uses e.g. this one. And more.":       "Uses e.g. this one.",
		"Multi\r\n   line   text without dot": "Multi line text without dot",
	}
	for in, want := range cases {
		if got := firstSentence(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	long := firstSentence(strings.Repeat("wordy ", 40))
	if len(long) > 164 || !strings.HasSuffix(long, "...") {
		t.Errorf("long: %q", long)
	}
}
