package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	refutils "github.com/behrsin/go-refutils"
)

type Value struct {
	refutils.RefHolder

	context    *Context
	pointer    C.ValuePtr
	kinds      kinds
	finalizers []func()
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

	v := &Value{
		context:    c,
		pointer:    pointer,
		kinds:      kinds(k),
		finalizers: make([]func(), 0),
	}
	runtime.SetFinalizer(v, (*Value).release)

	tracer.Add(v)

	return v
}

func (v *Value) ref() refutils.ID {
	return v.context.refs.Ref(v)
}

func (v *Value) unref() {
	v.context.refs.Unref(v)
}

func (v *Value) IsKind(k Kind) bool {
	return v.kinds.Is(k)
}

func (v *Value) GetContext() *Context {
	return v.context
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

func (v *Value) SetInternalField(i int, value uint32) {
	v.context.ref()
	defer v.context.unref()

	C.v8_Object_SetInternalField(v.context.pointer, v.pointer, C.int(i), C.uint32_t(value))
}

func (v *Value) GetInternalField(i int) int64 {
	v.context.ref()
	defer v.context.unref()

	return int64(C.v8_Object_GetInternalField(v.context.pointer, v.pointer, C.int(i)))
}

func (v *Value) GetInternalFieldCount() int {
	v.context.ref()
	defer v.context.unref()
	return int(C.v8_Object_GetInternalFieldCount(v.context.pointer, v.pointer))
}

func (v *Value) Bind(argv ...*Value) (*Value, error) {
	if bind, err := v.Get("bind"); err != nil {
		return nil, err
	} else if fn, err := bind.Call(v, argv...); err != nil {
		return nil, err
	} else {
		return fn, nil
	}
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

func (v *Value) CallMethod(name string, argv ...*Value) (*Value, error) {
	if m, err := v.Get(name); err != nil {
		return nil, err
	} else if value, err := m.Call(v, argv...); err != nil {
		return nil, err
	} else {
		return value, nil
	}
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

func (v *Value) Float64() float64 {
	return float64(C.v8_Value_Float64(v.context.pointer, v.pointer))
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
	defer C.free(unsafe.Pointer(ps.data))

	s := C.GoStringN(ps.data, ps.length)
	return s
}

func (v *Value) tracerString() string {
	return v.String()
}

func (v *Value) MarshalJSON() ([]byte, error) {
	if s, err := v.context.newValueFromTuple(C.v8_JSON_Stringify(v.context.pointer, v.pointer)); err != nil {
		return nil, err
	} else {
		return []byte(s.String()), nil
	}
}

func (v *Value) receiver() *valueRef {
	if v.GetInternalFieldCount() == 0 {
		return nil
	}

	idn := refutils.ID(v.GetInternalField(0))
	if idn == 0 {
		return nil
	}

	ref := v.context.values.Get(idn)
	if ref == nil {
		return nil
	}

	if vref, ok := ref.(*valueRef); !ok {
		return nil
	} else {
		return vref
	}
}

func (v *Value) Receiver(t reflect.Type) *reflect.Value {
	var r reflect.Value
	if vref := v.receiver(); vref == nil {
		return nil
	} else {
		r = vref.value
	}

	if t.Kind() == reflect.Interface && r.Type().ConvertibleTo(t) {
		r = r.Convert(t)
		return &r
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
		return nil
	}

	if ptr && r.Kind() != reflect.Ptr {
		r = r.Addr()
	} else if !ptr && r.Kind() == reflect.Ptr {
		r = r.Elem()
	}

	return &r
}

func (v *Value) SetReceiver(value *reflect.Value) {
	if !v.IsKind(KindObject) {
		return
	}

	if v.GetInternalFieldCount() == 0 {
		return
	}

	if vref := v.receiver(); vref != nil {
		v.context.values.Release(vref)
	}

	if value == nil {
		v.SetInternalField(0, 0)
		return
	}

	id := v.context.values.Ref(&valueRef{value: *value})
	// v.setID("r", id)
	v.SetInternalField(0, uint32(id))
}

func (v *Value) AddFinalizer(finalizer func()) {
	v.finalizers = append(v.finalizers, finalizer)
}

type weakCallbackInfo struct {
	object   interface{}
	callback func()
}

//export ValueWeakCallbackHandler
func ValueWeakCallbackHandler(pid C.String) {
	ids := C.GoStringN(pid.data, pid.length)

	parts := strings.SplitN(ids, ":", 3)
	isolateId, _ := strconv.Atoi(parts[0])
	contextId, _ := strconv.Atoi(parts[1])

	isolateRef := isolates.Get(refutils.ID(isolateId))
	if isolateRef == nil {
		panic(fmt.Errorf("missing isolate pointer during weak callback for isolate #%d", isolateId))
	}
	isolate := isolateRef.(*Isolate)

	contextRef := isolate.contexts.Get(refutils.ID(contextId))
	if contextRef == nil {
		panic(fmt.Errorf("missing context pointer during weak callback for context #%d", contextId))
	}
	context := contextRef.(*Context)

	context.weakCallbackMutex.Lock()
	if info, ok := context.weakCallbacks[ids]; !ok {
		panic(fmt.Errorf("missing callback pointer during weak callback"))
	} else {
		context.weakCallbackMutex.Unlock()
		info.callback()
		delete(context.weakCallbacks, ids)
	}
}

func (v *Value) setWeak(id string, callback func()) {
	pid := C.CString(id)
	defer C.free(unsafe.Pointer(pid))

	v.context.weakCallbackMutex.Lock()
	v.context.weakCallbacks[id] = &weakCallbackInfo{v, callback}
	v.context.weakCallbackMutex.Unlock()
	C.v8_Value_SetWeak(v.context.pointer, v.pointer, pid)
}

func (v *Value) release() {
	// if false {
	// 	iid := v.context.isolate.ref()
	// 	defer v.context.isolate.unref()
	//
	// 	cid := v.context.ref()
	// 	defer v.context.unref()
	//
	// 	vid := v.ref()
	// 	defer v.unref()
	//
	// 	id := fmt.Sprintf("%d:%d:%d", iid, cid, vid)
	//
	// 	v.setWeak(id, func() {
	// 		for _, finalizer := range v.finalizers {
	// 			finalizer()
	// 		}
	// 		v.finalize()
	// 	})
	// } else {
	// 	v.finalize()
	// }
	v.finalize()
	runtime.SetFinalizer(v, nil)
}

func (v *Value) finalize() {
	tracer.Remove(v)
	if v.pointer != nil {
		// if id := v.getID("r"); id != 0 {
		// 	if vref := v.context.values.Get(id); vref != nil {
		// 		log.Println("releasing valueRef", id)
		// 		v.context.values.Release(vref)
		// 	}
		// }

		C.v8_Value_Release(v.context.pointer, v.pointer)
		v.context = nil
		v.pointer = nil
	}
}
