package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"time"
	"unsafe"

	refutils "github.com/grexie/refutils"
)

type Value struct {
	refutils.RefHolder

	context *Context
	pointer C.ValuePtr
	kinds   kinds
	// finalizers []func()
}

type PropertyDescriptor struct {
	Get          *Value
	Set          *Value
	Value        *Value
	Enumerable   bool
	Configurable bool
	Writable     bool
}

func (c *Context) newValueFromTuple(ctx context.Context, vt C.ValueTuple) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		return c.newValue(vt.value, vt.kinds), c.isolate.newError(vt.error)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) newValue(pointer C.ValuePtr, k C.Kinds) *Value {
	if pointer == nil {
		return nil
	}

	v := &Value{
		context: c,
		pointer: pointer,
		kinds:   kinds(k),
		// finalizers: make([]func(), 0),
	}
	runtime.SetFinalizer(v, (*Value).release)

	tracer.Add(v)

	return v
}

func (v *Value) Ref() refutils.ID {
	return v.context.refs.Ref(v)
}

func (v *Value) Unref() {
	v.context.refs.Unref(v)
}

func (v *Value) IsKind(k Kind) bool {
	return v.kinds.Is(k)
}

func (v *Value) GetContext() *Context {
	return v.context
}

func (v *Value) DefineProperty(ctx context.Context, key string, descriptor *PropertyDescriptor) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pk := C.CString(key)
		var err C.Error
		if descriptor.Value != nil || descriptor.Get == nil && descriptor.Set == nil {
			err = C.v8_Value_DefinePropertyValue(v.context.pointer, v.pointer, pk, descriptor.Value.pointer, C.bool(descriptor.Configurable), C.bool(descriptor.Enumerable), C.bool(descriptor.Writable))
		} else {
			err = C.v8_Value_DefineProperty(v.context.pointer, v.pointer, pk, descriptor.Get.pointer, descriptor.Set.pointer, C.bool(descriptor.Configurable), C.bool(descriptor.Enumerable))
		}
		C.free(unsafe.Pointer(pk))
		return nil, v.context.isolate.newError(err)
	})

	return err
}

func (v *Value) Get(ctx context.Context, key string) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pk := C.CString(key)
		vt := C.v8_Value_Get(v.context.pointer, v.pointer, pk)
		C.free(unsafe.Pointer(pk))
		return v.context.newValueFromTuple(ctx, vt)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) GetLength(ctx context.Context) (int64, error) {
	if length, err := v.Get(ctx, "length"); err != nil {
		return 0, err
	} else {
		return length.Int64(ctx)
	}
}

func (v *Value) GetIndex(ctx context.Context, i int) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		return v.context.newValueFromTuple(ctx, C.v8_Value_GetIndex(v.context.pointer, v.pointer, C.int(i)))
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) Set(ctx context.Context, key string, value *Value) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pk := C.CString(key)
		err := C.v8_Value_Set(v.context.pointer, v.pointer, pk, value.pointer)
		C.free(unsafe.Pointer(pk))
		return nil, v.context.isolate.newError(err)
	})

	return err
}

func (v *Value) SetIndex(ctx context.Context, i int, value *Value) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, v.context.isolate.newError(C.v8_Value_SetIndex(v.context.pointer, v.pointer, C.int(i), value.pointer))
	})

	return err
}

func (v *Value) SetInternalField(ctx context.Context, i int, value uint32) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		v.context.ref()
		defer v.context.unref()

		C.v8_Object_SetInternalField(v.context.pointer, v.pointer, C.int(i), C.uint32_t(value))
		return nil, nil
	})

	return err
}

func (v *Value) GetInternalField(ctx context.Context, i int) (int64, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		v.context.ref()
		defer v.context.unref()

		i := int64(C.v8_Object_GetInternalField(v.context.pointer, v.pointer, C.int(i)))
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int64)), nil
	}
}

func (v *Value) GetInternalFieldCount(ctx context.Context) (int, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		v.context.ref()
		defer v.context.unref()

		i := int(C.v8_Object_GetInternalFieldCount(v.context.pointer, v.pointer))
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int)), nil
	}
}

func (v *Value) Bind(ctx context.Context, argv ...any) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if bind, err := v.Get(ctx, "bind"); err != nil {
			return nil, err
		} else if fn, err := bind.Call(ctx, v, argv...); err != nil {
			return nil, err
		} else {
			return fn, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) Call(ctx context.Context, self any, args ...any) (*Value, error) {
	var this *Value
	var err error

	if this, err = v.context.Create(ctx, self); err != nil {
		return nil, err
	}

	argsv := make([]*Value, len(args))
	for i, arg := range args {
		if argv, err := v.context.Create(ctx, arg); err != nil {
			return nil, err
		} else {
			argsv[i] = argv
		}
	}

	return v.CallValue(ctx, this, argsv...)
}

func (v *Value) CallValue(ctx context.Context, self *Value, argv ...*Value) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pargv := make([]C.ValuePtr, len(argv)+1)
		for i, argvi := range argv {
			pargv[i] = argvi.pointer
		}

		pself := C.ValuePtr(nil)
		if self != nil {
			pself = self.pointer
		}

		v.context.ref()
		defer v.context.unref()

		vt := C.v8_Value_Call(v.context.pointer, v.pointer, pself, C.int(len(argv)), &pargv[0])
		return v.context.newValueFromTuple(ctx, vt)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}

}

func (v *Value) CallMethod(ctx context.Context, name string, argv ...any) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if m, err := v.Get(ctx, name); err != nil {
			return nil, err
		} else if value, err := m.Call(ctx, v, argv...); err != nil {
			return nil, err
		} else {
			return value, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) New(ctx context.Context, args ...any) (*Value, error) {
	argsv := make([]*Value, len(args))
	for i, arg := range args {
		if argv, err := v.context.Create(ctx, arg); err != nil {
			return nil, err
		} else {
			argsv[i] = argv
		}
	}
	return v.NewValue(ctx, argsv...)
}

func (v *Value) NewValue(ctx context.Context, argv ...*Value) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pargv := make([]C.ValuePtr, len(argv)+1)
		for i, argvi := range argv {
			pargv[i] = argvi.pointer
		}
		v.context.ref()
		vt := C.v8_Value_New(v.context.pointer, v.pointer, C.int(len(argv)), &pargv[0])
		v.context.unref()

		return v.context.newValueFromTuple(ctx, vt)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) Bytes(ctx context.Context) ([]byte, error) {
	b, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := C.v8_Value_Bytes(v.context.pointer, v.pointer)
		if b.data == nil {
			return nil, nil
		}

		buf := C.GoBytes(unsafe.Pointer(b.data), b.length)

		return buf, nil
	})

	if err != nil {
		return nil, err
	} else if b == nil {
		return nil, nil
	} else {
		return b.([]byte), nil
	}
}

func (v *Value) SetBytes(ctx context.Context, bytes []byte) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := C.v8_Value_Bytes(v.context.pointer, v.pointer)
		if b.data == nil {
			return nil, nil
		}

		copy(((*[1 << (maxArraySize - 13)]byte)(unsafe.Pointer(b.data)))[:len(bytes):len(bytes)], bytes)
		return nil, nil
	})

	return err
}

func (v *Value) GetByteLength(ctx context.Context) (int, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		bytes := C.v8_Value_ByteLength(v.context.pointer, v.pointer)
		i := int(bytes)
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int)), nil
	}
}

func (v *Value) Int64(ctx context.Context) (int64, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		i := int64(C.v8_Value_Int64(v.context.pointer, v.pointer))
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int64)), nil
	}
}

func (v *Value) Float64(ctx context.Context) (float64, error) {
	pf, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		f := float64(C.v8_Value_Float64(v.context.pointer, v.pointer))
		return &f, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pf.(*float64)), nil
	}
}

func (v *Value) Bool(ctx context.Context) (bool, error) {
	pb, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := C.v8_Value_Bool(v.context.pointer, v.pointer) == 1
		return &b, nil
	})

	if err != nil {
		return false, err
	} else {
		return *(pb.(*bool)), nil
	}
}

func (v *Value) Date(ctx context.Context) (time.Time, error) {
	pt, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if !v.IsKind(KindDate) {
			return time.Time{}, errors.New("not a date")
		} else if ms, err := v.Int64(ctx); err != nil {
			return time.Time{}, nil
		} else {
			s := ms / 1000
			ns := (ms % 1000) * 1e6
			return time.Unix(s, ns), nil
		}
	})

	if err != nil {
		return time.Time{}, err
	} else {
		return pt.(time.Time), nil
	}
}

func (v *Value) Equals(ctx context.Context, other *Value) (bool, error) {
	pb, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := false
		if v.GetContext() != other.GetContext() {
			return &b, nil
		} else {
			cb := C.v8_Value_Equals(v.context.pointer, v.pointer, other.pointer)
			pb := &cb
			return pb, nil
		}
	})

	if err != nil {
		return false, err
	} else {
		return *(pb.(*bool)), nil
	}
}

func (v *Value) StrictEquals(ctx context.Context, other *Value) (bool, error) {
	pb, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := false
		if v.GetContext() != other.GetContext() {
			return &b, nil
		} else {
			cb := C.v8_Value_StrictEquals(v.context.pointer, v.pointer, other.pointer)
			pb := bool(cb)
			return &pb, nil
		}
	})

	if err != nil {
		return false, err
	} else {
		return *(pb.(*bool)), nil
	}
}

func (v *Value) PromiseInfo(ctx context.Context) (PromiseState, *Value, error) {
	var state C.int

	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if !v.IsKind(KindPromise) {
			return nil, errors.New("not a promise")
		}

		return v.context.newValueFromTuple(ctx, C.v8_Value_PromiseInfo(v.context.pointer, v.pointer, &state))
	})

	if err != nil {
		return 0, nil, err
	} else {
		return PromiseState(state), pv.(*Value), nil
	}
}

func (v *Value) Await(ctx context.Context) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if !v.IsKind(KindPromise) {
			return v, nil
		}

		resolved := make(chan func() (*Value, error))

		if then, err := v.Get(ctx, "then"); err != nil {
			return nil, err
		} else if resolve, err := v.context.Create(ctx, func(in FunctionArgs) (*Value, error) {
			resolved <- func() (*Value, error) {
				return in.Arg(in.ExecutionContext, 0), nil
			}
			close(resolved)
			return nil, nil
		}); err != nil {
			return nil, err
		} else if reject, err := v.context.Create(ctx, func(in FunctionArgs) (*Value, error) {
			resolved <- func() (*Value, error) {
				errValue := in.Arg(in.ExecutionContext, 0)
				if stackValue, err := errValue.Get(in.ExecutionContext, "stack"); err == nil {
					if stack, err := stackValue.String(in.ExecutionContext); err != nil {
						return nil, err
					} else {
						return nil, fmt.Errorf(stack)
					}
				} else if message, err := errValue.String(in.ExecutionContext); err != nil {
					return nil, err
				} else {
					return nil, fmt.Errorf(message)
				}
			}
			close(resolved)
			return nil, nil
		}); err != nil {
			return nil, err
		} else {
			go then.Call(ctx, v, resolve, reject)
			return (<-resolved)()
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) String(ctx context.Context) (string, error) {
	s, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		ps := C.v8_Value_String(v.context.pointer, v.pointer)
		defer C.free(unsafe.Pointer(ps.data))

		s := C.GoStringN(ps.data, ps.length)
		return s, nil
	})

	if err != nil {
		return "", err
	} else {
		return s.(string), nil
	}

}

func (v *Value) MarshalJSON(ctx context.Context) ([]byte, error) {
	b, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if s, err := v.context.newValueFromTuple(ctx, C.v8_JSON_Stringify(v.context.pointer, v.pointer)); err != nil {
			return nil, err
		} else if string, err := s.String(ctx); err != nil {
			return nil, err
		} else {
			return []byte(string), nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return b.([]byte), nil
	}
}

func (v *Value) receiver(ctx context.Context) (*valueRef, error) {
	var intfield uint64

	if vref, err := v.context.receiverTable.CallMethod(ctx, "get", v); err != nil {
		return nil, err
	} else if vintfield, err := vref.Int64(ctx); err != nil {
		return nil, err
	} else {
		intfield = uint64(vintfield)
	}

	idn := refutils.ID(intfield)
	if idn == 0 {
		return nil, nil
	}

	ref := v.context.values.Get(idn)
	if ref == nil {
		return nil, nil
	}

	if vref, ok := ref.(*valueRef); !ok {
		return nil, nil
	} else {
		return vref, nil
	}
}

func (v *Value) Constructor(t reflect.Type) *FunctionTemplate {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if ft, ok := v.context.constructors[t]; ok {
		return ft
	} else if ft, ok := v.context.prototypes[t]; ok {
		return ft
	} else {
		return nil
	}
}

func (v *Value) Receiver(ctx context.Context, t reflect.Type) (*reflect.Value, error) {
	var r reflect.Value
	if vref, err := v.receiver(ctx); err != nil || vref == nil {
		// attempt to create a receiver
		if constructorTemplate := v.Constructor(t); constructorTemplate == nil {
			return nil, nil
		} else if constructor, err := constructorTemplate.GetFunction(ctx); err != nil {
			return nil, err
		} else if _, err := constructor.Call(ctx, v); err != nil {
			return nil, err
		} else if vref, err := v.receiver(ctx); err != nil {
			return nil, err
		} else if vref == nil {
			return nil, fmt.Errorf("no receiver found for %s and no constructor produces a receiver for %s (%p)", t, t.Elem(), v)
		} else {
			r = vref.value
		}
	} else {
		r = vref.value
	}

	if (t.Kind() == reflect.Interface || t.Kind() == reflect.Pointer) && r.Type().ConvertibleTo(t) {
		r = r.Convert(t)
		return &r, nil
	}

	ptr := t.Kind() == reflect.Ptr

	rt := r.Type()
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// if rt != t {
	// 	// TODO: should this return an error?
	// 	return nil, nil
	// }

	if ptr && r.Kind() != reflect.Ptr {
		r = r.Addr()
	} else if !ptr && r.Kind() == reflect.Ptr {
		r = r.Elem()
	}

	return &r, nil
}

func (v *Value) SetReceiver(ctx context.Context, value *reflect.Value) error {

	// if n, err := v.GetInternalFieldCount(ctx); err != nil || n == 0 {
	// 	return fmt.Errorf("error getting internal field")
	// }

	if vref, err := v.receiver(ctx); err == nil && vref != nil {
		v.context.values.Release(vref)
	}

	if value == nil {
		if _, err := v.context.receiverTable.CallMethod(ctx, "delete", v); err != nil {
			return err
		} else {
			return nil
		}
	}

	if value.Kind() != reflect.Pointer && value.Kind() != reflect.Interface {
		v := value.Addr()
		value = &v
	}

	id := v.context.values.Ref(&valueRef{value: *value})

	if vref, err := v.context.Create(ctx, uint64(id)); err != nil {
		log.Println(err)
		return err
	} else if _, err := v.context.receiverTable.CallMethod(ctx, "set", v, vref); err != nil {
		log.Println(err)
		return err
	} else {
		if value.CanAddr() {
			v.context.objects[uintptr(value.Addr().Pointer())] = v
		} else {
			v.context.objects[uintptr(value.Pointer())] = v
		}
		return nil
	}

	// return v.SetInternalField(ctx, 0, uint32(id))
}

// func (v *Value) AddFinalizer(finalizer func()) {
// 	v.finalizers = append(v.finalizers, finalizer)
// }

type weakCallbackInfo struct {
	object   interface{}
	callback func()
}

//export valueWeakCallbackHandler
func valueWeakCallbackHandler(pid C.String) {
	// 	ids := C.GoStringN(pid.data, pid.length)
	//
	// 	parts := strings.SplitN(ids, ":", 3)
	// 	isolateId, _ := strconv.Atoi(parts[0])
	// 	contextId, _ := strconv.Atoi(parts[1])
	//
	// 	isolateRef := isolates.Get(refutils.ID(isolateId))
	// 	if isolateRef == nil {
	// 		panic(fmt.Errorf("missing isolate pointer during weak callback for isolate #%d", isolateId))
	// 	}
	// 	isolate := isolateRef.(*Isolate)
	//
	// 	contextRef := isolate.contexts.Get(refutils.ID(contextId))
	// 	if contextRef == nil {
	// 		panic(fmt.Errorf("missing context pointer during weak callback for context #%d", contextId))
	// 	}
	// 	context := contextRef.(*Context)
	//
	// 	context.weakCallbackMutex.Lock()
	// 	if info, ok := context.weakCallbacks[ids]; !ok {
	// 		panic(fmt.Errorf("missing callback pointer during weak callback"))
	// 	} else {
	// 		context.weakCallbackMutex.Unlock()
	// 		info.callback()
	// 		delete(context.weakCallbacks, ids)
	// 	}
	// }
	//
	// func (v *Value) setWeak(id string, callback func()) error {
	// 	if err := v.context.isolate.lock(); err != nil {
	// 		return err
	// 	} else {
	// 		defer v.context.isolate.unlock()
	// 	}
	//
	// 	pid := C.CString(id)
	// 	defer C.free(unsafe.Pointer(pid))
	//
	// 	v.context.weakCallbackMutex.Lock()
	// 	v.context.weakCallbacks[id] = &weakCallbackInfo{v, callback}
	// 	v.context.weakCallbackMutex.Unlock()
	// 	C.v8_Value_SetWeak(v.context.pointer, v.pointer, pid)
	// 	return nil
}

func (v *Value) releaseWithContext(ctx context.Context) {
	v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		tracer.Remove(v)
		runtime.SetFinalizer(v, nil)

		if v.context.pointer != nil {
			C.v8_Value_Release(v.context.pointer, v.pointer)
		}
		v.context = nil
		v.pointer = nil

		return nil, nil
	})
}

func (v *Value) release() {
	ctx := WithContext(context.Background())
	v.releaseWithContext(ctx)
}
