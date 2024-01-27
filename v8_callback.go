package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"fmt"
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
	pv, err := v8Context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		functionRef := v8Context.functions.Get(functionId)
		if functionRef == nil {
			panic(fmt.Errorf("missing function pointer during callback for function #%d", functionId))
		}
		function := (functionRef.(*functionInfo)).Function

		argc := int(info.argc)
		pargv := (*[1 << (maxArraySize - 18)]C.CallResult)(unsafe.Pointer(info.argv))[:argc:argc]
		argv := make([]*Value, argc)
		for i := 0; i < argc; i++ {
			var err error
			if argv[i], err = v8Context.newValueFromTuple(ctx, pargv[i]); err != nil {
				return nil, err
			}
		}

		return function(FunctionArgs{
			ctx,
			v8Context,
			args.This,
			bool(info.isConstructCall),
			argv,
			args.Caller,
			args.Holder,
			nil,
		})
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), err
	}

}

func getterCallbackHandler(ctx context.Context, v8Context *Context, info C.CallbackInfo, args callbackArgs, accessorId refutils.ID) (*Value, error) {
	pv, err := v8Context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

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

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func setterCallbackHandler(ctx context.Context, v8Context *Context, info C.CallbackInfo, args callbackArgs, accessorId refutils.ID) (*Value, error) {
	pv, err := v8Context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		accessorRef := v8Context.accessors.Get(accessorId)
		if accessorRef == nil {
			panic(fmt.Errorf("missing function pointer during callback for setter #%d", accessorId))
		}
		setter := (accessorRef.(*accessorInfo)).Setter

		if v, err := v8Context.newValueFromTuple(ctx, info.value); err != nil {
			return nil, err
		} else {
			return nil, setter(SetterArgs{
				ctx,
				v8Context,
				args.Caller,
				args.This,
				args.Holder,
				C.GoStringN(info.key.data, info.key.length),
				v,
			})
		}
	})

	if err != nil {
		return nil, err
	} else if pv != nil {
		return pv.(*Value), nil
	} else {
		return nil, nil
	}
}

var callbackHandlers = map[C.CallbackType]func(context.Context, *Context, C.CallbackInfo, callbackArgs, refutils.ID) (*Value, error){
	C.kFunctionCallback: functionCallbackHandler,
	C.kGetterCallback:   getterCallbackHandler,
	C.kSetterCallback:   setterCallbackHandler,
}

//export callbackHandler
func callbackHandler(info *C.CallbackInfo) (r C.CallResult) {
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

	ctx := isolate.GetExecutionContext()
	For(ctx).SetContext(v8Context)

	callerInfo := CallerInfo{
		C.GoStringN(info.caller.funcname.data, info.caller.funcname.length),
		C.GoStringN(info.caller.filename.data, info.caller.filename.length),
		int(info.caller.line),
		int(info.caller.column),
	}

	vt, _ := isolate.Sync(ctx, func(ctx context.Context) (any, error) {
		self, _ := v8Context.newValueFromTuple(ctx, info.self)
		holder, _ := v8Context.newValueFromTuple(ctx, info.holder)

		args := callbackArgs{v8Context, callerInfo, self, holder}

		v, err := callbackHandlers[info._type](ctx, v8Context, *info, args, refutils.ID(callbackId))

		if v == nil && err == nil {
			v, err = v8Context.Undefined(ctx)
		}

		if err != nil {
			if m, err := v8Context.Create(ctx, err); err != nil {
				m := err.Error()
				return C.v8_Value_ValueTuple_New_Error(v8Context.pointer, C.CString(m)), nil
			} else {
				result := C.CallResult{}
				result.result = m.info
				C.v8_Value_ValueTuple_Retain(result.result)
				result.isError = C.bool(true)
				return result, nil
			}
		}

		if v.context.isolate.pointer != v8Context.isolate.pointer {
			m := fmt.Sprintf("callback returned a value from another isolate")
			return C.v8_Value_ValueTuple_New_Error(v8Context.pointer, C.CString(m)), nil
		}

		C.v8_Value_ValueTuple_Retain(v.info)

		result := C.v8_CallResult()
		result.result = v.info
		return result, nil
	})

	return vt.(C.CallResult)
}
