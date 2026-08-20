package def2lib20

// coff.go produces the library: COFF (i386) objects for the four kinds of
// import member and the archive ("!<arch>") that holds them, laid out like the
// import libraries written by Microsoft LINK 2.60 (1995) that Alaska ALINK
// reads.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// importEntry is one function to import, already resolved by the caller.
type importEntry struct {
	symbol string // linker symbol (export name with Options.Prefix)
	name   string // name exported by the DLL (hint/name table)
	hint   uint16
}

// buildLibrary assembles the import library for one DLL.
func buildLibrary(dll string, nm naming, imports []importEntry, ts uint32) []byte {
	members := make([]member, 0, 3+len(imports))
	members = append(members,
		descriptorMember(dll, nm, ts),
		nullDescriptorMember(dll, nm, ts),
		nullThunkMember(dll, nm, ts),
	)
	for _, e := range imports {
		members = append(members, importMember(dll, nm, e, ts))
	}
	return writeArchive(members, ts)
}

// ---------------------------------------------------------------- COFF constants

const (
	imageFileMachineI386  = 0x014C
	imageFile32BitMachine = 0x0100

	imageScnCntCode       = 0x00000020
	imageScnCntInitData   = 0x00000040
	imageScnLnkComdat     = 0x00001000
	imageScnAlign1Bytes   = 0x00100000
	imageScnAlign2Bytes   = 0x00200000
	imageScnAlign4Bytes   = 0x00300000
	imageScnMemExecute    = 0x20000000
	imageScnMemRead       = 0x40000000
	imageScnMemWrite      = 0x80000000
	imageRelI386Dir32     = 6
	imageRelI386Dir32NB   = 7
	imageSymClassExternal = 2
	imageSymClassStatic   = 3
	imageSymClassSection  = 104
	imageSymDTypeFunction = 0x20
	imageComdatNoDups     = 1
	imageComdatAssoc      = 5
	imageSymUndefined     = 0

	fileHeaderSize     = 20
	optionalHeaderSize = 0xE0
	sectionHeaderSize  = 40
	relocationSize     = 10
	symbolSize         = 18

	// Section flags exactly as MS LINK 2.60 wrote them (KERNEL32.LIB, 1995).
	flagsIdata1 = imageScnCntInitData | imageScnAlign1Bytes | imageScnMemRead | imageScnMemWrite                     // C0100040
	flagsIdata2 = imageScnCntInitData | imageScnAlign2Bytes | imageScnMemRead | imageScnMemWrite                     // C0200040
	flagsIdata4 = imageScnCntInitData | imageScnAlign4Bytes | imageScnMemRead | imageScnMemWrite                     // C0300040
	flagsText   = imageScnCntCode | imageScnLnkComdat | imageScnAlign2Bytes | imageScnMemExecute | imageScnMemRead   // 60201020
	flagsIdata5 = imageScnCntInitData | imageScnLnkComdat | imageScnAlign4Bytes | imageScnMemRead | imageScnMemWrite // C0301040
	flagsIdata6 = imageScnCntInitData | imageScnLnkComdat | imageScnAlign2Bytes | imageScnMemRead | imageScnMemWrite // C0201040
)

// ---------------------------------------------------------------- COFF object model

type reloc struct {
	off    uint32
	symbol uint32 // symbol table index
	typ    uint16
}

type section struct {
	name   string
	data   []byte
	flags  uint32
	relocs []reloc
}

type symbol struct {
	name    string
	value   uint32
	section int16 // 1-based, 0 = undefined
	typ     uint16
	class   uint8
	aux     []byte // nil or an 18-byte auxiliary record
}

// auxSectionDef builds the auxiliary record of a section definition symbol:
// Length, NumberOfRelocations, NumberOfLinenumbers, CheckSum, Number, Selection.
func auxSectionDef(length uint32, nrelocs uint16, selection uint8, number uint16) []byte {
	b := make([]byte, symbolSize)
	binary.LittleEndian.PutUint32(b[0:], length)
	binary.LittleEndian.PutUint16(b[4:], nrelocs)
	binary.LittleEndian.PutUint16(b[12:], number)
	b[14] = selection
	return b
}

type object struct {
	sections       []section
	symbols        []symbol
	optionalHeader bool
	timestamp      uint32
}

// marshal serialises the object in the classic layout: file header,
// [optional header], section headers, then for each section its raw data
// followed by its relocations, then the symbol table and the string table.
func (o *object) marshal() []byte {
	var out bytes.Buffer
	le := binary.LittleEndian
	w16 := func(v uint16) { binary.Write(&out, le, v) }
	w32 := func(v uint32) { binary.Write(&out, le, v) }

	optSize := 0
	if o.optionalHeader {
		optSize = optionalHeaderSize
	}
	pos := fileHeaderSize + optSize + len(o.sections)*sectionHeaderSize
	dataPtr := make([]int, len(o.sections))
	relocPtr := make([]int, len(o.sections))
	for i, s := range o.sections {
		dataPtr[i] = pos
		pos += len(s.data)
		relocPtr[i] = pos
		pos += len(s.relocs) * relocationSize
	}
	symPtr := pos
	nsym := 0
	for _, s := range o.symbols {
		nsym++
		if s.aux != nil {
			nsym++
		}
	}

	w16(imageFileMachineI386)
	w16(uint16(len(o.sections)))
	w32(o.timestamp)
	w32(uint32(symPtr))
	w32(uint32(nsym))
	w16(uint16(optSize))
	w16(imageFile32BitMachine)
	if o.optionalHeader {
		out.Write(optionalHeader())
	}
	for i, s := range o.sections {
		var name [8]byte
		copy(name[:], s.name)
		out.Write(name[:])
		w32(0) // VirtualSize
		w32(0) // VirtualAddress
		w32(uint32(len(s.data)))
		w32(uint32(dataPtr[i]))
		if len(s.relocs) > 0 {
			w32(uint32(relocPtr[i]))
		} else {
			w32(0)
		}
		w32(0) // PointerToLinenumbers
		w16(uint16(len(s.relocs)))
		w16(0) // NumberOfLinenumbers
		w32(s.flags)
	}
	for _, s := range o.sections {
		out.Write(s.data)
		for _, r := range s.relocs {
			w32(r.off)
			w32(r.symbol)
			w16(r.typ)
		}
	}
	var strtab bytes.Buffer
	strtab.Write([]byte{0, 0, 0, 0}) // size, patched below
	for _, s := range o.symbols {
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
		w16(s.typ)
		out.WriteByte(s.class)
		if s.aux != nil {
			out.WriteByte(1)
			out.Write(s.aux)
		} else {
			out.WriteByte(0)
		}
	}
	st := strtab.Bytes()
	le.PutUint32(st[0:], uint32(len(st)))
	out.Write(st)
	return out.Bytes()
}

// optionalHeader is the PE32 optional header MS LINK 2.60 wrote into the
// import descriptor member: PE32 magic, linker version 2.60 and the default
// alignments/sizes, everything else zero.
func optionalHeader() []byte {
	b := make([]byte, optionalHeaderSize)
	le := binary.LittleEndian
	le.PutUint16(b[0:], 0x10B)     // PE32
	b[2] = 2                       // MajorLinkerVersion
	b[3] = 60                      // MinorLinkerVersion
	le.PutUint32(b[32:], 0x1000)   // SectionAlignment
	le.PutUint32(b[36:], 0x200)    // FileAlignment
	le.PutUint16(b[40:], 4)        // MajorOperatingSystemVersion
	le.PutUint32(b[72:], 0x100000) // SizeOfStackReserve
	le.PutUint32(b[76:], 0x1000)   // SizeOfStackCommit
	le.PutUint32(b[80:], 0x100000) // SizeOfHeapReserve
	le.PutUint32(b[84:], 0x1000)   // SizeOfHeapCommit
	le.PutUint32(b[92:], 16)       // NumberOfRvaAndSizes
	return b
}

// ---------------------------------------------------------------- import members

// member is one archive member.
type member struct {
	name    string   // archive member name (the DLL file name for import members)
	data    []byte   // COFF object
	symbols []string // public symbols defined by the object, in symbol-table order
}

// descriptorMember: .idata$2 import directory entry (relocated against the
// DLL name in its own .idata$6 and against the undefined section symbols
// .idata$4 / .idata$5, which the linker resolves to the start of the
// ILT / IAT built from all the members of the same archive-member name).
func descriptorMember(dll string, nm naming, ts uint32) member {
	name := append([]byte(dll), 0)
	if len(name)%2 == 1 {
		name = append(name, 0)
	}
	o := &object{optionalHeader: true, timestamp: ts}
	o.sections = []section{
		{name: ".idata$2", data: make([]byte, 20), flags: flagsIdata1, relocs: []reloc{
			{off: 12, symbol: 2, typ: imageRelI386Dir32NB}, // Name RVA -> .idata$6
			{off: 0, symbol: 3, typ: imageRelI386Dir32NB},  // Import Lookup Table RVA -> .idata$4
			{off: 16, symbol: 4, typ: imageRelI386Dir32NB}, // Import Address Table RVA -> .idata$5
		}},
		{name: ".idata$6", data: name, flags: flagsIdata2},
	}
	o.symbols = []symbol{
		{name: nm.descriptor, section: 1, class: imageSymClassExternal},
		{name: ".idata$2", value: flagsIdata1, section: 1, class: imageSymClassSection},
		{name: ".idata$6", section: 2, class: imageSymClassStatic},
		{name: ".idata$4", value: flagsIdata4, section: imageSymUndefined, class: imageSymClassSection},
		{name: ".idata$5", value: flagsIdata4, section: imageSymUndefined, class: imageSymClassSection},
		{name: nm.nullDescriptor, section: imageSymUndefined, class: imageSymClassExternal},
		{name: nm.nullThunk, section: imageSymUndefined, class: imageSymClassExternal},
	}
	return member{name: dll, data: o.marshal(), symbols: []string{nm.descriptor}}
}

// nullDescriptorMember: the 20 zero bytes in .idata$3 that terminate the
// import directory table.
func nullDescriptorMember(dll string, nm naming, ts uint32) member {
	o := &object{timestamp: ts}
	o.sections = []section{{name: ".idata$3", data: make([]byte, 20), flags: flagsIdata1}}
	o.symbols = []symbol{{name: nm.nullDescriptor, section: 1, class: imageSymClassExternal}}
	return member{name: dll, data: o.marshal(), symbols: []string{nm.nullDescriptor}}
}

// nullThunkMember: the zero entries that terminate this DLL's IAT and ILT.
func nullThunkMember(dll string, nm naming, ts uint32) member {
	o := &object{timestamp: ts}
	o.sections = []section{
		{name: ".idata$5", data: make([]byte, 4), flags: flagsIdata4},
		{name: ".idata$4", data: make([]byte, 4), flags: flagsIdata4},
	}
	o.symbols = []symbol{{name: nm.nullThunk, section: 1, class: imageSymClassExternal}}
	return member{name: dll, data: o.marshal(), symbols: []string{nm.nullThunk}}
}

// importMember imports one function by name: the thunk symbol (jmp [__imp_x])
// in a COMDAT .text, the IAT slot __imp_x in a COMDAT .idata$5, the ILT slot
// in an associative .idata$4 and the hint/name entry in an associative .idata$6.
func importMember(dll string, nm naming, e importEntry, ts uint32) member {
	hn := make([]byte, 2, 2+len(e.name)+2)
	binary.LittleEndian.PutUint16(hn, e.hint)
	hn = append(hn, e.name...)
	hn = append(hn, 0)
	if len(hn)%2 == 1 {
		hn = append(hn, 0)
	}
	imp := "__imp_" + e.symbol
	o := &object{timestamp: ts}
	o.sections = []section{
		{name: ".text", data: []byte{0xFF, 0x25, 0, 0, 0, 0}, flags: flagsText, relocs: []reloc{
			{off: 2, symbol: 5, typ: imageRelI386Dir32}, // jmp dword ptr [__imp_x]
		}},
		{name: ".idata$5", data: make([]byte, 4), flags: flagsIdata5, relocs: []reloc{
			{off: 0, symbol: 8, typ: imageRelI386Dir32NB}, // -> hint/name
		}},
		{name: ".idata$4", data: make([]byte, 4), flags: flagsIdata5, relocs: []reloc{
			{off: 0, symbol: 8, typ: imageRelI386Dir32NB}, // -> hint/name
		}},
		{name: ".idata$6", data: hn, flags: flagsIdata6},
	}
	o.symbols = []symbol{
		{name: ".text", section: 1, class: imageSymClassStatic, aux: auxSectionDef(6, 1, imageComdatNoDups, 0)},                 // 0,1
		{name: e.symbol, section: 1, typ: imageSymDTypeFunction, class: imageSymClassExternal},                                  // 2
		{name: ".idata$5", section: 2, class: imageSymClassStatic, aux: auxSectionDef(4, 1, imageComdatNoDups, 0)},              // 3,4
		{name: imp, section: 2, class: imageSymClassExternal},                                                                   // 5
		{name: ".idata$4", section: 3, class: imageSymClassStatic, aux: auxSectionDef(4, 1, imageComdatAssoc, 2)},               // 6,7
		{name: ".idata$6", section: 4, class: imageSymClassStatic, aux: auxSectionDef(uint32(len(hn)), 0, imageComdatAssoc, 2)}, // 8,9
		{name: nm.descriptor, section: imageSymUndefined, class: imageSymClassExternal},                                         // 10
	}
	return member{name: dll, data: o.marshal(), symbols: []string{e.symbol, imp}}
}

// ---------------------------------------------------------------- archive

// writeArchive lays out "!<arch>\n", the first linker member (big-endian
// directory in member order), the second linker member (little-endian
// directory, symbols in lexical order), the longnames member when some member
// name does not fit in 16 bytes (as MS LINK 2.60 did: omitted otherwise), and
// the members, each header on an even offset.
func writeArchive(members []member, ts uint32) []byte {
	type symref struct {
		name string
		mem  int
	}
	var syms []symref
	for i, m := range members {
		for _, s := range m.symbols {
			syms = append(syms, symref{s, i})
		}
	}
	var longnames bytes.Buffer
	headerName := make([]string, len(members))
	for i, m := range members {
		n := m.name + "/"
		if len(n) <= 16 {
			headerName[i] = n
			continue
		}
		headerName[i] = fmt.Sprintf("/%d", longnames.Len())
		longnames.WriteString(m.name)
		longnames.WriteByte(0)
	}
	even := func(n int) int { return n + n%2 }
	strSize := 0
	for _, s := range syms {
		strSize += len(s.name) + 1
	}
	lm1Size := 4 + 4*len(syms) + strSize
	lm2Size := 4 + 4*len(members) + 4 + 2*len(syms) + strSize
	off := 8 + 60 + even(lm1Size) + 60 + even(lm2Size)
	if longnames.Len() > 0 {
		off += 60 + even(longnames.Len())
	}
	memberOff := make([]int, len(members))
	for i, m := range members {
		memberOff[i] = off
		off += 60 + even(len(m.data))
	}

	var lm1 bytes.Buffer
	binary.Write(&lm1, binary.BigEndian, uint32(len(syms)))
	for _, s := range syms {
		binary.Write(&lm1, binary.BigEndian, uint32(memberOff[s.mem]))
	}
	for _, s := range syms {
		lm1.WriteString(s.name)
		lm1.WriteByte(0)
	}

	sorted := make([]symref, len(syms))
	copy(sorted, syms)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].name < sorted[b].name })
	var lm2 bytes.Buffer
	binary.Write(&lm2, binary.LittleEndian, uint32(len(members)))
	for i := range members {
		binary.Write(&lm2, binary.LittleEndian, uint32(memberOff[i]))
	}
	binary.Write(&lm2, binary.LittleEndian, uint32(len(sorted)))
	for _, s := range sorted {
		binary.Write(&lm2, binary.LittleEndian, uint16(s.mem+1))
	}
	for _, s := range sorted {
		lm2.WriteString(s.name)
		lm2.WriteByte(0)
	}

	var out bytes.Buffer
	out.WriteString("!<arch>\n")
	put := func(name string, data []byte) {
		fmt.Fprintf(&out, "%-16s%-12d%-6s%-6s%-8s%-10d`\n", name, ts, "", "", "0", len(data))
		out.Write(data)
		if len(data)%2 == 1 {
			out.WriteByte('\n')
		}
	}
	put("/", lm1.Bytes())
	put("/", lm2.Bytes())
	if longnames.Len() > 0 {
		put("//", longnames.Bytes())
	}
	for i, m := range members {
		if out.Len() != memberOff[i] {
			panic(fmt.Sprintf("def2lib20: member %d offset %d, expected %d", i, out.Len(), memberOff[i]))
		}
		put(headerName[i], m.data)
	}
	return out.Bytes()
}
