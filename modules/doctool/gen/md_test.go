package gen

import "testing"

func TestMdRegions(t *testing.T) {
	m := &model{bySlug: map[string]*page{"x": {file: "x.md"}}, byGroup: map[string]*page{}, byKey: map[string]*topic{}, pageOf: map[int64]*page{}}
	// default: common indent stripped, ilinks resolved, marks gone
	f := field{label: "desc", value: "Flags {{begin-md}}\n      | flag | meaning |\n      |---|---|\n      | 0x01 | {{ilink: <slug x> X}} |\n      {{end-md}}"}
	got := m.renderField(f)
	want := "**desc:** Flags \n| flag | meaning |\n|---|---|\n| 0x01 | [X](x.md) |\n"
	if got != want {
		t.Fatalf("default:\n%q\nwant\n%q", got, want)
	}
	// raw: bytes kept, nothing replaced
	f = field{label: "", value: "{{begin-md: raw}}\n    if x\n       {{ilink: <slug x> X}}\n{{end-md}}"}
	got = m.renderField(f)
	want = "\n    if x\n       {{ilink: <slug x> X}}\n"
	if got != want {
		t.Fatalf("raw:\n%q\nwant\n%q", got, want)
	}
}
