package isolates

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"context"
	"errors"
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
	Enumerable   bool
	Configurable bool
}

func (c *Context) newValueFromTuple(ctx context.Context, vt C.ValueTuple) (*Value, error) {
	return c.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := c.isolate.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer c.isolate.unlock(ctx)
		}

		return c.newValue(vt.value, vt.kinds), c.isolate.newError(vt.error)
	})
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
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	pk := C.CString(key)
	err := C.v8_Value_DefineProperty(v.context.pointer, v.pointer, pk, descriptor.Get.pointer, descriptor.Set.pointer, C.bool(descriptor.Configurable), C.bool(descriptor.Enumerable))
	C.free(unsafe.Pointer(pk))
	return v.context.isolate.newError(err)
}

func (v *Value) Get(ctx context.Context, key string) (*Value, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	pk := C.CString(key)
	vt := C.v8_Value_Get(v.context.pointer, v.pointer, pk)
	C.free(unsafe.Pointer(pk))
	return v.context.newValueFromTuple(ctx, vt)
}

func (v *Value) GetIndex(ctx context.Context, i int) (*Value, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	return v.context.newValueFromTuple(ctx, C.v8_Value_GetIndex(v.context.pointer, v.pointer, C.int(i)))
}

func (v *Value) Set(ctx context.Context, key string, value *Value) error {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	pk := C.CString(key)
	err := C.v8_Value_Set(v.context.pointer, v.pointer, pk, value.pointer)
	C.free(unsafe.Pointer(pk))
	return v.context.isolate.newError(err)
}

func (v *Value) SetIndex(ctx context.Context, i int, value *Value) error {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	return v.context.isolate.newError(C.v8_Value_SetIndex(v.context.pointer, v.pointer, C.int(i), value.pointer))
}

func (v *Value) SetInternalField(ctx context.Context, i int, value uint32) error {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	v.context.ref()
	defer v.context.unref()

	C.v8_Object_SetInternalField(v.context.pointer, v.pointer, C.int(i), C.uint32_t(value))
	return nil
}

func (v *Value) GetInternalField(ctx context.Context, i int) (int64, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return 0, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	v.context.ref()
	defer v.context.unref()

	return int64(C.v8_Object_GetInternalField(v.context.pointer, v.pointer, C.int(i))), nil
}

func (v *Value) GetInternalFieldCount(ctx context.Context) (int, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return 0, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	v.context.ref()
	defer v.context.unref()
	return int(C.v8_Object_GetInternalFieldCount(v.context.pointer, v.pointer)), nil
}

func (v *Value) Bind(ctx context.Context, argv ...*Value) (*Value, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	if bind, err := v.Get(ctx, "bind"); err != nil {
		return nil, err
	} else if fn, err := bind.Call(ctx, v, argv...); err != nil {
		return nil, err
	} else {
		return fn, nil
	}
}

func (v *Value) Call(ctx context.Context, self *Value, argv ...*Value) (*Value, error) {
	return v.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := v.context.isolate.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer v.context.isolate.unlock(ctx)
		}

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
}

func (v *Value) CallMethod(ctx context.Context, name string, argv ...*Value) (*Value, error) {
	return v.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := v.context.isolate.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer v.context.isolate.unlock(ctx)
		}

		if m, err := v.Get(ctx, name); err != nil {
			return nil, err
		} else if value, err := m.Call(ctx, v, argv...); err != nil {
			return nil, err
		} else {
			return value, nil
		}
	})

}

func (v *Value) New(ctx context.Context, argv ...*Value) (*Value, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	pargv := make([]C.ValuePtr, len(argv)+1)
	for i, argvi := range argv {
		pargv[i] = argvi.pointer
	}
	v.context.ref()
	vt := C.v8_Value_New(v.context.pointer, v.pointer, C.int(len(argv)), &pargv[0])
	v.context.unref()
	return v.context.newValueFromTuple(ctx, vt)
}

func (v *Value) Bytes(ctx context.Context) ([]byte, error) {

	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	b := C.v8_Value_Bytes(v.context.pointer, v.pointer)
	if b.data == nil {
		return nil, nil
	}

	buf := C.GoBytes(unsafe.Pointer(b.data), b.length)

	return buf, nil
}

func (v *Value) SetBytes(ctx context.Context, bytes []byte) error {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	b := C.v8_Value_Bytes(v.context.pointer, v.pointer)
	if b.data == nil {
		return nil
	}

	copy(((*[1 << (maxArraySize - 13)]byte)(unsafe.Pointer(b.data)))[:len(bytes):len(bytes)], bytes)
	return nil
}

func (v *Value) GetByteLength(ctx context.Context) (int, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return 0, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	bytes := C.v8_Value_ByteLength(v.context.pointer, v.pointer)

	return int(bytes), nil
}

func (v *Value) Int64(ctx context.Context) (int64, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return 0, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	return int64(C.v8_Value_Int64(v.context.pointer, v.pointer)), nil
}

func (v *Value) Float64(ctx context.Context) (float64, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return 0, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	return float64(C.v8_Value_Float64(v.context.pointer, v.pointer)), nil
}

func (v *Value) Bool(ctx context.Context) (bool, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return false, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}
	return C.v8_Value_Bool(v.context.pointer, v.pointer) == 1, nil
}

func (v *Value) Date(ctx context.Context) (time.Time, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return time.Time{}, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	if !v.IsKind(KindDate) {
		return time.Time{}, errors.New("not a date")
	} else if ms, err := v.Int64(ctx); err != nil {
		return time.Time{}, nil
	} else {
		s := ms / 1000
		ns := (ms % 1000) * 1e6
		return time.Unix(s, ns), nil
	}
}

func (v *Value) PromiseInfo(ctx context.Context) (PromiseState, *Value, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return 0, nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	if !v.IsKind(KindPromise) {
		return 0, nil, errors.New("not a promise")
	}
	var state C.int
	p, err := v.context.newValueFromTuple(ctx, C.v8_Value_PromiseInfo(v.context.pointer, v.pointer, &state))
	return PromiseState(state), p, err
}

func (v *Value) String(ctx context.Context) (string, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return "", err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	ps := C.v8_Value_String(v.context.pointer, v.pointer)
	defer C.free(unsafe.Pointer(ps.data))

	s := C.GoStringN(ps.data, ps.length)
	return s, nil
}

func (v *Value) MarshalJSON(ctx context.Context) ([]byte, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	if s, err := v.context.newValueFromTuple(ctx, C.v8_JSON_Stringify(v.context.pointer, v.pointer)); err != nil {
		return nil, err
	} else if string, err := s.String(ctx); err != nil {
		return nil, err
	} else {
		return []byte(string), nil
	}
}

func (v *Value) receiver(ctx context.Context) (*valueRef, error) {
	if locked, err := v.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer v.context.isolate.unlock(ctx)
	}

	if n, err := v.GetInternalFieldCount(ctx); err != nil || n == 0 {
		return nil, err
	}

	intfield, err := v.GetInternalField(ctx, 0)

	if err != nil {
		return nil, err
	}
	idn := refutils.ID(intfield)
	if idn == 0 {
		return nil, err
	}

	ref := v.context.values.Get(idn)
	if ref == nil {
		return nil, err
	}

	if vref, ok := ref.(*valueRef); !ok {
		return nil, err
	} else {
		return vref, nil
	}
}

func (v *Value) Receiver(ctx context.Context, t reflect.Type) (*reflect.Value, error) {
	var r reflect.Value
	if vref, err := v.receiver(ctx); err != nil {
		return nil, err
	} else {
		r = vref.value
	}

	if t.Kind() == reflect.Interface && r.Type().ConvertibleTo(t) {
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

	if rt != t {
		// TODO: should this return an error?
		return nil, nil
	}

	if ptr && r.Kind() != reflect.Ptr {
		r = r.Addr()
	} else if !ptr && r.Kind() == reflect.Ptr {
		r = r.Elem()
	}

	return &r, nil
}

func (v *Value) SetReceiver(ctx context.Context, value *reflect.Value) error {
	if !v.IsKind(KindObject) {
		return nil
	}

	if n, err := v.GetInternalFieldCount(ctx); err != nil || n == 0 {
		return nil
	}

	if vref, err := v.receiver(ctx); err != nil {
		v.context.values.Release(vref)
	}

	if value == nil {
		v.SetInternalField(ctx, 0, 0)
		return nil
	}

	id := v.context.values.Ref(&valueRef{value: *value})
	// v.setID("r", id)
	return v.SetInternalField(ctx, 0, uint32(id))
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
	v.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		tracer.Remove(v)
		runtime.SetFinalizer(v, nil)

		if locked, _ := v.context.isolate.lock(ctx); locked {
			defer v.context.isolate.unlock(ctx)
		}

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
