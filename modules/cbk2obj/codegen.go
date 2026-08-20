// codegen.go is the first consumer stage: it turns a Script into the code
// model (unit) that asm.go renders as FASM text and obj.go encodes into
// COFF. The instruction templates and the frame layout are those of the
// legacy xppcbk (TCallBack.prg, TParam.prg, TReturn.prg, TExternals.prg);
// every instruction carries its exact assembler text, so the .asm output
// stays byte-compatible with the legacy tool where the code is the same.

package cbk2obj

import (
	"fmt"
	"strings"
)

// opcode identifies an instruction shape for the encoder; the assembler
// text travels alongside.
type opcode uint8

const (
	opPushEbp   opcode = iota // push ebp
	opMovEbpEsp               // mov ebp, esp
	opMovEspEbp               // mov esp, ebp
	opPopEbp                  // pop ebp
	opPushEax                 // push eax
	opSubEsp                  // sub esp, n
	opAddEsp                  // add esp, n
	opPushImm                 // push n
	opPushSym                 // push <label>         (its address: DIR32)
	opMovEaxMem               // mov eax, [ebp+d]
	opMovEdxMem               // mov edx, [ebp+d]
	opMovMemEax               // mov [ebp+d], eax
	opLeaEaxMem               // lea eax, [ebp+d]
	opMovEaxImm               // mov eax, n
	opAndEaxImm               // and eax, n
	opCall                    // call <external>      (REL32)
	opFldQword                // fld qword [ebp+d]
	opFldDword                // fld dword [ebp+d]
	opRet                     // ret n
)

// ins is one generated instruction: the assembler line and what the
// encoder needs (immediate or displacement, symbol).
type ins struct {
	text string
	op   opcode
	n    int32
	sym  string
}

// block is one labelled routine of .text.
type block struct {
	label  string
	public bool
	code   []ins
}

// datum is one .data item: the NUL-terminated PRG function name.
type datum struct {
	label string
	text  string // assembler line
	bytes []byte
}

// unit is the generated module.
type unit struct {
	externs []string // external symbols, declaration order
	blocks  []block  // .text routines in order: all entries, then all thunks
	data    []datum  // .data items in callback order
}

// genOptions are the generator knobs. legacy reproduces the xppcbk 1.0.17
// text, defects included; it exists for the byte-exact test against the
// legacy output and is never used for objects.
type genOptions struct {
	legacy bool
	seed   int // number of the first _xbfn_<N>_<name> symbol (default 1)
}

// names are the symbols whose spelling differs between the legacy text
// and the real code: the integer result conversion and the ot4xb helpers
// (the legacy mixed one and two underscores, see the package doc).
type names struct {
	getInt, putFloat, getFloat, putQWord, getQWord string
}

func (g genOptions) names() names {
	if g.legacy {
		return names{getInt: "__conGetNL", putFloat: "_conPutFloat", getFloat: "__conGetFloat", putQWord: "_conPutQWord", getQWord: "__conGetQWord"}
	}
	return names{getInt: "_conGetLong", putFloat: "_conPutFloat", getFloat: "_conGetFloat", putQWord: "_conPutQWord", getQWord: "_conGetQWord"}
}

// frame is the stack frame of one thunk (TCallBack:PrepareStack): below
// ebp the result variable, the result container and one container handle
// per parameter, laid out so that the handles form an array in parameter
// order; above ebp the caller's arguments.
type frame struct {
	size     int   // bytes reserved below ebp
	retVar   int   // [ebp-retVar]: result variable (unused for VOID)
	retCon   int   // [ebp-retCon]: result container handle
	conBase  int   // [ebp-conBase]: first entry of the container array
	paramPos []int // [ebp+paramPos[i]]: caller argument i
	conPos   []int // [ebp-conPos[i]]: container of argument i
	stack    int   // bytes of caller arguments (the N of a stdcall ret N)
}

func layout(cb *Callback) frame {
	var f frame
	if cb.Ret != Void {
		f.size = cb.Ret.Size()
	}
	f.retVar = f.size
	f.size += 4
	f.retCon = f.size
	n := len(cb.Params)
	f.paramPos = make([]int, n)
	f.conPos = make([]int, n)
	for i, k := range cb.Params {
		f.size += 4
		f.conPos[n-1-i] = f.size // the last parameter gets the smallest offset: the array is in memory order
		f.paramPos[i] = 8 + f.stack
		f.stack += k.Size()
	}
	if n > 0 {
		f.conBase = f.conPos[0]
	}
	return f
}

// generate builds the unit for s.
func generate(s *Script, g genOptions) *unit {
	nm := g.names()
	seed := g.seed
	if seed <= 0 {
		seed = 1
	}
	u := &unit{}
	syms := make([]string, len(s.Callbacks))
	for i := range s.Callbacks {
		syms[i] = fmt.Sprintf("_xbfn_%d_%s", seed+i, s.Callbacks[i].Name)
	}
	for i := range s.Callbacks {
		u.blocks = append(u.blocks, entryBlock(&s.Callbacks[i]))
	}
	for i := range s.Callbacks {
		cb := &s.Callbacks[i]
		u.blocks = append(u.blocks, thunkBlock(cb, syms[i], layout(cb), nm, g.legacy))
	}
	for i := range s.Callbacks {
		name := s.Callbacks[i].Name
		u.data = append(u.data, datum{label: syms[i], text: syms[i] + " db  '" + name + "',0", bytes: append([]byte(name), 0)})
	}
	u.externs = externs(s, u, nm, g.legacy)
	return u
}

// entryBlock is _CALLBACK_<NAME> (TCallBack:PutTheXBase): an Xbase++
// function returning the thunk address through _retnl(paramList, value).
func entryBlock(cb *Callback) block {
	b := &block{label: "_CALLBACK_" + strings.ToUpper(cb.Name), public: true}
	thunk := "_static_cb_" + cb.Name
	b.put("push ebp", opPushEbp, 0, "")
	b.put("mov ebp, esp", opMovEbpEsp, 0, "")
	b.put("push  "+thunk, opPushSym, 0, thunk)
	b.put("mov eax, [ebp+8]", opMovEaxMem, 8, "")
	b.pushEax()
	b.call("__retnl")
	b.put("add esp, 8", opAddEsp, 8, "")
	b.put("pop ebp", opPopEbp, 0, "")
	b.put("ret 0", opRet, 0, "")
	return *b
}

// thunkBlock is _static_cb_<name> (TCallBack:PutTheCall): the routine the
// OS calls with the C arguments.
func thunkBlock(cb *Callback, sym string, f frame, nm names, legacy bool) block {
	b := &block{label: "_static_cb_" + cb.Name}
	b.put("push ebp", opPushEbp, 0, "")
	b.put("mov ebp, esp", opMovEbpEsp, 0, "")
	if f.size > 0 {
		b.put(fmt.Sprintf("sub esp,%d", f.size), opSubEsp, int32(f.size), "")
	}
	b.initCon(cb.Ret, f, nm)
	for i, k := range cb.Params {
		b.putCon(k, f.paramPos[i], f.conPos[i], nm)
	}
	if n := len(cb.Params); n > 0 {
		// _conCallPa(resultCon, "name", n, &containers[0])
		b.put(fmt.Sprintf("lea eax,[ebp-%d]", f.conBase), opLeaEaxMem, int32(-f.conBase), "")
		b.pushEax()
		b.pushImm(n)
		b.put("push "+sym, opPushSym, 0, sym)
		b.movEaxCon(f.retCon)
		b.pushEax()
		b.call("__conCallPa")
		b.addEsp(16)
		for i := range cb.Params {
			b.conRelease(f.conPos[i])
		}
	} else {
		// _conCall(resultCon, "name", 0)
		b.pushImm(0)
		b.put("push "+sym, opPushSym, 0, sym)
		b.movEaxCon(f.retCon)
		b.pushEax()
		b.call("__conCall")
		b.addEsp(12)
	}
	b.getResult(cb.Ret, f, nm)
	b.conRelease(f.retCon)
	b.putResult(cb.Ret, f, legacy)
	b.put("mov esp, ebp", opMovEspEbp, 0, "")
	b.put("pop ebp", opPopEbp, 0, "")
	if cb.Conv == CDecl {
		b.put("ret 0", opRet, 0, "")
	} else {
		b.put(fmt.Sprintf("ret %d", f.stack), opRet, int32(f.stack), "")
	}
	return *b
}

func (b *block) put(text string, op opcode, n int32, sym string) {
	b.code = append(b.code, ins{text: text, op: op, n: n, sym: sym})
}

func (b *block) pushEax()        { b.put("push eax", opPushEax, 0, "") }
func (b *block) pushImm(n int)   { b.put(fmt.Sprintf("push %d", n), opPushImm, int32(n), "") }
func (b *block) call(sym string) { b.put("call "+sym, opCall, 0, sym) }
func (b *block) addEsp(n int)    { b.put(fmt.Sprintf("add esp,%d", n), opAddEsp, int32(n), "") }
func (b *block) andEax(n int)    { b.put(fmt.Sprintf("and eax, %d", n), opAndEaxImm, int32(n), "") }
func (b *block) movEaxArg(pos int) {
	b.put(fmt.Sprintf("mov eax,[ebp+%d]", pos), opMovEaxMem, int32(pos), "")
}
func (b *block) movEaxCon(pos int) {
	b.put(fmt.Sprintf("mov eax , [ebp-%d]", pos), opMovEaxMem, int32(-pos), "")
}
func (b *block) movConEax(pos int) {
	b.put(fmt.Sprintf("mov [ebp-%d] , eax", pos), opMovMemEax, int32(-pos), "")
}
func (b *block) movEaxVar(pos int) {
	b.put(fmt.Sprintf("mov eax, [ebp-%d]", pos), opMovEaxMem, int32(-pos), "")
}
func (b *block) zeroVar(pos int) {
	b.put("mov eax,0", opMovEaxImm, 0, "")
	b.movConEax(pos)
}

// initCon (TReturn:InitCon) creates the result container, zeroing the
// result variable first.
func (b *block) initCon(k Kind, f frame, nm names) {
	switch k {
	case Void:
		b.pushImm(0)
		b.call("__conNew")
		b.addEsp(4)
		b.movConEax(f.retCon)
	case Double, QWord:
		fn := "__conPutND"
		if k == QWord {
			fn = nm.putQWord
		}
		b.zeroVar(f.retVar)
		b.zeroVar(f.retVar - 4)
		b.pushImm(0)
		b.pushImm(0)
		b.pushImm(0)
		b.call(fn)
		b.addEsp(12)
		b.movConEax(f.retCon)
	default:
		fn := "__conPutNL"
		switch k {
		case Bool:
			fn = "__conPutL"
		case Float:
			fn = nm.putFloat
		}
		b.zeroVar(f.retVar)
		b.pushImm(0)
		b.pushImm(0)
		b.call(fn)
		b.addEsp(8)
		b.movConEax(f.retCon)
	}
}

// putCon (TParam:PutCon) wraps caller argument [ebp+ppos] into a container
// stored at [ebp-cpos].
func (b *block) putCon(k Kind, ppos, cpos int, nm names) {
	switch k {
	case Double, QWord:
		fn := "__conPutND"
		if k == QWord {
			fn = nm.putQWord
		}
		b.movEaxArg(ppos + 4)
		b.pushEax()
		b.movEaxArg(ppos)
		b.pushEax()
		b.pushImm(0)
		b.call(fn)
		b.addEsp(12)
		b.movConEax(cpos)
	default:
		b.movEaxArg(ppos)
		fn := "__conPutNL"
		switch k {
		case Byte:
			b.andEax(255)
		case Word:
			b.andEax(65535)
		case Bool:
			fn = "__conPutL"
		case Float:
			fn = nm.putFloat
		}
		b.pushEax()
		b.pushImm(0)
		b.call(fn)
		b.addEsp(8)
		b.movConEax(cpos)
	}
}

// conRelease releases the container stored at [ebp-pos].
func (b *block) conRelease(pos int) {
	b.movEaxCon(pos)
	b.pushEax()
	b.call("__conRelease")
	b.addEsp(4)
}

// getResult (TReturn:GetResult) converts the result container into the
// result variable.
func (b *block) getResult(k Kind, f frame, nm names) {
	if k == Void {
		return
	}
	var fn string
	switch k {
	case Bool:
		fn = "__conGetL"
	case Double:
		fn = "__conGetND"
	case Float:
		fn = nm.getFloat
	case QWord:
		fn = nm.getQWord
	default:
		fn = nm.getInt
	}
	b.put(fmt.Sprintf("lea eax , [ebp-%d]", f.retVar), opLeaEaxMem, int32(-f.retVar), "")
	b.pushEax()
	b.movEaxVar(f.retCon)
	b.pushEax()
	b.call(fn)
	b.addEsp(8)
}

// putResult (TReturn:PutResult) loads the result variable into the return
// register(s): eax, edx:eax for QWORD, st0 for DOUBLE and FLOAT.
func (b *block) putResult(k Kind, f frame, legacy bool) {
	switch k {
	case Void:
		if legacy { // defect C1: the legacy read the saved ebp into eax
			b.movEaxVar(0)
			b.andEax(255)
		}
	case Byte:
		b.movEaxVar(f.retVar)
		b.andEax(255)
	case Word:
		b.movEaxVar(f.retVar)
		b.andEax(65535)
	case DWord, Bool:
		b.movEaxVar(f.retVar)
	case QWord:
		b.movEaxVar(f.retVar)
		if legacy { // defect C2: the high dword went to eax instead of edx
			b.movEaxVar(f.retVar - 4)
		} else {
			b.put(fmt.Sprintf("mov edx, [ebp-%d]", f.retVar-4), opMovEdxMem, int32(-(f.retVar - 4)), "")
		}
	case Double:
		b.put(fmt.Sprintf("fld qword [ebp-%d]", f.retVar), opFldQword, int32(-f.retVar), "")
	case Float:
		b.put(fmt.Sprintf("fld dword [ebp-%d]", f.retVar), opFldDword, int32(-f.retVar), "")
	}
}

// externs lists the external symbols. The legacy declared them from type
// flags, used or not (TExternals.prg), __fltused included; the real code
// declares exactly the symbols it calls, in the same canonical order.
func externs(s *Script, u *unit, nm names, legacy bool) []string {
	if legacy {
		var dword, qword, boolean, double, float bool
		mark := func(k Kind) {
			switch k {
			case Byte, Word, DWord:
				dword = true
			case QWord:
				qword = true
			case Bool:
				boolean = true
			case Double:
				double = true
			case Float:
				float = true
			}
		}
		for _, cb := range s.Callbacks {
			mark(cb.Ret)
			for _, p := range cb.Params {
				mark(p)
			}
		}
		var out []string
		if double || float {
			out = append(out, "__fltused")
		}
		out = append(out, "__retnl", "__conCall", "__conCallPa", "__conNew", "__conRelease")
		if dword {
			out = append(out, "__conPutNL", "__conGetNL")
		}
		if double {
			out = append(out, "__conPutND", "__conGetND")
		}
		if boolean {
			out = append(out, "__conPutL", "__conGetL")
		}
		if float {
			out = append(out, "_conPutFloat", "_conGetFloat")
		}
		if qword {
			out = append(out, "_conPutQWord", "_conGetQWord")
		}
		return out
	}
	used := map[string]bool{}
	for _, b := range u.blocks {
		for _, in := range b.code {
			if in.op == opCall {
				used[in.sym] = true
			}
		}
	}
	var out []string
	for _, sym := range []string{
		"__retnl", "__conCall", "__conCallPa", "__conNew", "__conRelease",
		"__conPutNL", nm.getInt, "__conPutND", "__conGetND", "__conPutL", "__conGetL",
		nm.putFloat, nm.getFloat, nm.putQWord, nm.getQWord,
	} {
		if used[sym] {
			out = append(out, sym)
		}
	}
	return out
}
