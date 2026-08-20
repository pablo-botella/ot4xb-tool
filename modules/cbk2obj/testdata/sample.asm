format MS COFF
extrn __retnl
extrn __conCall
extrn __conCallPa
extrn __conNew
extrn __conRelease
extrn __conPutNL
extrn _conGetLong
extrn __conPutND
extrn __conGetND
extrn __conPutL
extrn __conGetL
extrn _conPutFloat
extrn _conPutQWord
extrn _conGetQWord
section '.text' code readable executable
public  _CALLBACK_MYWNDPROC
public  _CALLBACK_MYENUM
public  _CALLBACK_MYCMP
public  _CALLBACK_MYVOID
public  _CALLBACK_MYQ
public  _CALLBACK_MYDBL
_CALLBACK_MYWNDPROC:
push ebp
mov ebp, esp
push  _static_cb_MyWndProc
mov eax, [ebp+8]
push eax
call __retnl
add esp, 8
pop ebp
ret 0
_CALLBACK_MYENUM:
push ebp
mov ebp, esp
push  _static_cb_MyEnum
mov eax, [ebp+8]
push eax
call __retnl
add esp, 8
pop ebp
ret 0
_CALLBACK_MYCMP:
push ebp
mov ebp, esp
push  _static_cb_MyCmp
mov eax, [ebp+8]
push eax
call __retnl
add esp, 8
pop ebp
ret 0
_CALLBACK_MYVOID:
push ebp
mov ebp, esp
push  _static_cb_MyVoid
mov eax, [ebp+8]
push eax
call __retnl
add esp, 8
pop ebp
ret 0
_CALLBACK_MYQ:
push ebp
mov ebp, esp
push  _static_cb_MyQ
mov eax, [ebp+8]
push eax
call __retnl
add esp, 8
pop ebp
ret 0
_CALLBACK_MYDBL:
push ebp
mov ebp, esp
push  _static_cb_MyDbl
mov eax, [ebp+8]
push eax
call __retnl
add esp, 8
pop ebp
ret 0
_static_cb_MyWndProc:
push ebp
mov ebp, esp
sub esp,24
mov eax,0
mov [ebp-4] , eax
push 0
push 0
call __conPutNL
add esp,8
mov [ebp-8] , eax
mov eax,[ebp+8]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-24] , eax
mov eax,[ebp+12]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-20] , eax
mov eax,[ebp+16]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-16] , eax
mov eax,[ebp+20]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-12] , eax
lea eax,[ebp-24]
push eax
push 4
push _xbfn_1_MyWndProc
mov eax , [ebp-8]
push eax
call __conCallPa
add esp,16
mov eax , [ebp-24]
push eax
call __conRelease
add esp,4
mov eax , [ebp-20]
push eax
call __conRelease
add esp,4
mov eax , [ebp-16]
push eax
call __conRelease
add esp,4
mov eax , [ebp-12]
push eax
call __conRelease
add esp,4
lea eax , [ebp-4]
push eax
mov eax, [ebp-8]
push eax
call _conGetLong
add esp,8
mov eax , [ebp-8]
push eax
call __conRelease
add esp,4
mov eax, [ebp-4]
mov esp, ebp
pop ebp
ret 16
_static_cb_MyEnum:
push ebp
mov ebp, esp
sub esp,16
mov eax,0
mov [ebp-4] , eax
push 0
push 0
call __conPutL
add esp,8
mov [ebp-8] , eax
mov eax,[ebp+8]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-16] , eax
mov eax,[ebp+12]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-12] , eax
lea eax,[ebp-16]
push eax
push 2
push _xbfn_2_MyEnum
mov eax , [ebp-8]
push eax
call __conCallPa
add esp,16
mov eax , [ebp-16]
push eax
call __conRelease
add esp,4
mov eax , [ebp-12]
push eax
call __conRelease
add esp,4
lea eax , [ebp-4]
push eax
mov eax, [ebp-8]
push eax
call __conGetL
add esp,8
mov eax , [ebp-8]
push eax
call __conRelease
add esp,4
mov eax, [ebp-4]
mov esp, ebp
pop ebp
ret 8
_static_cb_MyCmp:
push ebp
mov ebp, esp
sub esp,20
mov eax,0
mov [ebp-4] , eax
push 0
push 0
call __conPutNL
add esp,8
mov [ebp-8] , eax
mov eax,[ebp+8]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-20] , eax
mov eax,[ebp+16]
push eax
mov eax,[ebp+12]
push eax
push 0
call __conPutND
add esp,12
mov [ebp-16] , eax
mov eax,[ebp+20]
push eax
push 0
call __conPutL
add esp,8
mov [ebp-12] , eax
lea eax,[ebp-20]
push eax
push 3
push _xbfn_3_MyCmp
mov eax , [ebp-8]
push eax
call __conCallPa
add esp,16
mov eax , [ebp-20]
push eax
call __conRelease
add esp,4
mov eax , [ebp-16]
push eax
call __conRelease
add esp,4
mov eax , [ebp-12]
push eax
call __conRelease
add esp,4
lea eax , [ebp-4]
push eax
mov eax, [ebp-8]
push eax
call _conGetLong
add esp,8
mov eax , [ebp-8]
push eax
call __conRelease
add esp,4
mov eax, [ebp-4]
mov esp, ebp
pop ebp
ret 16
_static_cb_MyVoid:
push ebp
mov ebp, esp
sub esp,4
push 0
call __conNew
add esp,4
mov [ebp-4] , eax
push 0
push _xbfn_4_MyVoid
mov eax , [ebp-4]
push eax
call __conCall
add esp,12
mov eax , [ebp-4]
push eax
call __conRelease
add esp,4
mov esp, ebp
pop ebp
ret 0
_static_cb_MyQ:
push ebp
mov ebp, esp
sub esp,28
mov eax,0
mov [ebp-8] , eax
mov eax,0
mov [ebp-4] , eax
push 0
push 0
push 0
call _conPutQWord
add esp,12
mov [ebp-12] , eax
mov eax,[ebp+12]
push eax
mov eax,[ebp+8]
push eax
push 0
call _conPutQWord
add esp,12
mov [ebp-28] , eax
mov eax,[ebp+16]
push eax
push 0
call _conPutFloat
add esp,8
mov [ebp-24] , eax
mov eax,[ebp+20]
and eax, 65535
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-20] , eax
mov eax,[ebp+24]
and eax, 255
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-16] , eax
lea eax,[ebp-28]
push eax
push 4
push _xbfn_5_MyQ
mov eax , [ebp-12]
push eax
call __conCallPa
add esp,16
mov eax , [ebp-28]
push eax
call __conRelease
add esp,4
mov eax , [ebp-24]
push eax
call __conRelease
add esp,4
mov eax , [ebp-20]
push eax
call __conRelease
add esp,4
mov eax , [ebp-16]
push eax
call __conRelease
add esp,4
lea eax , [ebp-8]
push eax
mov eax, [ebp-12]
push eax
call _conGetQWord
add esp,8
mov eax , [ebp-12]
push eax
call __conRelease
add esp,4
mov eax, [ebp-8]
mov edx, [ebp-4]
mov esp, ebp
pop ebp
ret 20
_static_cb_MyDbl:
push ebp
mov ebp, esp
sub esp,16
mov eax,0
mov [ebp-8] , eax
mov eax,0
mov [ebp-4] , eax
push 0
push 0
push 0
call __conPutND
add esp,12
mov [ebp-12] , eax
mov eax,[ebp+8]
push eax
push 0
call __conPutNL
add esp,8
mov [ebp-16] , eax
lea eax,[ebp-16]
push eax
push 1
push _xbfn_6_MyDbl
mov eax , [ebp-12]
push eax
call __conCallPa
add esp,16
mov eax , [ebp-16]
push eax
call __conRelease
add esp,4
lea eax , [ebp-8]
push eax
mov eax, [ebp-12]
push eax
call __conGetND
add esp,8
mov eax , [ebp-12]
push eax
call __conRelease
add esp,4
fld qword [ebp-8]
mov esp, ebp
pop ebp
ret 0
section '.data' data readable writeable
_xbfn_1_MyWndProc db  'MyWndProc',0
_xbfn_2_MyEnum db  'MyEnum',0
_xbfn_3_MyCmp db  'MyCmp',0
_xbfn_4_MyVoid db  'MyVoid',0
_xbfn_5_MyQ db  'MyQ',0
_xbfn_6_MyDbl db  'MyDbl',0
