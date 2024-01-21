package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"
import (
	"context"
	"fmt"
	"strconv"
	"strings"

	refutils "github.com/grexie/refutils"
)

//export importModuleDynamicallyCallbackHandler
func importModuleDynamicallyCallbackHandler(info *C.ImportModuleDynamicallyCallbackInfo) (r C.CallResult) {
	ids := C.GoStringN(info.id.data, info.id.length)

	parts := strings.SplitN(ids, ":", 3)
	isolateId, _ := strconv.Atoi(parts[0])
	moduleId, _ := strconv.Atoi(parts[1])

	isolateRef := isolateRefs.Get(refutils.ID(isolateId))
	if isolateRef == nil {
		panic(fmt.Errorf("missing isolate pointer during callback for isolate #%d", isolateId))
	}
	isolate := isolateRef.(*Isolate)

	moduleRef := isolate.modules.Get(refutils.ID(moduleId))
	if moduleRef == nil {
		panic(fmt.Errorf("missing module pointer during callback for module #%d", moduleId))
	}
	module := moduleRef.(*Module)

	ctx := module.Context.isolate.GetExecutionContext()
	For(ctx).SetContext(module.Context)

	v, err := For(ctx).Sync(func(ctx context.Context) (any, error) {
		if resourceNamev, err := For(ctx).Context().newValueFromTuple(ctx, info.resourceName); err != nil {
			return nil, err
		} else if specifierv, err := For(ctx).Context().newValueFromTuple(ctx, info.specifier); err != nil {
			return nil, err
		} else if importAssertions, err := For(ctx).Context().newValuesFromTuples(ctx, info.importAssertions, info.importAssertionsLength); err != nil {
			return nil, err
		} else if resolver, err := For(ctx).Context().NewResolver(ctx); err != nil {
			return nil, err
		} else if promise, err := resolver.Promise(ctx); err != nil {
			return nil, err
		} else if resourceName, err := resourceNamev.StringValue(ctx); err != nil {
			return nil, err
		} else if specifier, err := specifierv.StringValue(ctx); err != nil {
			return nil, err
		} else {
			For(ctx).Background(func(ctx context.Context) {
				For(ctx).Context().AddMicrotask(ctx, func(in FunctionArgs) error {
					if exports, err := module.ImportModuleDynamically(in.ExecutionContext, specifier, resourceName, importAssertions); err != nil {
						return resolver.Reject(in.ExecutionContext, err)
					} else {
						return resolver.Resolve(in.ExecutionContext, exports)
					}
				})
			})

			return promise, nil
		}
	})

	if err != nil {
		if m, err := module.Context.Create(ctx, err); err != nil {
			m := err.Error()
			return C.v8_Value_ValueTuple_New_Error(module.Context.pointer, C.CString(m))
		} else {
			result := C.CallResult{}
			result.result = m.info
			C.v8_Value_ValueTuple_Retain(result.result)
			result.isError = C.bool(true)
			return result
		}
	} else {
		value := v.(*Value)

		C.v8_Value_ValueTuple_Retain(value.info)

		result := C.v8_CallResult()
		result.result = value.info
		return result
	}
}
