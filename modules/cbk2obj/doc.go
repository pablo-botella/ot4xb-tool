// Package cbk2obj compiles an Xbase++ callback script (.cbk, the input of
// the legacy xppcbk "Callback Compiler for Xbase++") straight into a
// linkable x86-32 COFF object (.obj), optionally emitting the equivalent
// FASM source (.asm) as well. No assembler is involved.
//
// Xbase++ code cannot hand a PRG function to a Win32 API that wants a C
// callback (WNDPROC, EnumWindowsProc, hook procs, TIMERPROC, ...). The
// script names the callbacks and their C signatures; for each one the object
// provides:
//
//   - _CALLBACK_<NAME> (public): an Xbase++-callable function that returns
//     the address of the thunk, so PRG code does nProc := _CALLBACK_<NAME>()
//     and passes nProc to the API;
//   - _static_cb_<name>: the thunk the OS calls. It wraps the raw stack
//     arguments into Xbase++ containers, calls the PRG function <name> by
//     name through the runtime, converts the result back and returns it in
//     eax / edx:eax / st0 with the stdcall or cdecl epilogue;
//   - a .data string with the PRG function name.
//
// The generated code is the legacy xppcbk 1.0.17 code with its known defects
// fixed: QWORD results return the high dword in edx, VOID results leave eax
// alone, the ot4xb helpers are called by the names ot4xb exports (the legacy
// mixed one and two underscores), no __fltused (nothing in an Xbase++ link
// resolves it), and integer results go through ot4xb's _conGetLong, which
// keeps the 32-bit representation of the value whatever numeric type the
// PRG function returned.
//
// On the PRG side the arguments arrive as Xbase++ values: integers
// (BYTE/WORD/DWORD, pointers included) and FLOAT/DOUBLE as numerics, BOOL
// as a logical, QWORD as an eight-byte character value (the ot4xb
// convention of _conPutQWord / qwFpCall). Results go back the same way: the
// PRG function returns a numeric for integer and floating-point callbacks,
// a logical for BOOL and an eight-byte character value for QWORD
// (_conGetQWord reads eight binary bytes from a character container).
//
// Externals resolve from two import libraries an Xbase++ application has
// anyway: XppRt1.lib (the runtime, always linked by ALINK: _retnl, _conCall,
// _conCallPa, _conNew, _conRelease, _conPutNL, _conPutND, _conGetND, _conPutL,
// _conGetL) and ot4xb.lib for the five C helpers (_conGetLong, _conPutFloat,
// _conGetFloat, _conPutQWord, _conGetQWord), which ot4xb registers with
// _CDECL_EXPORT_ in its .xbmac so that xbmac2h/def2lib20 import them under
// their own names. The object links with ot4xb.lib; nothing else is needed.
//
// Script syntax (one command per line, "//" starts a comment, tokens are
// separated by blanks, commands and type names are case-insensitive,
// callback names keep their case):
//
//	XPPCBK VERSION 001.000.017        // optional requirement (error above 001.000.017)
//	USING CDECL | USING STDCALL       // default convention of the next END CALLBACK
//	__CDECL | __STDCALL               // same, legacy spelling
//	CALLBACK <TEMPLATE> <name>        // one predefined callback (always stdcall)
//	BEGIN CALLBACK <name> RETURNS <TYPE>
//	   PARAM <TYPE>
//	END CALLBACK                      // closes with the default convention, then resets it to stdcall
//
// Templates: WNDPROC, DIALOGPROC, MSGBOXCALLBACK, the common-dialog hooks
// (CCHOOKPROC, CFHOOKPROC, FRHOOKPROC, OFNHOOKPROC, OFNHOOKPROCOLDSTYLE,
// PAGEPAINTHOOK, PAGESETUPHOOK, PRINTHOOKPROC, SETUPHOOKPROC), SENDASYNCPROC,
// TIMERPROC, ENUMCHILDPROC, ENUMTHREADWNDPROC, ENUMWINDOWSPROC and the
// SetWindowsHookEx procs (CALLWNDPROC, CALLWNDRETPROC, CBTPROC, DEBUGPROC,
// FOREGROUNDIDLEPROC, GETMSGPROC, JOURNALPLAYBACKPROC, JOURNALRECORDPROC,
// KEYBOARDPROC, LOWLEVELKEYBOARDPROC, LOWLEVELMOUSEPROC, MESSAGEPROC,
// MOUSEPROC, SHELLPROC, SYSMSGPROC). Types: VOID, BYTE/CHAR, WORD/SHORT/INT16,
// DWORD/LONG/ULONG/INT/UINT/INT32/LRESULT/LPARAM/POINTER32/POINTER/HANDLE/LPSTR,
// QWORD/LONGLONG/ULONGLONG/INT64, BOOL, DOUBLE, FLOAT.
//
// The package has a producer side (script.go: .cbk -> Script) and a consumer
// side (codegen.go: Script -> code model; asm.go: model -> FASM text;
// obj.go: model -> COFF bytes). Build and BuildFile in cbk2obj.go tie them
// together.
package cbk2obj
