package isolates

// #include "v8_c_bridge.h"
import "C"

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"unsafe"

	refutils "github.com/grexie/refutils"
)

type callbackArgs struct {
	Context *Context
	Caller  CallerInfo
	This    *Value
	Holder  *Value
}

func functionCallbackHandler(ctx context.Context, v8Context *Context, info C.CallbackInfo, args callbackArgs, functionId refutils.ID) (*Value, error) {
	return v8Context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		functionRef := v8Context.functions.Get(functionId)
		if functionRef == nil {
			panic(fmt.Errorf("missing function pointer during callback for function #%d", functionId))
		}
		function := (functionRef.(*functionInfo)).Function

		argc := int(info.argc)
		pargv := (*[1 << (maxArraySize - 18)]C.ValueTuple)(unsafe.Pointer(info.argv))[:argc:argc]
		argv := make([]*Value, argc)
		for i := 0; i < argc; i++ {
			argv[i] = v8Context.newValue(pargv[i].value, pargv[i].kinds)
		}

		return function(FunctionArgs{
			ctx,
			v8Context,
			args.Caller,
			args.This,
			args.Holder,
			bool(info.isConstructCall),
			argv,
		})
	})

}

func getterCallbackHandler(ctx context.Context, v8Context *Context, info C.CallbackInfo, args callbackArgs, accessorId refutils.ID) (*Value, error) {
	return v8Context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		accessorRef := v8Context.accessors.Get(accessorId)
		if accessorRef == nil {
			panic(fmt.Errorf("missing function pointer during callback for getter #%d", accessorId))
		}
		getter := (accessorRef.(*accessorInfo)).Getter

		return getter(GetterArgs{
			ctx,
			v8Context,
			args.Caller,
			args.This,
			args.Holder,
			C.GoStringN(info.key.data, info.key.length),
		})
	})
}

func setterCallbackHandler(ctx context.Context, v8Context *Context, info C.CallbackInfo, args callbackArgs, accessorId refutils.ID) (*Value, error) {
	return v8Context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		accessorRef := v8Context.accessors.Get(accessorId)
		if accessorRef == nil {
			panic(fmt.Errorf("missing function pointer during callback for setter #%d", accessorId))
		}
		setter := (accessorRef.(*accessorInfo)).Setter

		v := v8Context.newValue(info.value.value, info.value.kinds)

		return nil, setter(SetterArgs{
			ctx,
			v8Context,
			args.Caller,
			args.This,
			args.Holder,
			C.GoStringN(info.key.data, info.key.length),
			v,
		})
	})
}

var callbackHandlers = map[C.CallbackType]func(context.Context, *Context, C.CallbackInfo, callbackArgs, refutils.ID) (*Value, error){
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

	parts := strings.SplitN(ids, ":", 4)
	isolateId, _ := strconv.Atoi(parts[0])
	contextId, _ := strconv.Atoi(parts[1])
	callbackId, _ := strconv.Atoi(parts[2])
	executionContextId, _ := strconv.Atoi(parts[3])

	isolateRef := isolateRefs.Get(refutils.ID(isolateId))
	if isolateRef == nil {
		panic(fmt.Errorf("missing isolate pointer during callback for isolate #%d", isolateId))
	}
	isolate := isolateRef.(*Isolate)

	contextRef := isolate.contexts.Get(refutils.ID(contextId))
	if contextRef == nil {
		panic(fmt.Errorf("missing context pointer during callback for context #%d", contextId))
	}
	v8Context := contextRef.(*Context)

	executionContextRef := executionContextRefs.Get(refutils.ID(executionContextId))
	if executionContextRef == nil {
		panic(fmt.Errorf("missing execution context pointer during callback for execution context #%d", executionContextId))
	}
	ctx := executionContextRef.(*ExecutionContext)

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

	self, _ := v8Context.newValueFromTuple(ctx.ctx, info.self)
	holder, _ := v8Context.newValueFromTuple(ctx.ctx, info.holder)

	args := callbackArgs{v8Context, callerInfo, self, holder}
	v, err := callbackHandlers[info._type](ctx.ctx, v8Context, *info, args, refutils.ID(callbackId))

	if locked, err := isolate.lock(ctx.ctx); err != nil {
		return C.ValueTuple{}
	} else if locked {
		defer isolate.unlock(ctx.ctx)
	}

	if err != nil {
		m := err.Error()
		cerr := C.Error{data: C.CString(m), length: C.int(len(m))}
		return C.ValueTuple{value: nil, kinds: 0, error: cerr}
	}

	if v == nil {
		return C.ValueTuple{}
	} else if v.context.isolate.pointer != v8Context.isolate.pointer {
		m := fmt.Sprintf("callback returned a value from another isolate")
		cerr := C.Error{data: C.CString(m), length: C.int(len(m))}
		return C.ValueTuple{error: cerr}
	}

	return C.ValueTuple{value: v.pointer, kinds: C.Kinds(v.kinds)}
}
