package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fpic -std=c++11
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

type Context struct {
	id        ID
	isolate   *Isolate
	pointer   C.ContextPtr
	undefined *Value

	functions    *ReferenceMap
	accessors    *ReferenceMap
	values       *ReferenceMap
	constructors map[reflect.Type]*FunctionTemplate
}

func (i *Isolate) NewContext() *Context {
	context := &Context{
		isolate:      i,
		pointer:      C.v8_Context_New(i.pointer),
		functions:    NewReferenceMap(),
		accessors:    NewReferenceMap(),
		values:       NewReferenceMap(),
		constructors: map[reflect.Type]*FunctionTemplate{},
	}
	runtime.SetFinalizer(context, (*Context).release)
	return context
}

func (c *Context) GetIsolate() *Isolate {
	return c.isolate
}

func (c *Context) GetID() ID {
	return c.id
}

func (c *Context) SetID(id ID) {
	c.id = id
}

func (c *Context) ref() ID {
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
		c.undefined, _ = c.Create(nil)
	}
	return c.undefined
}

func (c *Context) Global() *Value {
	return c.newValue(C.v8_Context_Global(c.pointer), C.Kinds(KindObject))
}

func (c *Context) ParseJSON(json string) (*Value, error) {
	if j, err := c.Global().Get("JSON"); err != nil {
		return nil, fmt.Errorf("cannot get JSON: %+v", err)
	} else if jp, err := j.Get("parse"); err != nil {
		return nil, fmt.Errorf("cannot get JSON.parse: %+v", err)
	} else if s, err := c.Create(json); err != nil {
		return nil, err
	} else {
		return jp.Call(nil, s)
	}
}

func (c *Context) release() {
	if c.pointer != nil {
		C.v8_Context_Release(c.pointer)
	}
	c.pointer = nil

	runtime.SetFinalizer(c, nil)
	c.isolate = nil
}
