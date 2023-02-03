package isolates

// #include "v8_c_bridge.h"
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
	ExecutionContext context.Context
	Context          *Context
	Caller           CallerInfo
	This             *Value
	Holder           *Value
	IsConstructCall  bool
	Args             []*Value
}

func (c *FunctionArgs) Arg(ctx context.Context, n int) *Value {
	if n < len(c.Args) && n >= 0 {
		return c.Args[n]
	}
	if undefined, err := c.Context.Undefined(ctx); err != nil {
		// a panic should be ok here as it will be recovered in CallbackHandler
		// unless FunctionArgs has been passed to a goroutine
		panic(err)
	} else {
		return undefined
	}
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
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
	}

	iid := c.isolate.ref()
	defer c.isolate.unref()

	cid := c.ref()
	defer c.unref()

	ecid := FromContext(ctx).ref()
	defer FromContext(ctx).unref()

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
}

func (f *FunctionTemplate) Inherit(ctx context.Context, parent *FunctionTemplate) error {
	if locked, err := f.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer f.context.isolate.unlock(ctx)
	}

	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_Inherit(f.context.pointer, f.pointer, parent.pointer)
	return nil
}

func (f *FunctionTemplate) SetName(ctx context.Context, name string) error {
	if locked, err := f.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer f.context.isolate.unlock(ctx)
	}

	pname := C.CString(name)
	defer C.free(unsafe.Pointer(pname))

	f.context.ref()
	defer f.context.unref()

	C.v8_FunctionTemplate_SetName(f.context.pointer, f.pointer, pname)
	return nil
}

func (f *FunctionTemplate) GetFunction(ctx context.Context) (*Value, error) {
	if locked, err := f.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer f.context.isolate.unlock(ctx)
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

func (f *FunctionTemplate) GetInstanceTemplate(ctx context.Context) (*ObjectTemplate, error) {
	if locked, err := f.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer f.context.isolate.unlock(ctx)
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

func (f *FunctionTemplate) GetPrototypeTemplate(ctx context.Context) (*ObjectTemplate, error) {
	if locked, err := f.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer f.context.isolate.unlock(ctx)
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
	ctx := WithContext(context.Background())
	f.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		tracer.Remove(f)
		runtime.SetFinalizer(f, nil)
		f.info = nil
		f.value = nil

		if locked, _ := f.context.isolate.lock(ctx); locked {
			defer f.context.isolate.unlock(ctx)
		}

		if f.context.pointer != nil {
			C.v8_FunctionTemplate_Release(f.context.pointer, f.pointer)
		}

		f.context = nil
		f.pointer = nil
		return nil, nil
	})

}

func (o *ObjectTemplate) SetInternalFieldCount(ctx context.Context, count int) error {
	if locked, err := o.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer o.context.isolate.unlock(ctx)
	}

	o.context.ref()
	defer o.context.unref()

	C.v8_ObjectTemplate_SetInternalFieldCount(o.context.pointer, o.pointer, C.int(count))
	return nil
}

func (o *ObjectTemplate) SetAccessor(ctx context.Context, name string, getter Getter, setter Setter) error {
	if locked, err := o.context.isolate.lock(ctx); err != nil {
		return err
	} else if locked {
		defer o.context.isolate.unlock(ctx)
	}

	iid := o.context.isolate.ref()
	defer o.context.isolate.unref()

	cid := o.context.ref()
	defer o.context.unref()

	ecid := FromContext(ctx).ref()
	defer FromContext(ctx).unref()

	id := o.context.accessors.Ref(&accessorInfo{
		Getter: getter,
		Setter: setter,
	})

	pid := C.CString(fmt.Sprintf("%d:%d:%d:%d", iid, cid, id, ecid))
	defer C.free(unsafe.Pointer(pid))

	pname := C.CString(name)
	defer C.free(unsafe.Pointer(pname))

	C.v8_ObjectTemplate_SetAccessor(o.context.pointer, o.pointer, pname, pid, setter != nil)
	return nil
}

func (o *ObjectTemplate) release() {
	ctx := WithContext(context.Background())
	o.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		tracer.Remove(o)
		runtime.SetFinalizer(o, nil)

		if locked, _ := o.context.isolate.lock(ctx); locked {
			defer o.context.isolate.unlock(ctx)
		}

		if o.context.pointer != nil {
			C.v8_ObjectTemplate_Release(o.context.pointer, o.pointer)
		}

		o.context = nil
		o.pointer = nil
		return nil, nil
	})
}
