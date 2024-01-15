package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"runtime"
	"sync"
	"unsafe"

	refutils "github.com/grexie/refutils"
)

type Context struct {
	refutils.RefHolder

	isolate *Isolate
	pointer C.ContextPtr
	pinner  runtime.Pinner

	global                    *Value
	objectCreate              *Value
	assign                    *Value
	keys                      *Value
	getOwnPropertyDescriptors *Value
	getPrototypeOf            *Value
	undefined                 *Value
	null                      *Value
	vfalse                    *Value
	vtrue                     *Value
	errorConstructor          *Value

	values int

	functions *refutils.RefMap
	accessors *refutils.RefMap
	refs      *refutils.RefMap

	receivers      map[uintptr]*Value
	receiversMutex sync.Mutex

	baseConstructor   *FunctionTemplate
	constructors      map[reflect.Type]*FunctionTemplate
	prototypes        map[reflect.Type]*FunctionTemplate
	constructorsMutex sync.Mutex

	weakCallbacks     map[string]*weakCallbackInfo
	weakCallbackMutex sync.Mutex

	data sync.Map
}

func (i *Isolate) NewContext(ctx context.Context) (*Context, error) {
	c, err := i.Sync(ctx, func(ctx context.Context) (any, error) {
		context := &Context{
			isolate:       i,
			pointer:       C.v8_Context_New(i.pointer),
			functions:     refutils.NewRefMap("f"),
			accessors:     refutils.NewRefMap("a"),
			receivers:     map[uintptr]*Value{},
			refs:          refutils.NewRefMap("v"),
			constructors:  map[reflect.Type]*FunctionTemplate{},
			prototypes:    map[reflect.Type]*FunctionTemplate{},
			weakCallbacks: map[string]*weakCallbackInfo{},
		}

		For(ctx).SetContext(context)

		context.ref()
		runtime.SetFinalizer(context, (*Context).release)

		if global, err := context.Global(ctx); err != nil {
			return nil, err
		} else if _, err := context.Undefined(ctx); err != nil {
			return nil, err
		} else if _, err := context.Null(ctx); err != nil {
			return nil, err
		} else if _, err := context.False(ctx); err != nil {
			return nil, err
		} else if _, err := context.True(ctx); err != nil {
			return nil, err
		} else if Object, err := global.Get(ctx, "Object"); err != nil {
			return nil, err
		} else if context.objectCreate, err = Object.Get(ctx, "create"); err != nil {
			return nil, err
		} else if context.assign, err = Object.Get(ctx, "assign"); err != nil {
			return nil, err
		} else if context.keys, err = Object.Get(ctx, "keys"); err != nil {
			return nil, err
		} else if context.getOwnPropertyDescriptors, err = Object.Get(ctx, "getOwnPropertyDescriptors"); err != nil {
			return nil, err
		} else if context.getPrototypeOf, err = Object.Get(ctx, "getPrototypeOf"); err != nil {
			return nil, err
		} else if context.errorConstructor, err = global.Get(ctx, "Error"); err != nil {
			return nil, err
		}

		return context, nil
	})

	if err != nil {
		return nil, err
	} else {
		return c.(*Context), nil
	}
}

func (c *Context) Receivers() int {
	return len(c.receivers)
}

func (c *Context) Values() int {
	return c.values
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
	_, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		wrapper := func(in FunctionArgs) (*Value, error) {
			_, err := For(ctx).Sync(func(ctx context.Context) (any, error) {
				return nil, fn(in)
			})

			return nil, err
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

func (c *Context) Run(ctx context.Context, code string, filename string, module *Module) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		pcode := C.CString(code)
		pfilename := C.CString(filename)

		iid := c.isolate.ref()
		defer c.isolate.unref()

		mid := c.isolate.modules.Ref(module)
		pid := C.CString(fmt.Sprintf("%d:%d", iid, mid))

		c.ref()
		vt := C.v8_Context_Run(c.pointer, pcode, pfilename, pid)
		c.unref()

		C.free(unsafe.Pointer(pcode))
		C.free(unsafe.Pointer(pfilename))

		return c.newValueFromTuple(ctx, vt)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) Data(key any) (any, bool) {
	return c.data.Load(key)
}

func (c *Context) SetData(key any, value any) {
	c.data.Store(key, value)
}

func (c *Context) ErrorConstructor(ctx context.Context) (*Value, error) {
	return c.errorConstructor, nil
}

func (c *Context) Undefined(ctx context.Context) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		if c.undefined == nil {
			var err error
			if c.undefined, err = c.newValueFromTuple(ctx, C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tUNDEFINED})); err != nil {
				return nil, err
			}
		}

		return c.undefined, nil
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) Null(ctx context.Context) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		if c.null == nil {
			var err error
			if c.null, err = c.newValueFromTuple(ctx, C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tNULL})); err != nil {
				return nil, err
			}
		}

		return c.null, nil
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) ObjectCreate(ctx context.Context, args ...any) (*Value, error) {
	return c.objectCreate.Call(ctx, nil, args...)
}

func (c *Context) CreateAll(ctx context.Context, objects ...any) ([]*Value, error) {
	out := make([]*Value, len(objects))
	var err error

	for i, object := range objects {
		if out[i], err = c.Create(ctx, object); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func (c *Context) Assign(ctx context.Context, objects ...any) (*Value, error) {
	if result, err := c.assign.Call(ctx, nil, objects...); err != nil {
		return nil, err
	} else {
		return result, nil
	}
}

func (c *Context) AssignAll(ctx context.Context, objects ...any) (*Value, error) {
	if objects, err := c.CreateAll(ctx, objects...); err != nil {
		return nil, err
	} else {
		for _, object := range objects[1:] {
			for ; !object.IsNil(); object, _ = object.GetPrototype(ctx) {
				if o, _ := object.GetPrototype(ctx); o.IsNil() {
					break
				}

				if descriptors, err := object.GetOwnPropertyDescriptors(ctx); err != nil {
					return nil, err
				} else {
					for name, descriptor := range descriptors {
						if name == "constructor" {
							continue
						}

						if err := objects[0].DefineProperty(ctx, name, &descriptor); err != nil {
							return nil, err
						}
					}
				}
			}
		}

		return objects[0], nil
	}

}

func (c *Context) NewObject(ctx context.Context) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		return c.newValueFromTuple(ctx, C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tOBJECT}))
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) False(ctx context.Context) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if c.vfalse == nil {
			var err error
			if c.vfalse, err = c.newValueFromTuple(ctx, C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tBOOL, _bool: false})); err != nil {
				return nil, err
			}
		}

		return c.vfalse, nil
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), err
	}
}

func (c *Context) True(ctx context.Context) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if c.vtrue == nil {
			var err error
			if c.vtrue, err = c.newValueFromTuple(ctx, C.v8_Context_Create(c.pointer, C.ImmediateValue{_type: C.tBOOL, _bool: true})); err != nil {
				return nil, err
			}
		}

		return c.vtrue, nil
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) Global(ctx context.Context) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if c.global == nil {
			var err error
			if c.global, err = c.newValueFromTuple(ctx, C.v8_Context_Global(c.pointer)); err != nil {
				return nil, err
			}
		}

		return c.global, nil
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) ParseJSON(ctx context.Context, json string) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pjson := C.CString(json)
		defer C.free(unsafe.Pointer(pjson))
		return c.newValueFromTuple(ctx, C.v8_JSON_Parse(c.pointer, pjson))
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) release() {
	ctx := c.isolate.GetExecutionContext()

	c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		runtime.SetFinalizer(c, nil)

		log.Println("CONTEXT RELEASED")

		c.global = nil
		c.undefined = nil
		c.null = nil
		c.vfalse = nil
		c.vtrue = nil

		c.functions.ReleaseAll()
		c.accessors.ReleaseAll()
		c.refs.ReleaseAll()

		c.baseConstructor = nil
		c.constructors = nil

		c.weakCallbacks = nil

		if c.pointer != nil {
			C.v8_Context_Release(c.pointer)
			c.pointer = nil
		}

		c.isolate.contexts.Release(c)

		return nil, nil
	})
}
