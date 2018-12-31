package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"fmt"
	"log"
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
	return c.Context.Undefined()
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

func (c *Context) NewFunctionTemplate(cb Function) *FunctionTemplate {
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
	return f
}

func (f *FunctionTemplate) Inherit(parent *FunctionTemplate) {
	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_Inherit(f.context.pointer, f.pointer, parent.pointer)
}

func (f *FunctionTemplate) SetName(name string) {
	pname := C.CString(name)
	defer C.free(unsafe.Pointer(pname))

	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_SetName(f.context.pointer, f.pointer, pname)
}

func (f *FunctionTemplate) SetHiddenPrototype(value bool) {
	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_SetHiddenPrototype(f.context.pointer, f.pointer, C.bool(value))
}

func (f *FunctionTemplate) GetFunction() *Value {
	if f.value == nil {
		pv := C.v8_FunctionTemplate_GetFunction(f.context.pointer, f.pointer)
		f.value = f.context.newValue(pv, unionKindFunction)

		f.value.AddFinalizer(func(c *Context, i *functionInfo) func() {
			return func() {
				log.Println("WeakCallback:finalizer")
				c.functions.Release(i)
			}
		}(f.context, f.info))
	}

	return f.value
}

func (f *FunctionTemplate) GetInstanceTemplate() *ObjectTemplate {
	f.context.ref()
	defer f.context.unref()

	po := C.v8_FunctionTemplate_InstanceTemplate(f.context.pointer, f.pointer)
	ot := &ObjectTemplate{
		context: f.context,
		pointer: po,
	}
	runtime.SetFinalizer(ot, (*ObjectTemplate).release)
	tracer.Add(ot)
	return ot
}

func (f *FunctionTemplate) GetPrototypeTemplate() *ObjectTemplate {
	f.context.ref()
	defer f.context.unref()

	pp := C.v8_FunctionTemplate_PrototypeTemplate(f.context.pointer, f.pointer)
	ot := &ObjectTemplate{
		context: f.context,
		pointer: pp,
	}
	runtime.SetFinalizer(ot, (*ObjectTemplate).release)
	tracer.Add(ot)
	return ot
}

func (f *FunctionTemplate) release() {
	tracer.Remove(f)

	if f.pointer != nil {
		f.context.ref()
		C.v8_FunctionTemplate_Release(f.context.pointer, f.pointer)
		f.context.unref()
	}

	f.info = nil
	f.value = nil
	f.context = nil
	f.pointer = nil
	runtime.SetFinalizer(f, nil)
}

func (o *ObjectTemplate) SetInternalFieldCount(count int) {
	o.context.ref()
	defer o.context.unref()

	C.v8_ObjectTemplate_SetInternalFieldCount(o.context.pointer, o.pointer, C.int(count))
}

func (o *ObjectTemplate) SetAccessor(name string, getter Getter, setter Setter) {
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
}

func (o *ObjectTemplate) release() {
	tracer.Remove(o)

	if o.pointer != nil {
		o.context.ref()
		C.v8_ObjectTemplate_Release(o.context.pointer, o.pointer)
		o.context.unref()
	}

	o.context = nil
	o.pointer = nil
	runtime.SetFinalizer(o, nil)
}
