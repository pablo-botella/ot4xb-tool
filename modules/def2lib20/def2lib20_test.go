package def2lib20

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const sampleDef = "LIBRARY ot4xb\r\nEXPORTS\r\n;; comment\r\n     OT4XB =  _OT4XB\r\n     APPINSTANCE =  _APPINSTANCE\r\n     GETCURRENTPROCESSHANDLE =  _GETCURRENTPROCESSHANDLE\r\n     PLAIN\r\n     WITHORD = _WITHORD @12\r\n     HIDDEN  PRIVATE\r\n     APPINSTANCE =  _APPINSTANCE\r\n"

func parseSample(t *testing.T) *Def {
	t.Helper()
	d, err := ParseDef(strings.NewReader(sampleDef))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// arMember is what the test reader extracts from an archive.
type arMember struct {
	off  int
	name string
	data []byte
}

func readArchive(t *testing.T, b []byte) []arMember {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("!<arch>\n")) {
		t.Fatalf("bad signature")
	}
	var ms []arMember
	pos := 8
	for pos < len(b) {
		if pos%2 != 0 {
			t.Fatalf("member header at odd offset %d", pos)
		}
		h := b[pos : pos+60]
		if string(h[58:60]) != "`\n" {
			t.Fatalf("bad header end at %d", pos)
		}
		size, err := strconv.Atoi(strings.TrimSpace(string(h[48:58])))
		if err != nil {
			t.Fatalf("bad size at %d: %v", pos, err)
		}
		ms = append(ms, arMember{off: pos, name: strings.TrimRight(string(h[:16]), " "), data: b[pos+60 : pos+60+size]})
		pos += 60 + size
		if pos%2 == 1 {
			if b[pos] != '\n' {
				t.Fatalf("bad padding at %d", pos)
			}
			pos++
		}
	}
	return ms
}

// linkerMember1 decodes the first linker member: names in member order and
// the member offset of each.
func linkerMember1(t *testing.T, data []byte) (names []string, offs []int) {
	t.Helper()
	n := int(binary.BigEndian.Uint32(data))
	offs = make([]int, n)
	for i := range offs {
		offs[i] = int(binary.BigEndian.Uint32(data[4+4*i:]))
	}
	names = strings.Split(strings.TrimRight(string(data[4+4*n:]), "\x00"), "\x00")
	if len(names) != n {
		t.Fatalf("lm1: %d names for %d symbols", len(names), n)
	}
	return names, offs
}

func TestBuildStructure(t *testing.T) {
	var warnings []string
	lib, err := Build(parseSample(t), Options{Timestamp: 801347941, Warn: func(s string) { warnings = append(warnings, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "APPINSTANCE") || !strings.Contains(warnings[0], "line 10") {
		t.Errorf("warnings = %q", warnings)
	}
	ms := readArchive(t, lib)
	// 2 linker members + 3 special + 5 imports (HIDDEN private and the duplicate dropped)
	if len(ms) != 2+3+5 {
		t.Fatalf("got %d members", len(ms))
	}
	if ms[0].name != "/" || ms[1].name != "/" {
		t.Fatalf("linker member names %q %q", ms[0].name, ms[1].name)
	}
	for _, m := range ms[2:] {
		if m.name != "ot4xb.dll/" {
			t.Errorf("member name %q", m.name)
		}
		if binary.LittleEndian.Uint16(m.data) != imageFileMachineI386 {
			t.Errorf("member %q is not an i386 object", m.name)
		}
	}
	names1, offs1 := linkerMember1(t, ms[0].data)
	if !sort.IntsAreSorted(offs1) {
		t.Errorf("lm1 offsets not ascending")
	}
	wantSyms := []string{"__IMPORT_DESCRIPTOR_ot4xb", "__NULL_IMPORT_DESCRIPTOR", "\x7fot4xb_NULL_THUNK_DATA",
		"OT4XB", "__imp_OT4XB", "APPINSTANCE", "__imp_APPINSTANCE", "GETCURRENTPROCESSHANDLE", "__imp_GETCURRENTPROCESSHANDLE",
		"PLAIN", "__imp_PLAIN", "WITHORD", "__imp_WITHORD"}
	if strings.Join(names1, ",") != strings.Join(wantSyms, ",") {
		t.Errorf("lm1 names = %q", names1)
	}
	memberAt := map[int]int{}
	for i, m := range ms {
		memberAt[m.off] = i
	}
	for i, o := range offs1 {
		mi, ok := memberAt[o]
		if !ok {
			t.Errorf("lm1 symbol %s offset %d is not a member", names1[i], o)
			continue
		}
		if !bytes.Contains(ms[mi].data, []byte(names1[i])) {
			t.Errorf("member for %s does not contain the name", names1[i])
		}
	}
	// second linker member
	lm2 := ms[1].data
	nm := int(binary.LittleEndian.Uint32(lm2))
	if nm != len(ms)-2 {
		t.Fatalf("lm2 members = %d, want %d", nm, len(ms)-2)
	}
	p := 4
	for i := 0; i < nm; i++ {
		if int(binary.LittleEndian.Uint32(lm2[p:])) != ms[2+i].off {
			t.Errorf("lm2 offset %d mismatch", i)
		}
		p += 4
	}
	n2 := int(binary.LittleEndian.Uint32(lm2[p:]))
	p += 4
	if n2 != len(names1) {
		t.Fatalf("lm2 symbols = %d, lm1 = %d", n2, len(names1))
	}
	idx := make([]int, n2)
	for i := range idx {
		idx[i] = int(binary.LittleEndian.Uint16(lm2[p:]))
		p += 2
	}
	names2 := strings.Split(strings.TrimRight(string(lm2[p:]), "\x00"), "\x00")
	if !sort.StringsAreSorted(names2) {
		t.Errorf("lm2 names not sorted: %q", names2)
	}
	for i, name := range names2 {
		if idx[i] < 1 || idx[i] > nm {
			t.Errorf("lm2 index out of range for %s", name)
		} else if !bytes.Contains(ms[1+idx[i]].data, []byte(name)) {
			t.Errorf("lm2 maps %s to a member that does not define it", name)
		}
	}
	// the hint/name entry of WITHORD carries its ordinal as hint (MS mode)
	if !bytes.Contains(lib, []byte("\x0c\x00WITHORD\x00")) {
		t.Errorf("WITHORD hint/name entry with hint 12 not found")
	}
}

func TestAimplibMonkey(t *testing.T) {
	d := &Def{Library: "ot4xb", Exports: []Export{{Name: "APPINSTANCE", Ordinal: 7}}}
	lib, err := Build(d, Options{Monkey: true})
	if err != nil {
		t.Fatal(err)
	}
	ms := readArchive(t, lib)
	names, _ := linkerMember1(t, ms[0].data)
	want := []string{"ot4xb_IMPORT_DESCRIPTOR", "NULL_IMPORT_DESCRIPTOR", "ot4xb_NULL_THUNK_DATA", "APPINSTANCE", "__imp_APPINSTANCE"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("monkey names = %q", names)
	}
	if !bytes.Contains(ms[5].data, []byte("\xFF\xFFAPPINSTANCE\x00")) {
		t.Errorf("monkey hint/name entry (hint 0xFFFF) not found")
	}
	lib2, err := Build(d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ms2 := readArchive(t, lib2)
	if !bytes.Contains(ms2[5].data, []byte("\x07\x00APPINSTANCE\x00")) {
		t.Errorf("MS hint/name entry (hint = ordinal 7) not found")
	}
	if !bytes.Contains(ms2[2].data, []byte("__IMPORT_DESCRIPTOR_ot4xb")) || !bytes.Contains(ms2[2].data, []byte("\x7fot4xb_NULL_THUNK_DATA")) {
		t.Errorf("MS descriptor names not found")
	}
}

func TestPrefixAndDLL(t *testing.T) {
	d := &Def{Library: "ot4xb", Exports: []Export{{Name: "conGetLong"}}}
	lib, err := Build(d, Options{Prefix: "_", DLL: "other.dll"})
	if err != nil {
		t.Fatal(err)
	}
	ms := readArchive(t, lib)
	names, _ := linkerMember1(t, ms[0].data)
	if names[0] != "__IMPORT_DESCRIPTOR_other" || names[3] != "_conGetLong" || names[4] != "__imp__conGetLong" {
		t.Errorf("names = %q", names)
	}
	if ms[2].name != "other.dll/" || !bytes.Contains(ms[2].data, []byte("other.dll\x00")) {
		t.Errorf("descriptor member %q", ms[2].name)
	}
	// the DLL export name keeps no prefix
	if !bytes.Contains(ms[5].data, []byte("\x00\x00conGetLong\x00")) {
		t.Errorf("hint/name must use the undecorated export name")
	}
}

func TestLongDLLName(t *testing.T) {
	d := &Def{Library: "averyveryverylongdllname", Exports: []Export{{Name: "F"}}}
	lib, err := Build(d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ms := readArchive(t, lib)
	if ms[2].name != "//" {
		t.Fatalf("expected longnames member, got %q", ms[2].name)
	}
	if !strings.HasPrefix(ms[3].name, "/0") {
		t.Errorf("member name %q should reference the longnames member", ms[3].name)
	}
	if !bytes.HasPrefix(ms[2].data, []byte("averyveryverylongdllname.dll\x00")) {
		t.Errorf("longnames content %q", ms[2].data)
	}
}

func TestUnsupported(t *testing.T) {
	for _, e := range []Export{{Name: "X", NoName: true, Ordinal: 3}, {Name: "Y", Data: true}} {
		_, err := Build(&Def{Library: "a", Exports: []Export{e}}, Options{})
		if err == nil {
			t.Errorf("export %+v should be rejected", e)
		}
	}
	if _, err := Build(&Def{}, Options{}); err == nil {
		t.Errorf("missing LIBRARY and DLL should be rejected")
	}
}

// TestAlink links a small Xbase++ object against a library generated from
// testdata/ot4xb-sample.def using Alaska's ALINK, when it is installed on the
// machine (Windows only, skipped otherwise).
func TestAlink(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	alink := `C:\Alaska\XPPW32\bin\Alink.exe`
	libDir := `C:\Alaska\XPPW32\lib`
	if _, err := os.Stat(alink); err != nil {
		t.Skip("alink not installed")
	}
	obj, err := os.ReadFile(filepath.Join("testdata", "t1.obj"))
	if err != nil {
		t.Skip("testdata/t1.obj missing")
	}
	for _, monkey := range []bool{false, true} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "t1.obj"), obj, 0o644); err != nil {
			t.Fatal(err)
		}
		libPath := filepath.Join(dir, "ot4xb.lib")
		if _, err := BuildFile(filepath.Join("testdata", "ot4xb-sample.def"), libPath, Options{Monkey: monkey}); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(alink, "/NOLOGO", "/PM:VIO", "/OUT:t1.exe", "t1.obj", "ot4xb.lib")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "LIB="+libDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("monkey=%v: alink failed: %v\n%s", monkey, err, out)
		}
		if _, err := os.Stat(filepath.Join(dir, "t1.exe")); err != nil {
			t.Fatalf("monkey=%v: t1.exe not produced\n%s", monkey, out)
		}
	}
}
