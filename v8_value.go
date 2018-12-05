package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fpic -std=c++11
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

type Value struct {
	context *Context
	pointer C.ValuePtr
	kinds   kinds
}

type PropertyDescriptor struct {
	Get          *Value
	Set          *Value
	Enumerable   bool
	Configurable bool
}

func (c *Context) newValueFromTuple(vt C.ValueTuple) (*Value, error) {
	return c.newValue(vt.value, vt.kinds), c.isolate.newError(vt.error)
}

func (c *Context) newValue(pointer C.ValuePtr, k C.Kinds) *Value {
	if pointer == nil {
		return nil
	}

	v := &Value{c, pointer, kinds(k)}

	runtime.SetFinalizer(v, (*Value).release)
	return v
}

func (v *Value) IsKind(k Kind) bool {
	return v.kinds.Is(k)
}

func (v *Value) DefineProperty(key string, descriptor *PropertyDescriptor) error {
	pk := C.CString(key)
	err := C.v8_Value_DefineProperty(v.context.pointer, v.pointer, pk, descriptor.Get.pointer, descriptor.Set.pointer, C.bool(descriptor.Configurable), C.bool(descriptor.Enumerable))
	C.free(unsafe.Pointer(pk))
	return v.context.isolate.newError(err)
}

func (v *Value) Get(key string) (*Value, error) {
	pk := C.CString(key)
	vt := C.v8_Value_Get(v.context.pointer, v.pointer, pk)
	C.free(unsafe.Pointer(pk))
	return v.context.newValueFromTuple(vt)
}

func (v *Value) GetIndex(i int) (*Value, error) {
	return v.context.newValueFromTuple(C.v8_Value_GetIndex(v.context.pointer, v.pointer, C.int(i)))
}

func (v *Value) Set(key string, value *Value) error {
	pk := C.CString(key)
	err := C.v8_Value_Set(v.context.pointer, v.pointer, pk, value.pointer)
	C.free(unsafe.Pointer(pk))
	return v.context.isolate.newError(err)
}

func (v *Value) SetIndex(i int, value *Value) error {
	return v.context.isolate.newError(C.v8_Value_SetIndex(v.context.pointer, v.pointer, C.int(i), value.pointer))
}

func (v *Value) SetInternalField(i int, value *Value) {
	v.context.ref()
	defer v.context.unref()

	C.v8_Object_SetInternalField(v.context.pointer, v.pointer, C.int(i), value.pointer)
}

func (v *Value) GetInternalField(i int) (*Value, error) {
	v.context.ref()
	defer v.context.unref()

	return v.context.newValueFromTuple(C.v8_Object_GetInternalField(v.context.pointer, v.pointer, C.int(i)))
}

func (v *Value) Call(self *Value, argv ...*Value) (*Value, error) {
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
	return v.context.newValueFromTuple(vt)
}

func (v *Value) New(argv ...*Value) (*Value, error) {
	pargv := make([]C.ValuePtr, len(argv)+1)
	for i, argvi := range argv {
		pargv[i] = argvi.pointer
	}
	v.context.ref()
	vt := C.v8_Value_New(v.context.pointer, v.pointer, C.int(len(argv)), &pargv[0])
	v.context.unref()
	return v.context.newValueFromTuple(vt)
}

func (v *Value) Bytes() []byte {
	b := C.v8_Value_Bytes(v.context.pointer, v.pointer)
	if b.data == nil {
		return nil
	}
	buf := make([]byte, b.length)
	copy(buf, ((*[1 << (maxArraySize - 13)]byte)(unsafe.Pointer(b.data)))[:b.length:b.length])
	return buf
}

func (v *Value) Int64() int64 {
	return int64(C.v8_Value_Int64(v.context.pointer, v.pointer))
}

func (v *Value) Bool() bool {
	return C.v8_Value_Bool(v.context.pointer, v.pointer) == 1
}

func (v *Value) Date() (time.Time, error) {
	if !v.IsKind(KindDate) {
		return time.Time{}, errors.New("not a date")
	}

	ms := v.Int64()
	s := ms / 1000
	ns := (ms % 1000) * 1e6
	return time.Unix(s, ns), nil
}

func (v *Value) PromiseInfo() (PromiseState, *Value, error) {
	if !v.IsKind(KindPromise) {
		return 0, nil, errors.New("not a promise")
	}
	var state C.int
	p, err := v.context.newValueFromTuple(C.v8_Value_PromiseInfo(v.context.pointer, v.pointer, &state))
	return PromiseState(state), p, err
}

func (v *Value) String() string {
	ps := C.v8_Value_String(v.context.pointer, v.pointer)
	s := C.GoStringN(ps.data, ps.length)
	C.free(unsafe.Pointer(ps.data))
	return s
}

func (v *Value) MarshalJSON() ([]byte, error) {
	if j, err := v.context.Global().Get("JSON"); err != nil {
		return nil, fmt.Errorf("cannot get JSON: %+v", err)
	} else if js, err := j.Get("stringify"); err != nil {
		return nil, fmt.Errorf("cannot get JSON.stringify: %+v", err)
	} else if s, err := js.Call(nil, v); err != nil {
		return nil, fmt.Errorf("failed to stringify value: %+v", err)
	} else {
		return []byte(s.String()), nil
	}
}

func (v *Value) release() {
	if v.pointer != nil {
		C.v8_Value_Release(v.context.pointer, v.pointer)
	}
	v.context = nil
	v.pointer = nil
	runtime.SetFinalizer(v, nil)
}
