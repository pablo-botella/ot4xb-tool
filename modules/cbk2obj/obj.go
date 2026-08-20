// obj.go encodes the code model into x86-32 machine code and writes the
// COFF object, laid out as FASM writes "format MS COFF" objects: .text and
// .data with 4-byte alignment flags, the data of both sections first, then
// the relocations, then the symbol table with the externals first
// (declaration order), the .text section symbol and its public labels, and
// .data; the string table closes the file. Instruction encodings
// are the ones FASM picks (shortest displacement / immediate, 89 /r for
// register moves, ret 0 as C3), so that the bytes of an assembled .asm and
// the bytes written here are identical.

package cbk2obj

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

const (
	imageFileMachineI386 = 0x014C
	// fasmCharacteristics is the file header Characteristics FASM writes:
	// 32-bit machine, line numbers stripped, bytes reversed lo.
	fasmCharacteristics = 0x0184
	flagsText           = 0x60300020 // code | align 4 | execute | read
	flagsData           = 0xC0300040 // initialized data | align 4 | read | write
	relDir32            = 6          // IMAGE_REL_I386_DIR32: 32-bit absolute address
	relRel32            = 20         // IMAGE_REL_I386_REL32: 32-bit relative displacement
	symClassExternal    = 2
	symClassStatic      = 3
	fileHeaderSize      = 20
	sectionHeaderSize   = 40
	relocationSize      = 10
)

// reloc is one relocation of .text: sym is an external name or the
// section (".text" / ".data") whose address the in-place value is
// relative to.
type reloc struct {
	off uint32
	sym string
	typ uint16
}

// public is a public label of .text.
type public struct {
	name string
	off  uint32
}

// fixup is a "push <label>" whose in-place value is patched once every
// label has an offset.
type fixup struct {
	at    uint32
	label string
}

type encoder struct {
	text   []byte
	relocs []reloc
	fixups []fixup
}

func (e *encoder) b(v ...byte) { e.text = append(e.text, v...) }
func (e *encoder) d32(v int32) { e.text = binary.LittleEndian.AppendUint32(e.text, uint32(v)) }
func (e *encoder) d16(v int32) { e.text = binary.LittleEndian.AppendUint16(e.text, uint16(v)) }

func fits8(v int32) bool { return v >= -128 && v <= 127 }

// modrmEbp emits opcode + ModRM for an [ebp+disp] operand with reg in the
// reg field: disp8 when it fits, disp32 otherwise.
func (e *encoder) modrmEbp(opc, reg byte, disp int32) {
	e.b(opc)
	if fits8(disp) {
		e.b(0x40|reg<<3|5, byte(int8(disp)))
	} else {
		e.b(0x80 | reg<<3 | 5)
		e.d32(disp)
	}
}

// aluEsp emits "op esp, imm" (83 /n ib or 81 /n id) with the ModRM byte
// given (EC = sub esp, C4 = add esp).
func (e *encoder) aluEsp(modrm byte, imm int32) {
	if fits8(imm) {
		e.b(0x83, modrm, byte(int8(imm)))
	} else {
		e.b(0x81, modrm)
		e.d32(imm)
	}
}

func (e *encoder) emit(in ins) error {
	switch in.op {
	case opPushEbp:
		e.b(0x55)
	case opMovEbpEsp:
		e.b(0x89, 0xE5)
	case opMovEspEbp:
		e.b(0x89, 0xEC)
	case opPopEbp:
		e.b(0x5D)
	case opPushEax:
		e.b(0x50)
	case opSubEsp:
		e.aluEsp(0xEC, in.n)
	case opAddEsp:
		e.aluEsp(0xC4, in.n)
	case opPushImm:
		if fits8(in.n) {
			e.b(0x6A, byte(int8(in.n)))
		} else {
			e.b(0x68)
			e.d32(in.n)
		}
	case opPushSym:
		e.b(0x68)
		e.fixups = append(e.fixups, fixup{at: uint32(len(e.text)), label: in.sym})
		e.d32(0)
	case opMovEaxMem:
		e.modrmEbp(0x8B, 0, in.n)
	case opMovEdxMem:
		e.modrmEbp(0x8B, 2, in.n)
	case opMovMemEax:
		e.modrmEbp(0x89, 0, in.n)
	case opLeaEaxMem:
		e.modrmEbp(0x8D, 0, in.n)
	case opMovEaxImm:
		e.b(0xB8)
		e.d32(in.n)
	case opAndEaxImm:
		if fits8(in.n) {
			e.b(0x83, 0xE0, byte(int8(in.n)))
		} else {
			e.b(0x25)
			e.d32(in.n)
		}
	case opCall:
		e.b(0xE8)
		e.relocs = append(e.relocs, reloc{off: uint32(len(e.text)), sym: in.sym, typ: relRel32})
		e.d32(0)
	case opFldQword:
		e.modrmEbp(0xDD, 0, in.n)
	case opFldDword:
		e.modrmEbp(0xD9, 0, in.n)
	case opRet:
		if in.n == 0 {
			e.b(0xC3)
		} else {
			e.b(0xC2)
			e.d16(in.n)
		}
	default:
		return fmt.Errorf("cbk2obj: unknown opcode %d", in.op)
	}
	return nil
}

// encode produces the .text bytes with their relocations (address order),
// the .data bytes and the public labels.
func encode(u *unit) (text []byte, relocs []reloc, data []byte, publics []public, err error) {
	e := &encoder{}
	labels := map[string]uint32{}
	for _, b := range u.blocks {
		off := uint32(len(e.text))
		labels[b.label] = off
		if b.public {
			publics = append(publics, public{name: b.label, off: off})
		}
		for _, in := range b.code {
			if err := e.emit(in); err != nil {
				return nil, nil, nil, nil, err
			}
		}
	}
	dataLabels := map[string]uint32{}
	for _, d := range u.data {
		dataLabels[d.label] = uint32(len(data))
		data = append(data, d.bytes...)
	}
	for _, fx := range e.fixups {
		if off, ok := labels[fx.label]; ok {
			binary.LittleEndian.PutUint32(e.text[fx.at:], off)
			e.relocs = append(e.relocs, reloc{off: fx.at, sym: ".text", typ: relDir32})
		} else if off, ok := dataLabels[fx.label]; ok {
			binary.LittleEndian.PutUint32(e.text[fx.at:], off)
			e.relocs = append(e.relocs, reloc{off: fx.at, sym: ".data", typ: relDir32})
		} else {
			return nil, nil, nil, nil, fmt.Errorf("cbk2obj: undefined label %s", fx.label)
		}
	}
	sort.Slice(e.relocs, func(i, j int) bool { return e.relocs[i].off < e.relocs[j].off })
	return e.text, e.relocs, data, publics, nil
}

// object writes the COFF object of u with the given header timestamp.
func object(u *unit, ts uint32) ([]byte, error) {
	text, relocs, data, publics, err := encode(u)
	if err != nil {
		return nil, err
	}
	type symbol struct {
		name    string
		value   uint32
		section int16
		class   uint8
	}
	var syms []symbol
	index := map[string]uint32{}
	add := func(s symbol) {
		index[s.name] = uint32(len(syms))
		syms = append(syms, s)
	}
	for _, x := range u.externs {
		add(symbol{name: x, class: symClassExternal})
	}
	for _, r := range relocs { // a called symbol missing from the declarations (cannot happen outside legacy mode)
		if _, ok := index[r.sym]; !ok && r.typ == relRel32 {
			add(symbol{name: r.sym, class: symClassExternal})
		}
	}
	add(symbol{name: ".text", section: 1, class: symClassStatic})
	for _, p := range publics { // FASM lists a section's publics right after its section symbol
		syms = append(syms, symbol{name: p.name, value: p.off, section: 1, class: symClassExternal})
	}
	add(symbol{name: ".data", section: 2, class: symClassStatic})

	textPtr := fileHeaderSize + 2*sectionHeaderSize
	dataPtr := textPtr + len(text)
	relPtr := dataPtr + len(data)
	symPtr := relPtr + len(relocs)*relocationSize

	var out bytes.Buffer
	le := binary.LittleEndian
	w16 := func(v uint16) { binary.Write(&out, le, v) }
	w32 := func(v uint32) { binary.Write(&out, le, v) }

	w16(imageFileMachineI386)
	w16(2)
	w32(ts)
	w32(uint32(symPtr))
	w32(uint32(len(syms)))
	w16(0)
	w16(fasmCharacteristics)

	section := func(name string, size, dataPtr, relPtr, nrel int, flags uint32) {
		var n [8]byte
		copy(n[:], name)
		out.Write(n[:])
		w32(0) // VirtualSize
		w32(0) // VirtualAddress
		w32(uint32(size))
		w32(uint32(dataPtr))
		w32(uint32(relPtr))
		w32(0) // PointerToLinenumbers
		w16(uint16(nrel))
		w16(0) // NumberOfLinenumbers
		w32(flags)
	}
	section(".text", len(text), textPtr, relPtr, len(relocs), flagsText)
	section(".data", len(data), dataPtr, 0, 0, flagsData)

	out.Write(text)
	out.Write(data)
	for _, r := range relocs {
		w32(r.off)
		w32(index[r.sym])
		w16(r.typ)
	}

	var strtab bytes.Buffer
	strtab.Write([]byte{0, 0, 0, 0}) // size, patched below
	for _, s := range syms {
		var name [8]byte
		if len(s.name) <= 8 {
			copy(name[:], s.name)
		} else {
			le.PutUint32(name[4:], uint32(strtab.Len()))
			strtab.WriteString(s.name)
			strtab.WriteByte(0)
		}
		out.Write(name[:])
		w32(s.value)
		binary.Write(&out, le, s.section)
		w16(0) // Type
		out.WriteByte(s.class)
		out.WriteByte(0) // NumberOfAuxSymbols
	}
	st := strtab.Bytes()
	le.PutUint32(st[0:], uint32(len(st)))
	out.Write(st)
	return out.Bytes(), nil
}
