package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

type Context struct {
	referenceObject

	isolate   *Isolate
	pointer   C.ContextPtr
	global    *Value
	undefined *Value
	null      *Value
	vfalse    *Value
	vtrue     *Value

	functionCache map[uintptr]*Value
	functions     *referenceMap
	accessors     *referenceMap
	values        *referenceMap
	refs          *referenceMap
	objects       map[uintptr]*Value

	baseConstructor *FunctionTemplate
	constructors    map[reflect.Type]*FunctionTemplate

	weakCallbacks     map[string]*weakCallbackInfo
	weakCallbackMutex sync.Mutex
}

func (i *Isolate) NewContext() *Context {
	context := &Context{
		isolate:       i,
		pointer:       C.v8_Context_New(i.pointer),
		functions:     newReferenceMap("f", reflect.TypeOf(&functionInfo{})),
		accessors:     newReferenceMap("a", reflect.TypeOf(&accessorInfo{})),
		values:        newReferenceMap("v", reflect.TypeOf(&valueRef{})),
		refs:          newReferenceMap("v", reflect.TypeOf(&Value{})),
		objects:       map[uintptr]*Value{},
		constructors:  map[reflect.Type]*FunctionTemplate{},
		weakCallbacks: map[string]*weakCallbackInfo{},
	}
	context.ref()
	runtime.SetFinalizer(context, (*Context).release)
	i.tracer.AddContext(context)
	return context
}

func (c *Context) GetIsolate() *Isolate {
	return c.isolate
}

func (c *Context) ref() id {
	return c.isolate.contexts.Ref(c)
}

func (c *Context) unref() {
	c.isolate.contexts.Unref(c)
}

func (c *Context) Run(code string, filename string) (*Value, error) {
	pcode := C.CString(code)
	pfilename := C.CString(filename)

	c.ref()
	vt := C.v8_Context_Run(c.pointer, pcode, pfilename)
	c.unref()

	C.free(unsafe.Pointer(pcode))
	C.free(unsafe.Pointer(pfilename))

	return c.newValueFromTuple(vt)
}

func (c *Context) Undefined() *Value {
	if c.undefined == nil {
		c.undefined = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tUNDEFINED}), C.Kinds(KindUndefined))
	}
	return c.undefined
}

func (c *Context) Null() *Value {
	if c.null == nil {
		c.null = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tOBJECT}), C.Kinds(KindNull))
	}
	return c.null
}

func (c *Context) False() *Value {
	if c.vfalse == nil {
		c.vfalse = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tBOOL, _bool: false}), C.Kinds(KindBoolean))
	}
	return c.vfalse
}

func (c *Context) True() *Value {
	if c.vtrue == nil {
		c.vtrue = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tBOOL, _bool: true}), C.Kinds(KindBoolean))
	}
	return c.vtrue
}

func (c *Context) Global() *Value {
	if c.global == nil {
		c.global = c.newValue(C.v8_Context_Global(c.pointer), C.Kinds(KindObject))
	}
	return c.global
}

func (c *Context) ParseJSON(json string) (*Value, error) {
	pjson := C.CString(json)
	defer C.free(unsafe.Pointer(pjson))
	return c.newValueFromTuple(C.v8_JSON_Parse(c.pointer, pjson))
}

func (c *Context) release() {
	c.isolate.tracer.RemoveContext(c)
	if c.pointer != nil {
		C.v8_Context_Release(c.pointer)
	}
	c.pointer = nil
	c.isolate.contexts.Release(c)
	runtime.SetFinalizer(c, nil)
	c.isolate = nil
}
