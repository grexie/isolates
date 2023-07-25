package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"unsafe"

	refutils "github.com/grexie/refutils"
)

type CallerInfo struct {
	Name     string
	Filename string
	Line     int
	Column   int
}

type FunctionTemplate struct {
	refutils.RefHolder

	context   *Context
	pointer   C.FunctionTemplatePtr
	info      *functionInfo
	value     *Value
	instance  *ObjectTemplate
	prototype *ObjectTemplate
}

type ObjectTemplate struct {
	refutils.RefHolder

	context *Context
	pointer C.ObjectTemplatePtr

	accessors map[string]*accessorInfo
}

type Function func(FunctionArgs) (*Value, error)
type Getter func(GetterArgs) (*Value, error)
type Setter func(SetterArgs) error

type FunctionArgs struct {
	ExecutionContext context.Context
	Context          *Context
	This             *Value
	IsConstructCall  bool
	Args             []*Value
	Caller           CallerInfo
	Holder           *Value
}

func (c *FunctionArgs) WithArgs(args ...any) (FunctionArgs, error) {
	var err error
	fnargs := *c

	fnargs.Args = make([]*Value, len(args))
	for i := 0; i < len(args); i++ {
		if fnargs.Args[i], err = c.Context.Create(c.ExecutionContext, args[i]); err != nil {
			return fnargs, err
		}
	}

	return fnargs, nil
}

func (c *FunctionArgs) Background(callback func(in FunctionArgs)) {
	c.Context.isolate.Background(c.ExecutionContext, func(ctx context.Context) {
		in := *c
		in.ExecutionContext = ctx
		callback(in)
	})
}

func (c *FunctionArgs) Arg(ctx context.Context, n int) *Value {
	pv, _ := c.Context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if n < len(c.Args) && n >= 0 {
			return c.Args[n], nil
		}
		if undefined, err := c.Context.Undefined(ctx); err != nil {
			// a panic should be ok here as it will be recovered in CallbackHandler
			// unless FunctionArgs has been passed to a goroutine
			panic(err)
		} else {
			return undefined, nil
		}
	})

	return pv.(*Value)
}

type GetterArgs struct {
	ExecutionContext context.Context
	Context          *Context
	Caller           CallerInfo
	This             *Value
	Holder           *Value
	Key              string
}

type SetterArgs struct {
	ExecutionContext context.Context
	Context          *Context
	Caller           CallerInfo
	This             *Value
	Holder           *Value
	Key              string
	Value            *Value
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

func (c *Context) NewFunctionTemplate(ctx context.Context, cb Function) (*FunctionTemplate, error) {
	ft, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		iid := c.isolate.ref()
		defer c.isolate.unref()

		cid := c.ref()
		defer c.unref()

		ecid := For(ctx).ref()
		defer For(ctx).unref()

		info := &functionInfo{
			Function: cb,
		}
		id := c.functions.Ref(info)
		pid := C.CString(fmt.Sprintf("%d:%d:%d:%d", iid, cid, id, ecid))
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
	})

	if err != nil {
		return nil, err
	} else {
		return ft.(*FunctionTemplate), nil
	}
}

func (f *FunctionTemplate) Inherit(ctx context.Context, parent *FunctionTemplate) error {
	_, err := f.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		f.context.ref()
		defer f.context.unref()

		C.v8_FunctionTemplate_Inherit(f.context.pointer, f.pointer, parent.pointer)
		return nil, nil
	})
	return err
}

func (f *FunctionTemplate) SetName(ctx context.Context, name string) error {
	_, err := f.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pname := C.CString(name)
		defer C.free(unsafe.Pointer(pname))

		f.context.ref()
		defer f.context.unref()

		C.v8_FunctionTemplate_SetName(f.context.pointer, f.pointer, pname)
		return nil, nil
	})

	return err
}

func (f *FunctionTemplate) GetFunction(ctx context.Context) (*Value, error) {
	pv, err := f.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
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

	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), err
	}
}

func (f *FunctionTemplate) GetInstanceTemplate(ctx context.Context) (*ObjectTemplate, error) {
	ot, err := f.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if f.instance != nil {
			return f.instance, nil
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
		f.instance = ot
		return ot, nil
	})

	if err != nil {
		return nil, err
	} else {
		return ot.(*ObjectTemplate), nil
	}

}

func (f *FunctionTemplate) GetPrototypeTemplate(ctx context.Context) (*ObjectTemplate, error) {
	ot, err := f.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		if f.prototype != nil {
			return f.prototype, nil
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

		f.prototype = ot

		return ot, nil
	})

	if err != nil {
		return nil, err
	} else {
		return ot.(*ObjectTemplate), nil
	}
}

func (f *FunctionTemplate) release() {
	ctx := WithContext(context.Background())

	f.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		tracer.Remove(f)
		runtime.SetFinalizer(f, nil)
		f.info = nil
		f.value = nil

		if f.context.pointer != nil {
			C.v8_FunctionTemplate_Release(f.context.pointer, f.pointer)
		}

		f.context = nil
		f.pointer = nil
		return nil, nil
	})

}

func (o *ObjectTemplate) SetInternalFieldCount(ctx context.Context, count int) error {
	_, err := o.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		o.context.ref()
		defer o.context.unref()

		C.v8_ObjectTemplate_SetInternalFieldCount(o.context.pointer, o.pointer, C.int(count))
		return nil, nil
	})

	return err
}

func (o *ObjectTemplate) SetAccessor(ctx context.Context, name string, getter Getter, setter Setter) error {
	_, err := o.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if o.accessors == nil {
			o.accessors = map[string]*accessorInfo{}
		}

		iid := o.context.isolate.ref()
		defer o.context.isolate.unref()

		cid := o.context.ref()
		defer o.context.unref()

		ecid := For(ctx).ref()
		defer For(ctx).unref()

		accessor := &accessorInfo{
			Getter: getter,
			Setter: setter,
		}

		o.accessors[name] = accessor

		id := o.context.accessors.Ref(accessor)

		pid := C.CString(fmt.Sprintf("%d:%d:%d:%d", iid, cid, id, ecid))
		defer C.free(unsafe.Pointer(pid))

		pname := C.CString(name)
		defer C.free(unsafe.Pointer(pname))

		C.v8_ObjectTemplate_SetAccessor(o.context.pointer, o.pointer, pname, pid, setter != nil)
		return nil, nil
	})

	return err
}

func (o *ObjectTemplate) Copy(ctx context.Context, other *ObjectTemplate) error {
	_, err := o.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if o.accessors != nil {
			for name, accessor := range o.accessors {
				if err := other.SetAccessor(ctx, name, accessor.Getter, accessor.Setter); err != nil {
					return nil, err
				}
			}
		}

		return nil, nil
	})

	return err
}

func (o *ObjectTemplate) release() {
	ctx := WithContext(context.Background())

	o.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		tracer.Remove(o)
		runtime.SetFinalizer(o, nil)

		if o.context.pointer != nil {
			C.v8_ObjectTemplate_Release(o.context.pointer, o.pointer)
		}

		o.context = nil
		o.pointer = nil
		return nil, nil
	})
}
