package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"fmt"
	"reflect"
	"runtime"

	refutils "github.com/grexie/refutils"
)

type Resolver struct {
	refutils.RefHolder

	context *Context
	pointer C.ResolverPtr
}

func (c *Context) NewResolver(ctx context.Context) (*Resolver, error) {
	r, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pr := C.v8_Promise_NewResolver(c.pointer)
		if pr == nil {
			return nil, fmt.Errorf("cannot create resolver for context")
		}
		r := &Resolver{
			context: c,
			pointer: pr,
		}
		runtime.SetFinalizer(r, (*Resolver).release)
		return r, nil
	})

	if err != nil {
		return nil, err
	} else {
		return r.(*Resolver), nil
	}
}

func (r *Resolver) ResolveWithValue(ctx context.Context, v *Value) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		err := C.v8_Resolver_Resolve(r.context.pointer, r.pointer, v.pointer)
		return nil, r.context.isolate.newError(err)
	})

	return err
}

func (r *Resolver) Resolve(ctx context.Context, value interface{}) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if v, err := r.context.create(ctx, reflect.ValueOf(value), nil, true); err != nil {
			return nil, err
		} else {
			err := C.v8_Resolver_Resolve(r.context.pointer, r.pointer, v.pointer)
			return nil, r.context.isolate.newError(err)
		}
	})

	return err
}

func (r *Resolver) RejectWithValue(ctx context.Context, v *Value) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		err := C.v8_Resolver_Reject(r.context.pointer, r.pointer, v.pointer)
		return nil, r.context.isolate.newError(err)
	})

	return err
}

func (r *Resolver) Reject(ctx context.Context, value interface{}) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if v, err := r.context.create(ctx, reflect.ValueOf(value), nil, true); err != nil {
			return nil, err
		} else {
			err := C.v8_Resolver_Reject(r.context.pointer, r.pointer, v.pointer)
			return nil, r.context.isolate.newError(err)
		}
	})

	return err
}

func (r *Resolver) Promise(ctx context.Context) (*Value, error) {
	pv, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pv := C.v8_Resolver_GetPromise(r.context.pointer, r.pointer)
		return r.context.newValueFromTuple(ctx, pv)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (r *Resolver) ToCallback(ctx context.Context, callback *Value) error {
	if callback.IsNil() || !callback.IsKind(KindFunction) {
		return fmt.Errorf("callback undefined or null")
	}

	For(ctx).Background(func(ctx context.Context) {
		if promise, err := r.Promise(ctx); err != nil {
			callback.Call(ctx, nil, err)
		} else {
			if v, err := promise.Await(ctx); err != nil {
				callback.Call(ctx, nil, err)
			} else if v.IsNil() {
				callback.Call(ctx, nil, nil)
			} else {
				callback.Call(ctx, nil, nil, v)
			}
		}
	})

	return nil
}

func (r *Resolver) release() {
	ctx := r.context.isolate.GetExecutionContext()

	r.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		runtime.SetFinalizer(r, nil)

		if r.context.pointer != nil {
			C.v8_Resolver_Release(r.context.pointer, r.pointer)
		}
		r.context = nil
		r.pointer = nil

		return nil, nil
	})
}
