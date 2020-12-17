package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++14
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"

	refutils "github.com/behrsin/go-refutils"
)

type CallerInfo struct {
	Name     string
	Filename string
	Line     int
	Column   int
}

type FunctionTemplate struct {
	refutils.RefHolder

	context *Context
	pointer C.FunctionTemplatePtr
	info    *functionInfo
	value   *Value
}

type ObjectTemplate struct {
	refutils.RefHolder

	context *Context
	pointer C.ObjectTemplatePtr
}

type Function func(FunctionArgs) (*Value, error)
type Getter func(GetterArgs) (*Value, error)
type Setter func(SetterArgs) error

type FunctionArgs struct {
	Context         *Context
	Caller          CallerInfo
	This            *Value
	Holder          *Value
	IsConstructCall bool
	Args            []*Value
}

func (c *FunctionArgs) Arg(n int) *Value {
	if n < len(c.Args) && n >= 0 {
		return c.Args[n]
	}
	if undefined, err := c.Context.Undefined(); err != nil {
		// a panic should be ok here as it will be recovered in CallbackHandler
		// unless FunctionArgs has been passed to a goroutine
		panic(err)
	} else {
		return undefined
	}
}

type GetterArgs struct {
	Context *Context
	Caller  CallerInfo
	This    *Value
	Holder  *Value
	Key     string
}

type SetterArgs struct {
	Context *Context
	Caller  CallerInfo
	This    *Value
	Holder  *Value
	Key     string
	Value   *Value
}

type functionInfo struct {
	refutils.RefHolder

	Function
}

func (fi *functionInfo) String() string {
	name := runtime.FuncForPC(reflect.ValueOf(fi.Function).Pointer()).Name()
	return fmt.Sprintf("function {%p %s}", fi.Function, name)
}

type accessorInfo struct {
	refutils.RefHolder

	Getter
	Setter
}

func (c *Context) NewFunctionTemplate(cb Function) (*FunctionTemplate, error) {
	if err := c.isolate.lock(); err != nil {
		return nil, err
	} else {
		defer c.isolate.unlock()
	}

	iid := c.isolate.ref()
	defer c.isolate.unref()

	cid := c.ref()
	defer c.unref()

	info := &functionInfo{
		Function: cb,
	}
	id := c.functions.Ref(info)
	pid := C.CString(fmt.Sprintf("%d:%d:%d", iid, cid, id))
	defer C.free(unsafe.Pointer(pid))

	pf := C.v8_FunctionTemplate_New(c.pointer, pid)

	f := &FunctionTemplate{
		context: c,
		pointer: pf,
		info:    info,
	}
	runtime.SetFinalizer(f, (*FunctionTemplate).release)
	tracer.Add(f)
	return f, nil
}

func (f *FunctionTemplate) Inherit(parent *FunctionTemplate) error {
	if err := f.context.isolate.lock(); err != nil {
		return err
	} else {
		defer f.context.isolate.unlock()
	}

	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_Inherit(f.context.pointer, f.pointer, parent.pointer)
	return nil
}

func (f *FunctionTemplate) SetName(name string) error {
	if err := f.context.isolate.lock(); err != nil {
		return err
	} else {
		defer f.context.isolate.unlock()
	}

	pname := C.CString(name)
	defer C.free(unsafe.Pointer(pname))

	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_SetName(f.context.pointer, f.pointer, pname)
	return nil
}

func (f *FunctionTemplate) SetHiddenPrototype(value bool) error {
	if err := f.context.isolate.lock(); err != nil {
		return err
	} else {
		defer f.context.isolate.unlock()
	}

	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_SetHiddenPrototype(f.context.pointer, f.pointer, C.bool(value))
	return nil
}

func (f *FunctionTemplate) GetFunction() (*Value, error) {
	if err := f.context.isolate.lock(); err != nil {
		return nil, err
	} else {
		defer f.context.isolate.unlock()
	}

	if f.value == nil {
		pv := C.v8_FunctionTemplate_GetFunction(f.context.pointer, f.pointer)
		f.value = f.context.newValue(pv, unionKindFunction)

		// f.value.AddFinalizer(func(c *Context, i *functionInfo) func() {
		// 	return func() {
		// 		log.Println("WeakCallback:finalizer")
		// 		c.functions.Release(i)
		// 	}
		// }(f.context, f.info))
	}

	return f.value, nil
}

func (f *FunctionTemplate) GetInstanceTemplate() (*ObjectTemplate, error) {
	if err := f.context.isolate.lock(); err != nil {
		return nil, err
	} else {
		defer f.context.isolate.unlock()
	}

	f.context.ref()
	defer f.context.unref()

	po := C.v8_FunctionTemplate_InstanceTemplate(f.context.pointer, f.pointer)
	ot := &ObjectTemplate{
		context: f.context,
		pointer: po,
	}
	runtime.SetFinalizer(ot, (*ObjectTemplate).release)
	tracer.Add(ot)
	return ot, nil
}

func (f *FunctionTemplate) GetPrototypeTemplate() (*ObjectTemplate, error) {
	if err := f.context.isolate.lock(); err != nil {
		return nil, err
	} else {
		defer f.context.isolate.unlock()
	}

	f.context.ref()
	defer f.context.unref()

	pp := C.v8_FunctionTemplate_PrototypeTemplate(f.context.pointer, f.pointer)
	ot := &ObjectTemplate{
		context: f.context,
		pointer: pp,
	}
	runtime.SetFinalizer(ot, (*ObjectTemplate).release)
	tracer.Add(ot)
	return ot, nil
}

func (f *FunctionTemplate) release() {
	tracer.Remove(f)
	runtime.SetFinalizer(f, nil)
	f.info = nil
	f.value = nil

	if err := f.context.isolate.lock(); err == nil {
		defer f.context.isolate.unlock()
	}

	if f.context.pointer != nil {
		C.v8_FunctionTemplate_Release(f.context.pointer, f.pointer)
	}

	f.context = nil
	f.pointer = nil

}

func (o *ObjectTemplate) SetInternalFieldCount(count int) error {
	if err := o.context.isolate.lock(); err != nil {
		return err
	} else {
		defer o.context.isolate.unlock()
	}

	o.context.ref()
	defer o.context.unref()

	C.v8_ObjectTemplate_SetInternalFieldCount(o.context.pointer, o.pointer, C.int(count))
	return nil
}

func (o *ObjectTemplate) SetAccessor(name string, getter Getter, setter Setter) error {
	if err := o.context.isolate.lock(); err != nil {
		return err
	} else {
		defer o.context.isolate.unlock()
	}

	iid := o.context.isolate.ref()
	defer o.context.isolate.unref()

	cid := o.context.ref()
	defer o.context.unref()

	id := o.context.accessors.Ref(&accessorInfo{
		Getter: getter,
		Setter: setter,
	})
	pid := C.CString(fmt.Sprintf("%d:%d:%d", iid, cid, id))
	defer C.free(unsafe.Pointer(pid))

	pname := C.CString(name)
	defer C.free(unsafe.Pointer(pname))

	C.v8_ObjectTemplate_SetAccessor(o.context.pointer, o.pointer, pname, pid, setter != nil)
	return nil
}

func (o *ObjectTemplate) release() {
	tracer.Remove(o)
	runtime.SetFinalizer(o, nil)

	if err := o.context.isolate.lock(); err == nil {
		defer o.context.isolate.unlock()
	}

	if o.context.pointer != nil {
		C.v8_ObjectTemplate_Release(o.context.pointer, o.pointer)
	}

	o.context = nil
	o.pointer = nil

}
