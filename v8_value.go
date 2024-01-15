package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"time"
	"unsafe"

	refutils "github.com/grexie/refutils"
)

type Value struct {
	refutils.RefHolder

	context  *Context
	pointer  C.ValuePtr
	kinds    kinds
	info     C.ValueTuplePtr
	receiver reflect.Value

	refCount int
}

type Error interface {
	error
	Message() string
}

type PropertyDescriptor struct {
	Get          *Value `v8:"get"`
	Set          *Value `v8:"set"`
	Value        *Value `v8:"value"`
	Enumerable   bool   `v8:"enumerable"`
	Configurable bool   `v8:"configurable"`
	Writable     bool   `v8:"writable"`
}

func (d *PropertyDescriptor) MarshalV8(ctx context.Context) any {
	out := map[string]any{}

	if d.Value != nil || d.Writable {
		out["value"] = d.Value
		out["enumerable"] = d.Enumerable
		out["configurable"] = d.Configurable
		out["writable"] = d.Writable
	} else {
		out["get"] = d.Get
		out["set"] = d.Set
		out["enumerable"] = d.Enumerable
		out["configurable"] = d.Configurable
	}

	return out
}

func (c *Context) newValuesFromTuples(ctx context.Context, r *C.CallResult, n C.int) ([]*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		out := make([]*Value, int(n))
		pr := (*[1 << (maxArraySize - 18)]C.CallResult)(unsafe.Pointer(r))[:n:n]

		for i := 0; i < len(out); i++ {
			if v, err := c.newValueFromTuple(ctx, pr[i]); err != nil {
				return nil, err
			} else {
				out[i] = v
			}
		}

		return out, nil
	})

	if err != nil {
		return nil, err
	} else {
		return pv.([]*Value), nil
	}
}

func (c *Context) newValueFromTuple(ctx context.Context, r C.CallResult) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if value, err := c.newValue(ctx, r.result), c.isolate.newError(r.error); err != nil {
			C.v8_Value_ValueTuple_Release(c.pointer, r.result)
			return nil, err
		} else {
			if r.isError {
				return nil, value
			} else {
				return value, nil
			}
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) newValue(ctx context.Context, vt C.ValueTuplePtr) *Value {
	pv, _ := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if vt == nil {
			return nil, nil
		}

		if vt.value == nil {
			return nil, nil
		}

		var v *Value

		if vt.internal != nil && vt.value != nil {
			v = (*Value)(C.Pointer(vt.internal))
			v.refCount++
		} else {
			v = &Value{
				context: c,
				pointer: vt.value,
				kinds:   kinds(vt.kinds),
				info:    vt,
			}

			ptr := C.Pointer(v)
			v.info.internal = ptr
			v.context.values++
			v.refCount++

			runtime.SetFinalizer(v, (*Value).release)
		}

		return v, nil
	})

	if pv == nil {
		return nil
	} else {
		return pv.(*Value)
	}
}

func (v *Value) Error() string {
	ctx := v.context.isolate.GetExecutionContext()

	if rv := v.Receiver(ctx); !isZero(rv) && rv.Type().ConvertibleTo(errorType) {
		return rv.Interface().(error).Error()
	} else if v.IsKind(KindObject) {
		if stack, err := v.Get(ctx, "stack"); err != nil || stack.IsNil() {
			if err != nil {
				return err.Error()
			} else {
				return v.String()
			}
		} else {
			return fmt.Sprintf("%s", stack)
		}
	} else {
		return v.String()
	}
}

func (v *Value) Ref() refutils.ID {
	return v.context.refs.Ref(v)
}

func (v *Value) Unref() {
	v.context.refs.Unref(v)
}

func (v *Value) IsKind(k Kind) bool {
	return v.kinds.Is(k)
}

func (v *Value) IsNil() bool {
	return v == nil || v.kinds.Is(KindUndefined) || v.kinds.Is(KindNull)
}

func (v *Value) GetContext() *Context {
	return v.context
}

func (v *Value) DefineProperty(ctx context.Context, key string, descriptor *PropertyDescriptor) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pk := C.CString(key)
		var err C.Error

		if !descriptor.Value.IsNil() && descriptor.Get.IsNil() && descriptor.Set.IsNil() {
			err = C.v8_Value_DefinePropertyValue(v.context.pointer, v.pointer, pk, descriptor.Value.pointer, C.bool(descriptor.Configurable), C.bool(descriptor.Enumerable), C.bool(descriptor.Writable))
		} else {
			err = C.v8_Value_DefineProperty(v.context.pointer, v.pointer, pk, descriptor.Get.pointer, descriptor.Set.pointer, C.bool(descriptor.Configurable), C.bool(descriptor.Enumerable))
		}
		C.free(unsafe.Pointer(pk))
		return nil, v.context.isolate.newError(err)
	})

	return err
}

func (v *Value) Get(ctx context.Context, key string) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (any, error) {
		pk := C.CString(key)
		vt := C.v8_Value_Get(v.context.pointer, v.pointer, pk)
		C.free(unsafe.Pointer(pk))
		return v.context.newValueFromTuple(ctx, vt)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) GetLength(ctx context.Context) (int64, error) {
	if length, err := v.Get(ctx, "length"); err != nil {
		return 0, err
	} else {
		return length.Int64(ctx)
	}
}

func (v *Value) GetIndex(ctx context.Context, i int) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		return v.context.newValueFromTuple(ctx, C.v8_Value_GetIndex(v.context.pointer, v.pointer, C.int(i)))
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) Set(ctx context.Context, key string, value any) error {
	if pv, err := v.context.Create(ctx, value); err != nil {
		return err
	} else {
		return v.SetValue(ctx, key, pv)
	}
}

func (v *Value) SetValue(ctx context.Context, key string, value *Value) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if value == nil {
			return nil, fmt.Errorf("value is nil")
		}

		pk := C.CString(key)
		err := C.v8_Value_Set(v.context.pointer, v.pointer, pk, value.pointer)
		C.free(unsafe.Pointer(pk))
		return nil, v.context.isolate.newError(err)
	})

	return err
}

func (v *Value) SetIndex(ctx context.Context, i int, value *Value) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		return nil, v.context.isolate.newError(C.v8_Value_SetIndex(v.context.pointer, v.pointer, C.int(i), value.pointer))
	})

	return err
}

func (v *Value) SetInternalField(ctx context.Context, i int, value uint32) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		v.context.ref()
		defer v.context.unref()

		C.v8_Object_SetInternalField(v.context.pointer, v.pointer, C.int(i), C.uint32_t(value))
		return nil, nil
	})

	return err
}

func (v *Value) GetInternalField(ctx context.Context, i int) (int64, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		v.context.ref()
		defer v.context.unref()

		i := int64(C.v8_Object_GetInternalField(v.context.pointer, v.pointer, C.int(i)))
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int64)), nil
	}
}

func (v *Value) GetInternalFieldCount(ctx context.Context) (int, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		v.context.ref()
		defer v.context.unref()

		i := int(C.v8_Object_GetInternalFieldCount(v.context.pointer, v.pointer))
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int)), nil
	}
}

func (v *Value) Bind(ctx context.Context, argv ...any) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if bind, err := v.Get(ctx, "bind"); err != nil {
			return nil, err
		} else if fn, err := bind.Call(ctx, v, argv...); err != nil {
			return nil, err
		} else {
			return fn, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) BindMethod(ctx context.Context, method string, argv ...any) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if v, err := v.Get(ctx, method); err != nil {
			return nil, err
		} else if bind, err := v.Get(ctx, "bind"); err != nil {
			return nil, err
		} else if fn, err := bind.Call(ctx, v, argv...); err != nil {
			return nil, err
		} else {
			return fn, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) RebindAll(ctx context.Context) error {
	if descriptors, err := v.GetOwnPropertyDescriptors(ctx); err != nil {
		return err
	} else if prototype, err := v.GetPrototype(ctx); err != nil {
		return err
	} else if descriptorsp, err := prototype.GetOwnPropertyDescriptors(ctx); err != nil {
		return err
	} else {
		for k, descriptor := range descriptors {
			descriptorsp[k] = descriptor
		}

		for method := range descriptorsp {
			if m, err := v.Get(ctx, method); err != nil {
				return err
			} else if m.IsKind(KindFunction) {
				if m, err := m.Bind(ctx, v); err != nil {
					return err
				} else if err := v.DefineProperty(ctx, method, &PropertyDescriptor{
					Value:    m,
					Writable: true,
				}); err != nil {
					return err
				}
			}
		}

		return nil
	}
}

func (v *Value) RebindMethod(ctx context.Context, method string, args ...any) error {
	args = append([]any{v}, args...)

	if m, err := v.Get(ctx, method); err != nil {
		return err
	} else if !m.IsKind(KindFunction) {
		return nil
	} else if m, err := m.Bind(ctx, args...); err != nil {
		return err
	} else if wrapper, err := v.context.CreateWithName(ctx, method, func(in FunctionArgs) (*Value, error) {
		args := append([]*Value{in.This}, in.Args...)
		return m.CallValue(in.ExecutionContext, nil, args...)
	}); err != nil {
		return err
	} else if err := v.DefineProperty(ctx, method, &PropertyDescriptor{
		Value:    wrapper,
		Writable: true,
	}); err != nil {
		return err
	} else {
		return nil
	}
}

func (v *Value) Call(ctx context.Context, self any, args ...any) (*Value, error) {
	var this *Value
	var err error

	if self == nil {
		this = nil
	} else if this, err = v.context.Create(ctx, self); err != nil {
		return nil, err
	}

	argsv := make([]*Value, len(args))
	for i, arg := range args {
		if arg == nil {
			if argv, err := v.context.Undefined(ctx); err != nil {
				return nil, err
			} else {
				argsv[i] = argv
			}
		} else if argv, err := v.context.Create(ctx, arg); err != nil {
			return nil, err
		} else {
			argsv[i] = argv
		}
	}

	return v.CallValue(ctx, this, argsv...)
}

func (v *Value) CallValue(ctx context.Context, self *Value, argv ...*Value) (*Value, error) {
	v.context.ref()
	defer v.context.unref()

	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if v.pointer == nil {
			return nil, fmt.Errorf("CallValue on %s", v)
		}

		pargv := make([]C.ValuePtr, len(argv)+1)
		for i, argvi := range argv {
			if argvi.pointer == nil {
				return nil, fmt.Errorf("passed arg(%d) is %s for call to %s", i, argvi, v)
			}
			pargv[i] = argvi.pointer
		}

		pself := C.ValuePtr(nil)
		if self != nil {
			if self.pointer == nil {
				return nil, fmt.Errorf("passed self is %s for call to %s", self, v)
			}
			pself = self.pointer
		}

		if v.context.pointer == nil {
			return nil, fmt.Errorf("context released for call to %s", v)
		}

		vt := C.v8_Value_Call(v.context.pointer, v.pointer, pself, C.int(len(argv)), &pargv[0])

		if value, err := v.context.newValueFromTuple(ctx, vt); err != nil {
			return nil, err
		} else {
			return value, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}

}

func (v *Value) CallMethod(ctx context.Context, name string, argv ...any) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if m, err := v.Get(ctx, name); err != nil {
			return nil, err
		} else if m.IsNil() || !m.IsKind(KindFunction) {
			return nil, fmt.Errorf("not a function: %s", m)
		} else if value, err := m.Call(ctx, v, argv...); err != nil {
			return nil, err
		} else {
			return value, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) New(ctx context.Context, args ...any) (*Value, error) {
	argsv := make([]*Value, len(args))
	for i, arg := range args {
		if arg == nil {
			if argv, err := v.context.Undefined(ctx); err != nil {
				return nil, err
			} else {
				argsv[i] = argv
			}
		} else if argv, err := v.context.Create(ctx, arg); err != nil {
			return nil, err
		} else {
			argsv[i] = argv
		}
	}

	return v.NewValue(ctx, argsv...)
}

func (v *Value) NewValue(ctx context.Context, argv ...*Value) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		pargv := make([]C.ValuePtr, len(argv)+1)

		for i, argvi := range argv {
			pargv[i] = argvi.pointer
		}

		v.context.ref()
		vt := C.v8_Value_New(v.context.pointer, v.pointer, C.int(len(argv)), &pargv[0])
		v.context.unref()

		return v.context.newValueFromTuple(ctx, vt)
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) ToError(ctx context.Context) error {
	if v.IsKind(KindUndefined) || v.IsKind(KindNull) {
		return nil
	} else if errv, err := v.Unmarshal(ctx, errorType); err != nil {
		return err
	} else {
		return errv.Interface().(error)
	}
}

func (v *Value) Keys(ctx context.Context) ([]string, error) {
	sa, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if keysv, err := v.context.keys.Call(ctx, nil, v); err != nil {
			return nil, err
		} else if keys, err := keysv.Unmarshal(ctx, reflect.TypeOf([]string{})); err != nil {
			return nil, err
		} else {
			return keys.Interface().([]string), nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return sa.([]string), nil
	}
}

func (v *Value) InstanceOf(ctx context.Context, object *Value) bool {
	pb, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		cb := C.v8_Value_InstanceOf(v.context.pointer, v.pointer, object.pointer)
		b := bool(cb)
		return &b, nil
	})

	if err != nil {
		return false
	} else {
		return *(pb.(*bool))
	}
}

func (v *Value) GetOwnPropertyDescriptors(ctx context.Context) (map[string]PropertyDescriptor, error) {
	pds, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if descriptorsv, err := v.context.getOwnPropertyDescriptors.Call(ctx, nil, v); err != nil {
			return nil, err
		} else if descriptorsrv, err := descriptorsv.Unmarshal(ctx, reflect.TypeOf(map[string]PropertyDescriptor{})); err != nil {
			return nil, err
		} else {
			return descriptorsrv.Interface().(map[string]PropertyDescriptor), nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pds.(map[string]PropertyDescriptor), nil
	}
}

func (v *Value) GetPrototype(ctx context.Context) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if prototype, err := v.context.getPrototypeOf.Call(ctx, nil, v); err != nil {
			return nil, err
		} else {
			return prototype, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) Bytes(ctx context.Context) ([]byte, error) {
	b, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := C.v8_Value_Bytes(v.context.pointer, v.pointer)
		if b.data == nil {
			return nil, nil
		}

		buf := C.GoBytes(unsafe.Pointer(b.data), b.length)

		return buf, nil
	})

	if err != nil {
		return nil, err
	} else if b == nil {
		return nil, nil
	} else {
		return b.([]byte), nil
	}
}

func (v *Value) SetBytes(ctx context.Context, bytes []byte) error {
	_, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := C.v8_Value_Bytes(v.context.pointer, v.pointer)
		if b.data == nil {
			return nil, nil
		}

		copy(((*[1 << (maxArraySize - 13)]byte)(unsafe.Pointer(b.data)))[:len(bytes):len(bytes)], bytes)
		return nil, nil
	})

	return err
}

func (v *Value) GetByteLength(ctx context.Context) (int, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		bytes := C.v8_Value_ByteLength(v.context.pointer, v.pointer)
		i := int(bytes)
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int)), nil
	}
}

func (v *Value) Int64(ctx context.Context) (int64, error) {
	pi, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		i := int64(C.v8_Value_Int64(v.context.pointer, v.pointer))
		return &i, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pi.(*int64)), nil
	}
}

func (v *Value) Float64(ctx context.Context) (float64, error) {
	pf, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		f := float64(C.v8_Value_Float64(v.context.pointer, v.pointer))
		return &f, nil
	})

	if err != nil {
		return 0, err
	} else {
		return *(pf.(*float64)), nil
	}
}

func (v *Value) Bool(ctx context.Context) (bool, error) {
	pb, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := C.v8_Value_Bool(v.context.pointer, v.pointer) == 1
		return &b, nil
	})

	if err != nil {
		return false, err
	} else {
		return *(pb.(*bool)), nil
	}
}

func (v *Value) Date(ctx context.Context) (time.Time, error) {
	pt, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if !v.IsKind(KindDate) {
			return time.Time{}, errors.New("not a date")
		} else if ms, err := v.Int64(ctx); err != nil {
			return time.Time{}, nil
		} else {
			s := ms / 1000
			ns := (ms % 1000) * 1e6
			return time.Unix(s, ns), nil
		}
	})

	if err != nil {
		return time.Time{}, err
	} else {
		return pt.(time.Time), nil
	}
}

func (v *Value) Equals(ctx context.Context, other *Value) (bool, error) {
	pb, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := false
		if v.GetContext() != other.GetContext() {
			return &b, nil
		} else {
			cb := C.v8_Value_Equals(v.context.pointer, v.pointer, other.pointer)
			pb := &cb
			return pb, nil
		}
	})

	if err != nil {
		return false, err
	} else {
		return *(pb.(*bool)), nil
	}
}

func (v *Value) StrictEquals(ctx context.Context, other *Value) (bool, error) {
	pb, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		b := false
		if v.GetContext() != other.GetContext() {
			return &b, nil
		} else {
			cb := C.v8_Value_StrictEquals(v.context.pointer, v.pointer, other.pointer)
			pb := bool(cb)
			return &pb, nil
		}
	})

	if err != nil {
		return false, err
	} else {
		return *(pb.(*bool)), nil
	}
}

func (v *Value) PromiseInfo(ctx context.Context) (PromiseState, *Value, error) {
	var state C.int

	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if !v.IsKind(KindPromise) {
			return nil, errors.New("not a promise")
		}

		return v.context.newValueFromTuple(ctx, C.v8_Value_PromiseInfo(v.context.pointer, v.pointer, &state))
	})

	if err != nil {
		return 0, nil, err
	} else {
		return PromiseState(state), pv.(*Value), nil
	}
}

func (v *Value) Await(ctx context.Context) (*Value, error) {
	pv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if !v.IsKind(KindPromise) {
			return v, nil
		}

		resolved := make(chan func() (*Value, error))

		if then, err := v.Get(ctx, "then"); err != nil {
			return nil, err
		} else if resolve, err := v.context.Create(ctx, func(in FunctionArgs) (*Value, error) {
			resolved <- func() (*Value, error) {
				return in.Arg(in.ExecutionContext, 0), nil
			}
			close(resolved)
			return nil, nil
		}); err != nil {
			return nil, err
		} else if reject, err := v.context.Create(ctx, func(in FunctionArgs) (*Value, error) {
			resolved <- func() (*Value, error) {
				errValue := in.Arg(in.ExecutionContext, 0)
				if stackValue, err := errValue.Get(in.ExecutionContext, "stack"); err == nil {
					if stack, err := stackValue.StringValue(in.ExecutionContext); err != nil {
						return nil, err
					} else {
						return nil, fmt.Errorf(stack)
					}
				} else if message, err := errValue.StringValue(in.ExecutionContext); err != nil {
					return nil, err
				} else {
					return nil, fmt.Errorf(message)
				}
			}
			close(resolved)
			return nil, nil
		}); err != nil {
			return nil, err
		} else {
			v.context.isolate.Background(ctx, func(ctx context.Context) {
				if _, err := then.Call(ctx, v, resolve, reject); err != nil {
					For(ctx).Error(err)
				}
			})
			return (<-resolved)()
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (v *Value) String() string {
	if v.pointer == nil {
		return "[released Value]"
	}

	if v.context == nil || v.context.pointer == nil {
		return "[Value in released Context]"
	}

	ps := C.v8_Value_String(v.context.pointer, v.pointer)
	defer C.free(unsafe.Pointer(ps.data))

	s := C.GoStringN(ps.data, ps.length)
	return s
}

func (v *Value) StringValue(ctx context.Context) (string, error) {
	s, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if v.pointer == nil {
			return "[released Value]", nil
		}

		if v.context == nil || v.context.pointer == nil {
			return "[Value in released Context]", nil
		}

		ps := C.v8_Value_String(v.context.pointer, v.pointer)
		defer C.free(unsafe.Pointer(ps.data))

		s := C.GoStringN(ps.data, ps.length)
		return s, nil
	})

	if err != nil {
		return "", err
	} else {
		return s.(string), nil
	}
}

func (v *Value) MarshalJSON(ctx context.Context) ([]byte, error) {
	b, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if s, err := v.context.newValueFromTuple(ctx, C.v8_JSON_Stringify(v.context.pointer, v.pointer)); err != nil {
			return nil, err
		} else if string, err := s.StringValue(ctx); err != nil {
			return nil, err
		} else {
			return []byte(string), nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return b.([]byte), nil
	}
}

func (v *Value) Constructor(t reflect.Type) *FunctionTemplate {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if ft, ok := v.context.constructors[t]; ok {
		return ft
	} else if ft, ok := v.context.prototypes[t]; ok {
		return ft
	} else {
		return nil
	}
}

func (v *Value) Receiver(ctx context.Context) reflect.Value {
	return v.receiver
}

func (v *Value) SetReceiver(ctx context.Context, value reflect.Value) {
	if !v.IsKind(KindObject) {
		panic("trying to set receiver on non-object")
	}

	v.context.receiversMutex.Lock()
	defer v.context.receiversMutex.Unlock()

	if !isZero(v.receiver) {
		delete(v.context.receivers, v.receiver.Pointer())
	}

	v.receiver = value

	if value.Kind() != reflect.Pointer && value.Kind() != reflect.Array && value.Kind() != reflect.Map && value.Kind() != reflect.Chan && value.Kind() != reflect.Func {
		value = value.Addr()
	}

	v.context.receivers[value.Pointer()] = v
}

// func (v *Value) AddFinalizer(finalizer func()) {
// 	v.finalizers = append(v.finalizers, finalizer)
// }

type weakCallbackInfo struct {
	object   interface{}
	callback func()
}

//export valueWeakCallbackHandler
func valueWeakCallbackHandler(pid C.String) {
	// 	ids := C.GoStringN(pid.data, pid.length)
	//
	// 	parts := strings.SplitN(ids, ":", 3)
	// 	isolateId, _ := strconv.Atoi(parts[0])
	// 	contextId, _ := strconv.Atoi(parts[1])
	//
	// 	isolateRef := isolates.Get(refutils.ID(isolateId))
	// 	if isolateRef == nil {
	// 		panic(fmt.Errorf("missing isolate pointer during weak callback for isolate #%d", isolateId))
	// 	}
	// 	isolate := isolateRef.(*Isolate)
	//
	// 	contextRef := isolate.contexts.Get(refutils.ID(contextId))
	// 	if contextRef == nil {
	// 		panic(fmt.Errorf("missing context pointer during weak callback for context #%d", contextId))
	// 	}
	// 	context := contextRef.(*Context)
	//
	// 	context.weakCallbackMutex.Lock()
	// 	if info, ok := context.weakCallbacks[ids]; !ok {
	// 		panic(fmt.Errorf("missing callback pointer during weak callback"))
	// 	} else {
	// 		context.weakCallbackMutex.Unlock()
	// 		info.callback()
	// 		delete(context.weakCallbacks, ids)
	// 	}
	// }
	//
	// func (v *Value) setWeak(id string, callback func()) error {
	// 	if err := v.context.isolate.lock(); err != nil {
	// 		return err
	// 	} else {
	// 		defer v.context.isolate.unlock()
	// 	}
	//
	// 	pid := C.CString(id)
	// 	defer C.free(unsafe.Pointer(pid))
	//
	// 	v.context.weakCallbackMutex.Lock()
	// 	v.context.weakCallbacks[id] = &weakCallbackInfo{v, callback}
	// 	v.context.weakCallbackMutex.Unlock()
	// 	C.v8_Value_SetWeak(v.context.pointer, v.pointer, pid)
	// 	return nil
}

func (v *Value) release() {
	ctx := v.context.isolate.GetExecutionContext()

	v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		v.refCount--

		if v.refCount > 0 {
			runtime.SetFinalizer(v, (*Value).release)
			return nil, nil
		} else {
			runtime.SetFinalizer(v, nil)
		}

		if v.info == nil {
			panic(fmt.Errorf("overrelease on instance: %s (%s)", v, v.kinds))
		}

		// tracer.Release(v)
		v.info.internal = nil
		C.v8_Value_ValueTuple_Release(v.context.pointer, v.info)
		v.context.values--

		v.context = nil
		v.info = nil
		v.pointer = nil
		v.kinds = kinds(KindUndefined)

		return nil, nil
	})
}

func (d *PropertyDescriptor) V8Construct(in FunctionArgs) error {
	return nil
}
