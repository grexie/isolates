package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++14
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"fmt"
	"runtime"

	refutils "github.com/behrsin/go-refutils"
)

type Resolver struct {
	refutils.RefHolder

	context *Context
	pointer C.ResolverPtr
}

func (c *Context) NewResolver() (*Resolver, error) {
	if err := c.isolate.lock(); err != nil {
		return nil, err
	} else {
		defer c.isolate.unlock()
	}

	pr := C.v8_Promise_NewResolver(c.pointer)
	if pr == nil {
		return nil, fmt.Errorf("cannot create resolver for context")
	}
	r := &Resolver{
		context: c,
		pointer: pr,
	}
	runtime.SetFinalizer(r, (*Resolver).release)
	tracer.Add(r)
	return r, nil
}

func (r *Resolver) ResolveWithValue(v *Value) error {
	if err := r.context.isolate.lock(); err != nil {
		return err
	} else {
		defer r.context.isolate.unlock()
	}

	err := C.v8_Resolver_Resolve(r.context.pointer, r.pointer, v.pointer)
	return r.context.isolate.newError(err)
}

func (r *Resolver) Resolve(value interface{}) error {
	if err := r.context.isolate.lock(); err != nil {
		return err
	} else {
		defer r.context.isolate.unlock()
	}

	if v, err := r.context.Create(value); err != nil {
		return err
	} else {
		return r.ResolveWithValue(v)
	}
}

func (r *Resolver) RejectWithValue(v *Value) error {
	if err := r.context.isolate.lock(); err != nil {
		return err
	} else {
		defer r.context.isolate.unlock()
	}

	err := C.v8_Resolver_Reject(r.context.pointer, r.pointer, v.pointer)
	return r.context.isolate.newError(err)
}

func (r *Resolver) Reject(value interface{}) error {
	if err := r.context.isolate.lock(); err != nil {
		return err
	} else {
		defer r.context.isolate.unlock()
	}

	if v, err := r.context.Create(value); err != nil {
		return err
	} else {
		return r.RejectWithValue(v)
	}
}

func (r *Resolver) Promise() (*Value, error) {
	if err := r.context.isolate.lock(); err != nil {
		return nil, err
	} else {
		defer r.context.isolate.unlock()
	}

	pv := C.v8_Resolver_GetPromise(r.context.pointer, r.pointer)
	v := r.context.newValue(pv, unionKindPromise)
	return v, nil
}

func (r *Resolver) release() {
	tracer.Remove(r)
	runtime.SetFinalizer(r, nil)

	if err := r.context.isolate.lock(); err == nil {
		defer r.context.isolate.unlock()
	}

	if r.context.pointer != nil {
		C.v8_Resolver_Release(r.context.pointer, r.pointer)
	}
	r.context = nil
	r.pointer = nil
}
