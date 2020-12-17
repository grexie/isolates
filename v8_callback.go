package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++14
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"unsafe"

	refutils "github.com/behrsin/go-refutils"
)

type callbackArgs struct {
	Context *Context
	Caller  CallerInfo
	This    *Value
	Holder  *Value
}

func functionCallbackHandler(context *Context, info C.CallbackInfo, args callbackArgs, functionId refutils.ID) (*Value, error) {
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

func getterCallbackHandler(context *Context, info C.CallbackInfo, args callbackArgs, accessorId refutils.ID) (*Value, error) {
	accessorRef := context.accessors.Get(accessorId)
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

func setterCallbackHandler(context *Context, info C.CallbackInfo, args callbackArgs, accessorId refutils.ID) (*Value, error) {
	accessorRef := context.accessors.Get(accessorId)
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

var callbackHandlers = map[C.CallbackType]func(*Context, C.CallbackInfo, callbackArgs, refutils.ID) (*Value, error){
	C.kFunctionCallback: functionCallbackHandler,
	C.kGetterCallback:   getterCallbackHandler,
	C.kSetterCallback:   setterCallbackHandler,
}

//export callbackHandler
func callbackHandler(info *C.CallbackInfo) (r C.ValueTuple) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered in callback handler", r)
		}
	}()

	ids := C.GoStringN(info.id.data, info.id.length)

	parts := strings.SplitN(ids, ":", 3)
	isolateId, _ := strconv.Atoi(parts[0])
	contextId, _ := strconv.Atoi(parts[1])
	callbackId, _ := strconv.Atoi(parts[2])

	isolateRef := isolates.Get(refutils.ID(isolateId))
	if isolateRef == nil {
		panic(fmt.Errorf("missing isolate pointer during callback for isolate #%d", isolateId))
	}
	isolate := isolateRef.(*Isolate)

	contextRef := isolate.contexts.Get(refutils.ID(contextId))
	if contextRef == nil {
		panic(fmt.Errorf("missing context pointer during callback for context #%d", contextId))
	}
	context := contextRef.(*Context)

	defer func() {
		if v := recover(); v != nil {
			fmt.Printf("%+v\n", v)
			debug.PrintStack()
			err := fmt.Sprintf("%+v", v)
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

	args := callbackArgs{context, callerInfo, self, holder}
	v, err := callbackHandlers[info._type](context, *info, args, refutils.ID(callbackId))

	if err := isolate.lock(); err != nil {
		return C.ValueTuple{}
	} else {
		defer isolate.unlock()
	}

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
