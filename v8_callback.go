package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fpic -std=c++11
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

type callbackArgs struct {
	Context *Context
	Caller  CallerInfo
	This    *Value
	Holder  *Value
}

func FunctionCallbackHandler(context *Context, info *C.CallbackInfo, args *callbackArgs, functionId ID) (*Value, error) {
	functionRef := context.functions.Get(functionId)
	if functionRef == nil {
		panic(fmt.Errorf("missing function pointer during callback for function #%d", functionId))
	}
	function := (functionRef.(*functionInfo)).Function

	argc := int(info.argc)
	pargv := (*[1 << (maxArraySize - 18)]C.ValueTuple)(unsafe.Pointer(info.argv))[:argc:argc]
	argv := make([]*Value, argc)
	for i := 0; i < argc; i++ {
		argv[i] = context.newValue(pargv[i].value, pargv[i].kinds)
	}

	return function(FunctionArgs{
		context,
		args.Caller,
		args.This,
		args.Holder,
		bool(info.isConstructCall),
		argv,
	})
}

func GetterCallbackHandler(context *Context, info *C.CallbackInfo, args *callbackArgs, accessorId ID) (*Value, error) {
	accessorRef := context.functions.Get(accessorId)
	if accessorRef == nil {
		panic(fmt.Errorf("missing function pointer during callback for getter #%d", accessorId))
	}
	getter := (accessorRef.(*accessorInfo)).Getter

	return getter(GetterArgs{
		context,
		args.Caller,
		args.This,
		args.Holder,
		C.GoStringN(info.key.data, info.key.length),
	})
}

func SetterCallbackHandler(context *Context, info *C.CallbackInfo, args *callbackArgs, accessorId ID) (*Value, error) {
	accessorRef := context.functions.Get(accessorId)
	if accessorRef == nil {
		panic(fmt.Errorf("missing function pointer during callback for setter #%d", accessorId))
	}
	setter := (accessorRef.(*accessorInfo)).Setter

	v := context.newValue(info.value.value, info.value.kinds)

	return nil, setter(SetterArgs{
		context,
		args.Caller,
		args.This,
		args.Holder,
		C.GoStringN(info.key.data, info.key.length),
		v,
	})
}

var callbackHandlers = map[C.CallbackType]func(*Context, *C.CallbackInfo, *callbackArgs, ID) (*Value, error){
	C.kFunctionCallback: FunctionCallbackHandler,
	C.kGetterCallback:   GetterCallbackHandler,
	C.kSetterCallback:   SetterCallbackHandler,
}

//export CallbackHandler
func CallbackHandler(info *C.CallbackInfo) (r C.ValueTuple) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered in callback handler", r)
		}
	}()

	id := C.GoStringN(info.id.data, info.id.length)
	parts := strings.SplitN(id, ":", 3)
	isolateId, _ := strconv.Atoi(parts[0])
	contextId, _ := strconv.Atoi(parts[1])
	callbackId, _ := strconv.Atoi(parts[2])

	isolateRef := isolates.Get(ID(isolateId))
	if isolateRef == nil {
		panic(fmt.Errorf("missing isolate pointer during callback for isolate #%d", isolateId))
	}
	isolate := isolateRef.(*Isolate)

	contextRef := isolate.contexts.Get(ID(contextId))
	if contextRef == nil {
		panic(fmt.Errorf("missing context pointer during callback for context #%d", contextId))
	}
	context := contextRef.(*Context)

	defer func() {
		if v := recover(); v != nil {
			err := fmt.Sprintf("panic during callback: %+v", v)
			r.error = C.Error{data: C.CString(err), length: C.int(len(err))}
		}
	}()

	callerInfo := CallerInfo{
		C.GoStringN(info.caller.funcname.data, info.caller.funcname.length),
		C.GoStringN(info.caller.filename.data, info.caller.filename.length),
		int(info.caller.line),
		int(info.caller.column),
	}

	self, _ := context.newValueFromTuple(info.self)
	holder, _ := context.newValueFromTuple(info.holder)

	args := &callbackArgs{context, callerInfo, self, holder}
	v, err := callbackHandlers[info._type](context, info, args, ID(callbackId))

	if err != nil {
		m := err.Error()
		cerr := C.Error{data: C.CString(m), length: C.int(len(m))}
		return C.ValueTuple{value: nil, kinds: 0, error: cerr}
	}

	if v == nil {
		return C.ValueTuple{}
	} else if v.context.isolate.pointer != context.isolate.pointer {
		m := fmt.Sprintf("callback returned a value from another isolate")
		cerr := C.Error{data: C.CString(m), length: C.int(len(m))}
		return C.ValueTuple{error: cerr}
	}

	return C.ValueTuple{value: v.pointer, kinds: C.Kinds(v.kinds)}
}
