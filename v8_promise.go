package isolates

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
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
	if locked, err := c.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer c.isolate.unlock(ctx)
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

func (r *Resolver) ResolveWithValue(ctx context.Context, v *Value) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := r.context.isolate.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer r.context.isolate.unlock(ctx)
		}

		err := C.v8_Resolver_Resolve(r.context.pointer, r.pointer, v.pointer)
		return nil, r.context.isolate.newError(err)
	})
	return err
}

func (r *Resolver) Resolve(ctx context.Context, value interface{}) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if v, err := r.context.create(ctx, reflect.ValueOf(value)); err != nil {
			return nil, err
		} else {
			if locked, err := r.context.isolate.lock(ctx); err != nil {
				return nil, err
			} else if locked {
				defer r.context.isolate.unlock(ctx)
			}

			err := C.v8_Resolver_Resolve(r.context.pointer, r.pointer, v.pointer)
			return nil, r.context.isolate.newError(err)
		}
	})
	return err
}

func (r *Resolver) RejectWithValue(ctx context.Context, v *Value) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if locked, err := r.context.isolate.lock(ctx); err != nil {
			return nil, err
		} else if locked {
			defer r.context.isolate.unlock(ctx)
		}

		err := C.v8_Resolver_Reject(r.context.pointer, r.pointer, v.pointer)
		return nil, r.context.isolate.newError(err)
	})
	return err
}

func (r *Resolver) Reject(ctx context.Context, value interface{}) error {
	_, err := r.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		if v, err := r.context.create(ctx, reflect.ValueOf(value)); err != nil {
			return nil, err
		} else {
			if locked, err := r.context.isolate.lock(ctx); err != nil {
				return nil, err
			} else if locked {
				defer r.context.isolate.unlock(ctx)
			}

			err := C.v8_Resolver_Reject(r.context.pointer, r.pointer, v.pointer)
			return nil, r.context.isolate.newError(err)
		}
	})

	return err
}

func (r *Resolver) Promise(ctx context.Context) (*Value, error) {
	if locked, err := r.context.isolate.lock(ctx); err != nil {
		return nil, err
	} else if locked {
		defer r.context.isolate.unlock(ctx)
	}

	pv := C.v8_Resolver_GetPromise(r.context.pointer, r.pointer)
	v := r.context.newValue(pv, unionKindPromise)
	return v, nil
}

func (r *Resolver) release() {
	ctx := WithContext(context.Background())
	r.context.isolate.Sync(ctx, func(ctx context.Context) (*Value, error) {
		tracer.Remove(r)
		runtime.SetFinalizer(r, nil)

		if locked, err := r.context.isolate.lock(ctx); err != nil {
			return nil, nil
		} else if locked {
			defer r.context.isolate.unlock(ctx)
		}

		if r.context.pointer != nil {
			C.v8_Resolver_Release(r.context.pointer, r.pointer)
		}
		r.context = nil
		r.pointer = nil

		return nil, nil
	})
}
