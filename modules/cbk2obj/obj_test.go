package cbk2obj

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// coffObj is a COFF object parsed for comparisons.
type coffObj struct {
	characteristics uint16
	sections        []coffSection
	symbols         []coffSymbol
}

type coffSection struct {
	name   string
	flags  uint32
	data   []byte
	relocs []coffReloc
}

type coffReloc struct {
	off     uint32
	sym     string
	typ     uint16
	inplace uint32
}

type coffSymbol struct {
	name    string
	value   uint32
	section int16
	class   uint8
	aux     uint8
}

func parseCoff(t *testing.T, b []byte) coffObj {
	t.Helper()
	le := binary.LittleEndian
	if le.Uint16(b[0:]) != imageFileMachineI386 {
		t.Fatalf("machine %04X", le.Uint16(b[0:]))
	}
	nsec := int(le.Uint16(b[2:]))
	symPtr := le.Uint32(b[8:])
	nsym := le.Uint32(b[12:])
	optSize := int(le.Uint16(b[16:]))
	o := coffObj{characteristics: le.Uint16(b[18:])}
	strtab := b[symPtr+nsym*18:]
	name := func(s []byte) string {
		if le.Uint32(s[0:]) == 0 {
			off := le.Uint32(s[4:])
			end := off
			for strtab[end] != 0 {
				end++
			}
			return string(strtab[off:end])
		}
		return strings.TrimRight(string(s[:8]), "\x00")
	}
	for i := uint32(0); i < nsym; i++ {
		s := b[symPtr+i*18:]
		sym := coffSymbol{name: name(s), value: le.Uint32(s[8:]), section: int16(le.Uint16(s[12:])), class: s[16], aux: s[17]}
		o.symbols = append(o.symbols, sym)
		i += uint32(sym.aux)
	}
	symName := func(i uint32) string {
		n := uint32(0)
		for _, s := range o.symbols {
			if n == i {
				return s.name
			}
			n += 1 + uint32(s.aux)
		}
		return fmt.Sprintf("#%d", i)
	}
	for i := range nsec {
		h := b[20+optSize+i*40:]
		size := le.Uint32(h[16:])
		dptr := le.Uint32(h[20:])
		rptr := le.Uint32(h[24:])
		nrel := le.Uint16(h[32:])
		sec := coffSection{name: strings.TrimRight(string(h[:8]), "\x00"), flags: le.Uint32(h[36:]), data: b[dptr : dptr+size]}
		for r := 0; r < int(nrel); r++ {
			e := b[rptr+uint32(r)*10:]
			off := le.Uint32(e[0:])
			sec.relocs = append(sec.relocs, coffReloc{off: off, sym: symName(le.Uint32(e[4:])), typ: le.Uint16(e[8:]), inplace: le.Uint32(sec.data[off:])})
		}
		o.sections = append(o.sections, sec)
	}
	return o
}

func buildSample(t *testing.T) ([]byte, []byte, *Script) {
	t.Helper()
	s := parseSample(t)
	obj, asm, err := Build(s, Options{Timestamp: 1234})
	if err != nil {
		t.Fatal(err)
	}
	return obj, asm, s
}

func TestObject(t *testing.T) {
	obj, _, s := buildSample(t)
	o := parseCoff(t, obj)
	if o.characteristics != fasmCharacteristics {
		t.Errorf("characteristics %04X", o.characteristics)
	}
	if len(o.sections) != 2 || o.sections[0].name != ".text" || o.sections[1].name != ".data" {
		t.Fatalf("sections %+v", o.sections)
	}
	if o.sections[0].flags != flagsText || o.sections[1].flags != flagsData {
		t.Errorf("flags %08X %08X", o.sections[0].flags, o.sections[1].flags)
	}
	var data []byte
	for _, cb := range s.Callbacks {
		data = append(append(data, cb.Name...), 0)
	}
	if !bytes.Equal(o.sections[1].data, data) {
		t.Errorf(".data = %q", o.sections[1].data)
	}
	if len(o.sections[1].relocs) != 0 {
		t.Errorf(".data relocs %d", len(o.sections[1].relocs))
	}
	// symbols: externals, .text, .data, publics
	var externs, publics []string
	for _, sym := range o.symbols {
		switch {
		case sym.aux != 0:
			t.Errorf("symbol %s has aux records", sym.name)
		case sym.class == symClassExternal && sym.section == 0:
			externs = append(externs, sym.name)
		case sym.class == symClassStatic:
			if (sym.name != ".text" || sym.section != 1) && (sym.name != ".data" || sym.section != 2) {
				t.Errorf("static symbol %+v", sym)
			}
		case sym.class == symClassExternal && sym.section == 1:
			publics = append(publics, sym.name)
		default:
			t.Errorf("unexpected symbol %+v", sym)
		}
	}
	// FLOAT appears only as a parameter in the sample: _conGetFloat is not called, hence not declared
	wantExterns := "__retnl __conCall __conCallPa __conNew __conRelease __conPutNL _conGetLong __conPutND __conGetND __conPutL __conGetL _conPutFloat _conPutQWord _conGetQWord"
	if got := strings.Join(externs, " "); got != wantExterns {
		t.Errorf("externs\n got %s\nwant %s", got, wantExterns)
	}
	if got := strings.Join(publics, " "); got != "_CALLBACK_MYWNDPROC _CALLBACK_MYENUM _CALLBACK_MYCMP _CALLBACK_MYVOID _CALLBACK_MYQ _CALLBACK_MYDBL" {
		t.Errorf("publics %s", got)
	}
	// relocations: REL32 to externals only, DIR32 to the two sections only
	ext := map[string]bool{}
	for _, x := range externs {
		ext[x] = true
	}
	calls, pushes := 0, 0
	text := o.sections[0]
	for i, r := range text.relocs {
		if i > 0 && r.off <= text.relocs[i-1].off {
			t.Errorf("relocations not in address order at %d", i)
		}
		switch r.typ {
		case relRel32:
			calls++
			if !ext[r.sym] || r.inplace != 0 || text.data[r.off-1] != 0xE8 {
				t.Errorf("REL32 %+v", r)
			}
		case relDir32:
			pushes++
			if (r.sym != ".text" && r.sym != ".data") || text.data[r.off-1] != 0x68 {
				t.Errorf("DIR32 %+v", r)
			}
			if r.sym == ".data" && int(r.inplace) >= len(data) {
				t.Errorf("DIR32 .data out of range %+v", r)
			}
		default:
			t.Errorf("reloc type %d", r.typ)
		}
	}
	// per callback: 2 address pushes (thunk, name) and at least 3 calls
	if pushes != 2*len(s.Callbacks) || calls < 3*len(s.Callbacks) {
		t.Errorf("pushes %d calls %d", pushes, calls)
	}
}

// TestEntryEncoding checks the bytes of one _CALLBACK_ entry against the
// hand-assembled template.
func TestEntryEncoding(t *testing.T) {
	s, diags, _ := Parse(strings.NewReader("CALLBACK WNDPROC X\r\n"))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	u := generate(s, genOptions{})
	text, relocs, _, publics, err := encode(u)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x55,       // push ebp
		0x89, 0xE5, // mov ebp, esp
		0x68, 0x16, 0x00, 0x00, 0x00, // push _static_cb_X (offset 22 = entry size)
		0x8B, 0x45, 0x08, // mov eax, [ebp+8]
		0x50,             // push eax
		0xE8, 0, 0, 0, 0, // call __retnl
		0x83, 0xC4, 0x08, // add esp, 8
		0x5D, // pop ebp
		0xC3, // ret 0
	}
	if !bytes.Equal(text[:len(want)], want) {
		t.Errorf("entry bytes\n got % X\nwant % X", text[:len(want)], want)
	}
	if len(publics) != 1 || publics[0].off != 0 || publics[0].name != "_CALLBACK_X" {
		t.Errorf("publics %+v", publics)
	}
	if len(relocs) < 2 || relocs[0] != (reloc{off: 4, sym: ".text", typ: relDir32}) || relocs[1] != (reloc{off: 13, sym: "__retnl", typ: relRel32}) {
		t.Errorf("relocs %+v", relocs)
	}
	// thunk prologue at offset 22: push ebp / mov ebp,esp / sub esp,24
	if !bytes.Equal(text[22:28], []byte{0x55, 0x89, 0xE5, 0x83, 0xEC, 0x18}) {
		t.Errorf("thunk prologue % X", text[22:28])
	}
	// the thunk ends with mov esp,ebp / pop ebp / ret 16
	tail := text[len(text)-6:]
	if !bytes.Equal(tail, []byte{0x89, 0xEC, 0x5D, 0xC2, 0x10, 0x00}) {
		t.Errorf("thunk epilogue % X", tail)
	}
}

func TestWideOperands(t *testing.T) {
	// 40 QWORD parameters: frame and argument offsets beyond 127 need disp32
	// forms, the frame needs sub esp with imm32, ret N beyond 255.
	var sb strings.Builder
	sb.WriteString("BEGIN CALLBACK Big RETURNS QWORD\r\n")
	for range 40 {
		sb.WriteString("PARAM QWORD\r\n")
	}
	sb.WriteString("END CALLBACK\r\n")
	s, diags, _ := Parse(strings.NewReader(sb.String()))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	u := generate(s, genOptions{})
	text, _, _, _, err := encode(u)
	if err != nil {
		t.Fatal(err)
	}
	f := layout(&s.Callbacks[0])
	if f.size != 8+4+40*4 || f.stack != 320 {
		t.Fatalf("frame %+v", f)
	}
	// sub esp,172 -> 81 EC AC 00 00 00
	if i := bytes.Index(text, []byte{0x81, 0xEC, 0xAC, 0x00, 0x00, 0x00}); i < 0 {
		t.Error("sub esp, imm32 not found")
	}
	// mov eax,[ebp+324] (last argument high dword: 8+39*8+4) -> 8B 85 44 01 00 00
	if i := bytes.Index(text, []byte{0x8B, 0x85, 0x44, 0x01, 0x00, 0x00}); i < 0 {
		t.Error("mov eax,[ebp+disp32] not found")
	}
	// mov [ebp-172] , eax -> 89 85 54 FF FF FF
	if i := bytes.Index(text, []byte{0x89, 0x85, 0x54, 0xFF, 0xFF, 0xFF}); i < 0 {
		t.Error("mov [ebp-disp32], eax not found")
	}
	// ret 320 -> C2 40 01
	if !bytes.Equal(text[len(text)-3:], []byte{0xC2, 0x40, 0x01}) {
		t.Errorf("ret % X", text[len(text)-3:])
	}
}

// TestFasmDifferential assembles the generated .asm with FASM (path in the
// FASM environment variable; skipped when unset) and compares its object
// with ours: section bytes, relocations and symbols.
func TestFasmDifferential(t *testing.T) {
	fasm := os.Getenv("FASM")
	if fasm == "" {
		t.Skip("FASM not set")
	}
	obj, asm, _ := buildSample(t)
	dir := t.TempDir()
	asmPath := filepath.Join(dir, "sample.asm")
	objPath := filepath.Join(dir, "sample.obj")
	if err := os.WriteFile(asmPath, asm, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(fasm, asmPath, objPath).CombinedOutput()
	if err != nil {
		t.Fatalf("fasm: %v\n%s", err, out)
	}
	ref, err := os.ReadFile(objPath)
	if err != nil {
		t.Fatal(err)
	}
	want := parseCoff(t, ref)
	got := parseCoff(t, obj)
	if got.characteristics != want.characteristics {
		t.Errorf("characteristics %04X, fasm %04X", got.characteristics, want.characteristics)
	}
	if len(got.sections) != len(want.sections) {
		t.Fatalf("%d sections, fasm %d", len(got.sections), len(want.sections))
	}
	for i := range want.sections {
		g, w := got.sections[i], want.sections[i]
		if g.name != w.name || g.flags != w.flags {
			t.Errorf("section %d: %s %08X, fasm %s %08X", i, g.name, g.flags, w.name, w.flags)
		}
		if !bytes.Equal(g.data, w.data) {
			n := 0
			for n < len(g.data) && n < len(w.data) && g.data[n] == w.data[n] {
				n++
			}
			t.Errorf("section %s: bytes differ at %04X (%d vs %d bytes)", g.name, n, len(g.data), len(w.data))
		}
		if len(g.relocs) != len(w.relocs) {
			t.Errorf("section %s: %d relocs, fasm %d", g.name, len(g.relocs), len(w.relocs))
			continue
		}
		for j := range w.relocs {
			if g.relocs[j] != w.relocs[j] {
				t.Errorf("section %s reloc %d: %+v, fasm %+v", g.name, j, g.relocs[j], w.relocs[j])
			}
		}
	}
	if len(got.symbols) != len(want.symbols) {
		t.Fatalf("%d symbols, fasm %d:\n%+v\n%+v", len(got.symbols), len(want.symbols), got.symbols, want.symbols)
	}
	for i := range want.symbols {
		if got.symbols[i] != want.symbols[i] {
			t.Errorf("symbol %d: %+v, fasm %+v", i, got.symbols[i], want.symbols[i])
		}
	}
	if dump := os.Getenv("CBK2OBJ_DUMP"); dump != "" { // keep both objects for a look with a COFF dumper
		os.WriteFile(filepath.Join(dump, "ours.obj"), obj, 0o644)
		os.WriteFile(filepath.Join(dump, "fasm.obj"), ref, 0o644)
	}
	if !bytes.Equal(obj[20:], ref[20:]) { // everything but the header (timestamp)
		n := 20
		for n < len(obj) && n < len(ref) && obj[n] == ref[n] {
			n++
		}
		t.Errorf("objects differ beyond the file header at offset %d (%d vs %d bytes):\n ours % X\n fasm % X", n, len(obj), len(ref), obj[n:min(n+16, len(obj))], ref[n:min(n+16, len(ref))])
	}
}
