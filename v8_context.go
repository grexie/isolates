package isolates

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"context"
	"reflect"
	"runtime"
	"sync"
	"unsafe"

	refutils "github.com/grexie/refutils"
)

type Context struct {
	refutils.RefHolder

	isolate   *Isolate
	pointer   C.ContextPtr
	global    *Value
	undefined *Value
	null      *Value
	vfalse    *Value
	vtrue     *Value

	functions    *refutils.RefMap
	accessors    *refutils.RefMap
	values       *refutils.RefMap
	refs         *refutils.RefMap
	objects      map[uintptr]*Value
	objectsMutex sync.Mutex

	baseConstructor   *FunctionTemplate
	constructors      map[reflect.Type]*FunctionTemplate
	constructorsMutex sync.Mutex

	weakCallbacks     map[string]*weakCallbackInfo
	weakCallbackMutex sync.Mutex
}

func (i *Isolate) NewContext(ctx context.Context) (*Context, error) {
	if locked, err := i.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer i.unlock(ctx)
	}

	context := &Context{
		isolate:       i,
		pointer:       C.v8_Context_New(i.pointer),
		functions:     refutils.NewRefMap("f"),
		accessors:     refutils.NewRefMap("a"),
		values:        refutils.NewRefMap("v"),
		refs:          refutils.NewRefMap("v"),
		objects:       map[uintptr]*Value{},
		constructors:  map[reflect.Type]*FunctionTemplate{},
		weakCallbacks: map[string]*weakCallbackInfo{},
	}
	context.ref()
	runtime.SetFinalizer(context, (*Context).release)
	tracer.Add(context)
	tracer.AddRefMap("functionInfo", context.functions)
	tracer.AddRefMap("accessorInfo", context.accessors)
	tracer.AddRefMap("valueRef", context.values)
	tracer.AddRefMap("refs", context.refs)
	return context, nil
}

func (c *Context) GetIsolate() *Isolate {
	return c.isolate
}

func (c *Context) ref() refutils.ID {
	return c.isolate.contexts.Ref(c)
}

func (c *Context) unref() {
	c.isolate.contexts.Unref(c)
}

func (c *Context) AddMicrotask(ctx context.Context, fn func(in FunctionArgs) error) error {
	_, err := c.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		wrapper := func(in FunctionArgs) (*Value, error) {
			return nil, fn(in)
		}

		if value, err := c.Create(ctx, wrapper); err != nil {
			return nil, err
		} else if err := c.isolate.EnqueueMicrotaskWithValue(ctx, value); err != nil {
			return nil, err
		}

		return nil, nil
	})

	return err
}

func (c *Context) Run(ctx context.Context, code string, filename string) (*Value, error) {
	return c.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := c.isolate.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer c.isolate.unlock(ctx)
		}

		pcode := C.CString(code)
		pfilename := C.CString(filename)

		c.ref()
		vt := C.v8_Context_Run(c.pointer, pcode, pfilename)
		c.unref()

		C.free(unsafe.Pointer(pcode))
		C.free(unsafe.Pointer(pfilename))

		return c.newValueFromTuple(ctx, vt)
	})
}

func (c *Context) Undefined(ctx context.Context) (*Value, error) {
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
	}

	if c.undefined == nil {
		c.undefined = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tUNDEFINED}), C.Kinds(KindUndefined))
	}
	return c.undefined, nil
}

func (c *Context) Null(ctx context.Context) (*Value, error) {
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
	}

	if c.null == nil {
		c.null = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tOBJECT}), C.Kinds(KindNull))
	}
	return c.null, nil
}

func (c *Context) False(ctx context.Context) (*Value, error) {
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
	}

	if c.vfalse == nil {
		c.vfalse = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tBOOL, _bool: false}), C.Kinds(KindBoolean))
	}
	return c.vfalse, nil
}

func (c *Context) True(ctx context.Context) (*Value, error) {
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
	}

	if c.vtrue == nil {
		c.vtrue = c.newValue(C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tBOOL, _bool: true}), C.Kinds(KindBoolean))
	}
	return c.vtrue, nil
}

func (c *Context) Global(ctx context.Context) (*Value, error) {
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
	}

	if c.global == nil {
		c.global = c.newValue(C.v8_Context_Global(c.pointer), C.Kinds(KindObject))
	}
	return c.global, nil
}

func (c *Context) ParseJSON(ctx context.Context, json string) (*Value, error) {
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
	}

	pjson := C.CString(json)
	defer C.free(unsafe.Pointer(pjson))
	return c.newValueFromTuple(ctx, C.v8_JSON_Parse(c.pointer, pjson))
}

func (c *Context) release() {
	ctx := WithContext(context.Background())
	c.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		runtime.SetFinalizer(c, nil)

		c.global = nil
		c.undefined = nil
		c.null = nil
		c.vfalse = nil
		c.vtrue = nil

		c.functions.ReleaseAll()
		c.accessors.ReleaseAll()
		c.values.ReleaseAll()
		c.refs.ReleaseAll()
		c.objects = nil

		c.baseConstructor = nil
		c.constructors = nil

		c.weakCallbacks = nil

		tracer.RemoveRefMap("functionInfo", c.functions)
		tracer.RemoveRefMap("accessorInfo", c.accessors)
		tracer.RemoveRefMap("valueRef", c.values)
		tracer.RemoveRefMap("refs", c.refs)
		tracer.Remove(c)

		if locked, _ := c.isolate.lock(ctx); locked {
			defer c.isolate.unlock(ctx)
		}

		if c.pointer != nil {
			C.v8_Context_Release(c.pointer)
			c.pointer = nil
		}

		c.isolate.contexts.Release(c)

		return nil, nil
	})
}
