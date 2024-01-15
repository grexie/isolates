package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unsafe"
)

var marshallers = map[reflect.Type]reflect.Value{}
var unmarshallers = map[reflect.Type]reflect.Value{}

func AddMarshaller(t reflect.Type, marshaller any) error {
	marshallers[t] = reflect.ValueOf(marshaller)
	return nil
}

func AddUnmarshaller(t reflect.Type, unmarshaller any) error {
	unmarshallers[t] = reflect.ValueOf(unmarshaller)
	return nil
}

type Marshaler interface {
	MarshalV8(ctx context.Context) any
}

type stringKeys []reflect.Value

func (s stringKeys) Len() int {
	return len(s)
}

func (s stringKeys) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

func (s stringKeys) Less(a, b int) bool {
	return s[a].String() < s[b].String()
}

var float64Type = reflect.TypeOf(float64(0))
var functionType = reflect.TypeOf(Function(nil))
var getterType = reflect.TypeOf(Getter(nil))
var setterType = reflect.TypeOf(Setter(nil))
var stringType = reflect.TypeOf(string(""))
var timeType = reflect.TypeOf(time.Time{})
var durationType = reflect.TypeOf(time.Duration(0))
var valueType = reflect.TypeOf((*Value)(nil))
var anyType = reflect.TypeOf((any)(nil))
var marshalerType = reflect.TypeOf((*Marshaler)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()
var v8ErrorType = reflect.TypeOf((*Error)(nil)).Elem()
var functionArgsType = reflect.TypeOf((*FunctionArgs)(nil)).Elem()

// https://stackoverflow.com/a/23555352
func isZero(v reflect.Value) bool {
	if !v.IsValid() || v.IsZero() {
		return true
	}

	switch v.Kind() {
	case reflect.Func, reflect.Map, reflect.Slice:
		return v.IsNil()
	case reflect.Array:
		z := true
		for i := 0; i < v.Len(); i++ {
			z = z && isZero(v.Index(i))
		}
		return z
	case reflect.Struct:
		z := true
		for i := 0; i < v.NumField(); i++ {
			z = z && isZero(v.Field(i))
		}
		return z
	}

	// Compare other types directly:
	z := reflect.Zero(v.Type())

	if v.CanInterface() && z.CanInterface() {
		return v.Interface() == z.Interface()
	} else {
		return false
	}
}

func (c *Context) Create(ctx context.Context, v interface{}) (*Value, error) {
	return c._create(ctx, nil, v, true)
}

func (c *Context) CreateWithoutMarshallers(ctx context.Context, v interface{}) (*Value, error) {
	return c._create(ctx, nil, v, false)
}

func (c *Context) New(ctx context.Context, cons any, args ...any) (any, error) {
	ct := reflect.TypeOf(cons)
	out := ct.Out(0)

	v, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		values := make([]any, len(args))

		if constructor, err := c.createConstructor(ctx, nil, cons); err != nil {
			return nil, err
		} else {
			for i, arg := range args {
				if value, err := c.Create(ctx, arg); err != nil {
					return nil, err
				} else {
					values[i] = value
				}
			}

			if value, err := constructor.New(ctx, values...); err != nil {
				return nil, err
			} else if v, err := value.Unmarshal(ctx, out); err != nil {
				return nil, err
			} else {
				return v.Interface(), nil
			}
		}

	})

	if err != nil {
		return nil, err
	} else {
		return v, nil
	}
}

func (c *Context) CreateWithName(ctx context.Context, name string, v interface{}) (*Value, error) {
	return c._create(ctx, &name, v, true)
}

func (c *Context) _create(ctx context.Context, name *string, v any, withMarshallers bool) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (any, error) {
		rv := reflect.ValueOf(v)
		value, err := c.create(ctx, rv, name, withMarshallers)
		return value, err
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}
}

func (c *Context) createImmediateValue(ctx context.Context, v C.ImmediateValue) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		return c.newValueFromTuple(ctx, C.v8_Context_Create(c.pointer, v))
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), err
	}

}

func marshalValue(ctx context.Context, val reflect.Value) reflect.Value {
	if val.Type().Implements(reflect.TypeOf((*Marshaler)(nil)).Elem()) {
		m := val.MethodByName("MarshalV8")
		val = m.Call([]reflect.Value{reflect.ValueOf(ctx)})[0]
	} else if val.Type().Kind() != reflect.Ptr && reflect.PtrTo(val.Type()).Implements(reflect.TypeOf((*Marshaler)(nil)).Elem()) {
		m := val.Addr().MethodByName("MarshalV8")
		val = m.Call([]reflect.Value{reflect.ValueOf(ctx)})[0]
	}
	return val
}

func (c *Context) create(ctx context.Context, v reflect.Value, name *string, withMarshallers bool) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if !v.IsValid() || (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer || v.Kind() == reflect.Ptr || v.Kind() == reflect.Func || v.Kind() == reflect.Chan || v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.IsNil() {
			return c.Undefined(ctx)
		}

		if v.CanAddr() && v.Addr().CanConvert(valueType) {
			value := v.Addr().Interface().(*Value)
			return value, nil
		} else if v.CanConvert(valueType) {
			value := v.Interface().(*Value)
			return value, nil
		}

		v = marshalValue(ctx, v)

		if withMarshallers {
			if marshaller, ok := marshallers[v.Type()]; ok {
				rvs := marshaller.Call([]reflect.Value{reflect.ValueOf(ctx), v})
				if !rvs[1].IsNil() {
					return nil, rvs[1].Interface().(error)
				} else {
					v = rvs[0]
				}
			}
		}

		if v.Type().ConvertibleTo(errorType) && !v.Type().ConvertibleTo(v8ErrorType) {
			if global, err := c.Global(ctx); err != nil {
				return nil, err
			} else if errorClass, err := global.Get(ctx, "Error"); err != nil {
				return nil, err
			} else if message, err := c.create(ctx, reflect.ValueOf(fmt.Sprintf("%v", v.Interface())), name, withMarshallers); err != nil {
				return nil, err
			} else if errorObject, err := errorClass.New(ctx, message); err != nil {
				return nil, err
			} else {
				return errorObject, nil
			}
		}

		if v.Type() == timeType {
			msec := C.double(v.Interface().(time.Time).UnixNano()) / 1e6
			return c.createImmediateValue(ctx, C.ImmediateValue{_type: C.tDATE, _float64: msec})
		}

		if v.Type() == durationType {
			msec := C.double(v.Interface().(time.Duration).Nanoseconds()) / 1e6
			return c.createImmediateValue(ctx, C.ImmediateValue{_type: C.tFLOAT64, _float64: msec})
		}

		switch v.Kind() {
		case reflect.Bool:
			b := C.bool(false)
			if v.Bool() {
				b = C.bool(true)
			}
			return c.createImmediateValue(ctx, C.ImmediateValue{_type: C.tBOOL, _bool: b})
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			n := C.double(v.Convert(float64Type).Float())
			return c.createImmediateValue(ctx, C.ImmediateValue{_type: C.tFLOAT64, _float64: n})
		case reflect.String:
			s := v.String()
			ps := C.ByteArray{data: C.CString(s), length: C.int(len(s))}
			defer C.free(unsafe.Pointer(ps.data))
			return c.createImmediateValue(ctx, C.ImmediateValue{_type: C.tSTRING, _data: ps})
		case reflect.UnsafePointer, reflect.Uintptr:
			s := fmt.Sprintf("%p", v.UnsafePointer())
			ps := C.ByteArray{data: C.CString(s), length: C.int(len(s))}
			defer C.free(unsafe.Pointer(ps.data))
			return c.createImmediateValue(ctx, C.ImmediateValue{_type: C.tSTRING, _data: ps})
		case reflect.Complex64, reflect.Complex128:
			return nil, fmt.Errorf("complex not supported: %#v", v.Interface())
		case reflect.Chan:
			return nil, fmt.Errorf("chan not supported: %#v", v.Interface())
		case reflect.Func:
			if v.Type().ConvertibleTo(functionType) {
				return c.CreateFunction(ctx, name, v.Convert(functionType).Interface().(Function))
			} else if err := isConstructor(v.Type()); err == nil {
				return c.createConstructor(ctx, name, v.Interface())
			}
			return nil, fmt.Errorf("func not supported: %#v", v.Interface())
		case reflect.Interface, reflect.Ptr:
			return c.create(ctx, v.Elem(), name, withMarshallers)
		case reflect.Map:
			if v.Type().Key() != stringType {
				return nil, fmt.Errorf("map keys must be strings, %s not permissable in v8", v.Type().Key())
			}

			if o, err := c.createImmediateValue(ctx, C.ImmediateValue{_type: C.tOBJECT}); err != nil {
				return nil, err
			} else {
				keys := v.MapKeys()
				sort.Sort(stringKeys(keys))
				for _, k := range keys {
					if vk, err := c.create(ctx, v.MapIndex(k), name, withMarshallers); err != nil {
						return nil, fmt.Errorf("map key %q: %v", k.String(), err)
					} else if err := o.Set(ctx, k.String(), vk); err != nil {
						return nil, err
					}
				}

				return o, nil
			}
		case reflect.Struct:
			c.receiversMutex.Lock()
			value, ok := c.receivers[v.Addr().Pointer()]
			c.receiversMutex.Unlock()

			if ok {
				return value, nil
			} else if p, err := c.createPrototype(ctx, nil, v, v.Type()); err != nil {
				return nil, err
			} else if fn, err := p.GetFunction(ctx); err != nil {
				return nil, err
			} else if value, err := fn.NewValue(ctx); err != nil {
				return nil, err
			} else if method, ok := reflect.PointerTo(v.Type()).MethodByName("V8Construct"); ok {
				value.SetReceiver(ctx, v.Addr())

				mrv := v

				if method.Type.In(0).Kind() == reflect.Pointer && v.Kind() != reflect.Pointer {
					mrv = mrv.Addr()
				} else if method.Type.In(0).Kind() != reflect.Pointer && v.Kind() == reflect.Pointer {
					mrv = mrv.Elem()
				}

				method.Func.Call([]reflect.Value{mrv, reflect.ValueOf(FunctionArgs{
					ExecutionContext: ctx,
					Context:          c,
					This:             value,
					IsConstructCall:  true,
					Args:             []*Value{},
					Holder:           value,
				})})

				return value, nil
			} else {

				// log.Println("SETTING RECEIVER", value, v)
				value.SetReceiver(ctx, v.Addr())
				return value, nil
			}
		case reflect.Array, reflect.Slice:
			if v.Type().Elem().Kind() == reflect.Uint8 {
				if v.Type().Kind() == reflect.Array {
					v = v.Slice(0, v.Len())
				}

				b := v.Bytes()
				var pb *C.char
				if b != nil && len(b) > 0 {
					pb = (*C.char)(unsafe.Pointer(&v.Bytes()[0]))
				}

				return c.createImmediateValue(ctx,
					C.ImmediateValue{
						_type: C.tARRAYBUFFER,
						_data: C.ByteArray{data: pb, length: C.int(v.Len())},
					},
				)
			} else {
				if o, err := c.createImmediateValue(ctx,
					C.ImmediateValue{
						_type: C.tARRAY,
						_data: C.ByteArray{data: nil, length: C.int(v.Len())},
					},
				); err != nil {
					return nil, err
				} else {
					for i := 0; i < v.Len(); i++ {
						if v, err := c.create(ctx, v.Index(i), name, withMarshallers); err != nil {
							return nil, fmt.Errorf("index %d: %v", i, err)
						} else if err := o.SetIndex(ctx, i, v); err != nil {
							return nil, err
						}
					}

					return o, nil
				}
			}
		}

		panic(fmt.Sprintf("unsupported kind: %v", v.Kind()))
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), nil
	}

}

func (c *Context) CreateFunction(ctx context.Context, name *string, function Function) (*Value, error) {
	v, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if ft, err := c.NewFunctionTemplate(ctx, function); err != nil {
			return nil, err
		} else {
			if name != nil {
				ft.SetName(ctx, *name)
			}
			return ft.GetFunction(ctx)
		}
	})

	if err != nil {
		return nil, err
	} else {
		return v.(*Value), nil
	}
}

func getName(name string) string {
	// split the string into tokens beginning with uppercase letters
	var w1 []string
	i := 0
	for s := name; s != ""; s = s[i:] {
		i = strings.IndexFunc(s[1:], unicode.IsUpper) + 1
		if i <= 0 {
			i = len(s)
		}
		w1 = append(w1, s[:i])
	}

	// convert strings of uppercase letters to camelcase
	var w2 []string
	for j := 0; j < len(w1); j++ {
		if len(w2) > 0 && strings.ToUpper(w1[j]) == w1[j] {
			w2[len(w2)-1] += strings.ToLower(w1[j])
		} else {
			w2 = append(w2, strings.ToLower(w1[j]))
		}
	}

	// title every word after the first
	for k := 1; k < len(w2); k++ {
		w2[k] = strings.Title(w2[k])
	}

	return strings.Join(w2, "")
}

func isConstructor(constructor reflect.Type) error {
	if constructor.NumIn() != 1 || !constructor.In(0).ConvertibleTo(functionArgsType) {
		return fmt.Errorf("expected input args to be of type FunctionArgs")
	}

	if constructor.NumOut() != 2 || !constructor.Out(1).Implements(errorType) {
		return fmt.Errorf("expected multi-value context with error as the second value")
	}

	return nil
}

func (c *Context) createConstructor(ctx context.Context, name *string, cons interface{}) (*Value, error) {
	pv, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		cv := reflect.ValueOf(cons)
		constructor := cv.Type()
		prototype := constructor.Out(0)

		if prototype.Kind() == reflect.Ptr || prototype.Kind() == reflect.Interface {
			prototype = prototype.Elem()
		}

		if fn, ok := c.constructors[prototype]; ok {
			return fn.GetFunction(ctx)
		} else if fn, err := c.createConstructorInstance(ctx, name, cons, true); err != nil {
			return nil, err
		} else {
			c.constructors[prototype] = fn
			c.prototypes[prototype] = fn
			return fn.GetFunction(ctx)
		}
	})

	if err != nil {
		return nil, err
	} else {
		return pv.(*Value), err
	}
}

func (c *Context) CreateConstructor(ctx context.Context, cons interface{}) (*Value, error) {
	if fn, err := c.createConstructorInstance(ctx, nil, cons, false); err != nil {
		return nil, err
	} else {
		return fn.GetFunction(ctx)
	}
}

func (c *Context) CreateConstructorWithName(ctx context.Context, name string, cons interface{}) (*Value, error) {
	if fn, err := c.createConstructorInstance(ctx, &name, cons, false); err != nil {
		return nil, err
	} else {
		return fn.GetFunction(ctx)
	}
}

func (c *Context) createConstructorInstance(ctx context.Context, name *string, cons interface{}, shared bool) (*FunctionTemplate, error) {
	fn, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		cv := reflect.ValueOf(cons)
		constructor := cv.Type()
		prototype := constructor.Out(0)

		if prototype.Kind() == reflect.Ptr || prototype.Kind() == reflect.Interface {
			prototype = prototype.Elem()
		}

		if fn, err := c.createPrototypeInstance(ctx, name, reflect.Zero(prototype), prototype, shared); err != nil {
			return nil, err
		} else {
			fn.info.Function = func(in FunctionArgs) (*Value, error) {
				r := cv.Call([]reflect.Value{reflect.ValueOf(in)})

				if r[1].Interface() != nil {
					return nil, r[1].Interface().(error)
				}

				in.This.SetReceiver(in.ExecutionContext, r[0])
				return in.This, nil
			}

			return fn, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return fn.(*FunctionTemplate), err
	}
}

func (c *Context) createPrototype(ctx context.Context, name *string, v reflect.Value, prototype reflect.Type) (*FunctionTemplate, error) {
	return c.createPrototypeInstance(ctx, name, v, prototype, true)
}

func (c *Context) createPrototypeInstance(ctx context.Context, name *string, v reflect.Value, prototype reflect.Type, shared bool) (*FunctionTemplate, error) {
	ft, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {

		if prototype.Kind() == reflect.Ptr || prototype.Kind() == reflect.Interface {
			prototype = prototype.Elem()
		}

		if fn, ok := c.prototypes[prototype]; shared && ok {
			return fn, nil
		} else if fn, err := c.NewFunctionTemplate(ctx, func(in FunctionArgs) (*Value, error) {
			return in.This, nil
		}); err != nil {
			return nil, err
		} else if instance, err := fn.GetInstanceTemplate(ctx); err != nil {
			return nil, err
		} else if err := instance.SetInternalFieldCount(ctx, 1); err != nil {
			return nil, err
		} else if proto, err := fn.GetPrototypeTemplate(ctx); err != nil {
			return nil, err
		} else if err := c.writePrototypeFields(ctx, instance, proto, v, prototype); err != nil {
			return nil, err
		} else {
			if name == nil {
				n := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(prototype.Name(), "Base"), "Base"), "Impl")
				name = &n
			}
			if name != nil {
				if err := fn.SetName(ctx, *name); err != nil {
					return nil, err
				}
			}

			baseFns := []*FunctionTemplate{}

			if prototype.Kind() == reflect.Struct {
				for i := 0; i < prototype.NumField(); i++ {
					f := prototype.Field(i)

					if f.Anonymous {
						sub := prototype.Field(i).Type

						for sub.Kind() == reflect.Ptr {
							sub = sub.Elem()
						}

						if sub.Kind() == reflect.Struct {
							elem := reflect.Zero(sub)

							if _, err := c.createPrototype(ctx, nil, elem, sub); err != nil {
								return nil, err
							} else {
								baseFns = append(baseFns, c.prototypes[sub])
							}
						}
					}
				}
			}

			if len(baseFns) > 0 {
				if err := fn.Inherit(ctx, baseFns[0]); err != nil {
					return nil, err
				}
			}

			if len(baseFns) > 1 {
				for _, baseFn := range baseFns[1:] {
					if pt, err := baseFn.GetPrototypeTemplate(ctx); err != nil {
						return nil, err
					} else if it, err := baseFn.GetInstanceTemplate(ctx); err != nil {
						return nil, err
					} else if err := pt.Copy(ctx, proto); err != nil {
						return nil, err
					} else if err := it.Copy(ctx, instance); err != nil {
						return nil, err
					}
				}
			}

			if shared {
				c.prototypes[prototype] = fn
			}

			return fn, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return ft.(*FunctionTemplate), err
	}
}

func (c *Context) Receiver(ctx context.Context, self *Value, t reflect.Type) (reflect.Value, error) {
	if self.IsNil() {
		return reflect.Value{}, nil
	}

	if r := self.Receiver(ctx); isZero(r) {
		if m, ok := t.MethodByName("V8Construct"); ok {
			in := FunctionArgs{
				ExecutionContext: ctx,
				Context:          c,
				This:             self,
				IsConstructCall:  true,
				Args:             []*Value{},
				Holder:           self,
			}

			v := m.Func.Call([]reflect.Value{reflect.New(t.Elem()), reflect.ValueOf(in)})
			self.SetReceiver(ctx, v[0])

			if !v[1].IsNil() {
				return reflect.Value{}, v[1].Interface().(error)
			} else {
				return v[0], nil
			}
		} else {
			return reflect.Value{}, fmt.Errorf("receiver not found")
		}
	} else {
		return r, nil
	}
}

func (c *Context) createFieldGetter(ctx context.Context, t reflect.Type, name string) Getter {
	return func(in GetterArgs) (*Value, error) {
		pv, err := in.Context.isolate.Sync(in.ExecutionContext, func(ctx context.Context) (interface{}, error) {
			if r, err := c.Receiver(ctx, in.Holder, t); err != nil {
				return nil, err
			} else {
				for r.Kind() == reflect.Ptr && !isZero(r) {
					r = r.Elem()
				}

				fval := r.FieldByName(name)
				if v, err := in.Context.create(ctx, fval, nil, true); err != nil {
					return nil, fmt.Errorf("field %q: %v", name, err)
				} else {
					return v, nil
				}
			}
		})

		if err != nil {
			return nil, err
		} else {
			return pv.(*Value), nil
		}
	}
}

func (c *Context) createFieldSetter(ctx context.Context, t reflect.Type, name string) Setter {
	return func(in SetterArgs) error {
		_, err := in.Context.isolate.Sync(in.ExecutionContext, func(ctx context.Context) (interface{}, error) {
			if r, err := c.Receiver(ctx, in.Holder, t); err != nil {
				return nil, err
			} else {
				if r.Kind() == reflect.Ptr {
					r = r.Elem()
				}

				fval := r.FieldByName(name)
				if v, err := in.Value.Unmarshal(ctx, fval.Type()); err != nil {
					return nil, err
				} else {
					fval.Set(*v)
					return nil, nil
				}
			}
		})

		return err
	}
}

func (c *Context) createGetter(ctx context.Context, t reflect.Type, method reflect.Value) Getter {
	methodNameParts := strings.Split(runtime.FuncForPC(method.Pointer()).Name(), ".")
	methodName := methodNameParts[len(methodNameParts)-1]
	methodReceiver := strings.Trim(methodNameParts[len(methodNameParts)-2], "(*)")
	return func(in GetterArgs) (*Value, error) {
		pv, err := in.Context.isolate.Sync(in.ExecutionContext, func(ctx context.Context) (interface{}, error) {
			if r, err := c.Receiver(ctx, in.Holder, t); err != nil {
				return nil, err
			} else {
				if r.Kind() == reflect.Pointer || r.Kind() == reflect.Interface {
					if methodReceiver != r.Type().Elem().Name() {
						r = r.Elem().FieldByName(methodReceiver)
					}
				} else if methodReceiver != r.Type().Name() {
					r = r.FieldByName(methodReceiver)
				}
				method := r.MethodByName(methodName)
				v := method.Call([]reflect.Value{reflect.ValueOf(in)})

				if v1, ok := v[1].Interface().(error); ok {
					return nil, v1
				} else if v0, ok := v[0].Interface().(*Value); ok {
					return v0, nil
				} else {
					return nil, nil
				}
			}
		})

		if err != nil {
			return nil, err
		} else {
			return pv.(*Value), nil
		}
	}
}

func (c *Context) createSetter(ctx context.Context, t reflect.Type, method reflect.Value) Setter {
	methodNameParts := strings.Split(runtime.FuncForPC(method.Pointer()).Name(), ".")
	methodName := methodNameParts[len(methodNameParts)-1]
	methodReceiver := strings.Trim(methodNameParts[len(methodNameParts)-2], "(*)")
	return func(in SetterArgs) error {
		_, err := in.Context.isolate.Sync(in.ExecutionContext, func(ctx context.Context) (interface{}, error) {

			if r, err := c.Receiver(ctx, in.Holder, t); err != nil {
				return nil, err
			} else {
				if r.Kind() == reflect.Pointer || r.Kind() == reflect.Interface {
					if methodReceiver != r.Type().Elem().Name() {
						r = r.Elem().FieldByName(methodReceiver)
					}
				} else if methodReceiver != r.Type().Name() {
					r = r.FieldByName(methodReceiver)
				}
				method := r.MethodByName(methodName)
				v := method.Call([]reflect.Value{reflect.ValueOf(in)})

				if v1, ok := v[0].Interface().(error); ok {
					return nil, v1
				} else {
					return nil, nil
				}
			}
		})

		return err
	}
}

func (c *Context) createFunctionAccessor(ctx context.Context, t reflect.Type, method reflect.Value, name string) (Getter, error) {
	methodNameParts := strings.Split(runtime.FuncForPC(method.Pointer()).Name(), ".")
	methodName := methodNameParts[len(methodNameParts)-1]
	methodReceiver := strings.Trim(methodNameParts[len(methodNameParts)-2], "(*)")
	g, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if ft, err := c.NewFunctionTemplate(ctx, func(in FunctionArgs) (*Value, error) {
			pv, err := in.Context.isolate.Sync(in.ExecutionContext, func(ctx context.Context) (interface{}, error) {
				if r, err := c.Receiver(ctx, in.Holder, t); err != nil {
					return nil, err
				} else {
					var r2 reflect.Value = r
					if r.Kind() == reflect.Pointer || r.Kind() == reflect.Interface {
						if methodReceiver != r.Type().Elem().Name() {
							r2 = r.Elem().FieldByName(methodReceiver)
						}
					} else if methodReceiver != r.Type().Name() {
						r2 = r.FieldByName(methodReceiver)
					}
					var v []reflect.Value
					if !r2.IsValid() || r2.IsNil() || r2.IsZero() {
						v = method.Call([]reflect.Value{r, reflect.ValueOf(in)})
					} else {
						m := r2.MethodByName(methodName)
						v = m.Call([]reflect.Value{reflect.ValueOf(in)})
					}

					if v1, ok := v[1].Interface().(error); ok {
						return nil, v1
					} else if v0, ok := v[0].Interface().(*Value); ok {
						return v0, nil
					} else {
						return nil, nil
					}
				}
			})

			if err != nil {
				return nil, err
			} else {
				return pv.(*Value), nil
			}
		}); err != nil {
			return nil, err
		} else if err := ft.SetName(ctx, name); err != nil {
			return nil, err
		} else if fn, err := ft.GetFunction(ctx); err != nil {
			return nil, err
		} else {
			return func(in GetterArgs) (*Value, error) {
				return fn, nil
			}, nil
		}
	})

	if err != nil {
		return nil, err
	} else {
		return g.(func(GetterArgs) (*Value, error)), nil
	}
}

func (c *Context) writePrototypeFields(ctx context.Context, v *ObjectTemplate, o *ObjectTemplate, value reflect.Value, prototype reflect.Type) error {
	_, err := c.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		getters := map[string]Getter{}
		setters := map[string]Setter{}

		if prototype.Kind() == reflect.Struct {
			for i := 0; i < prototype.NumField(); i++ {
				f := prototype.Field(i)

				name := f.Tag.Get("v8")
				if name == "" {
					continue
				}

				for value.Kind() == reflect.Ptr && !value.IsNil() {
					value = value.Elem()
				}
				t := value.Type()

				getters[name] = c.createFieldGetter(ctx, t, f.Name)
				setters[name] = c.createFieldSetter(ctx, prototype, f.Name)
			}
		}

		for i := 0; i < prototype.NumMethod(); i++ {
			name := prototype.Method(i).Name
			fn := runtime.FuncForPC(prototype.Method(i).Func.Pointer())
			file, _ := fn.FileLine(0)
			if file == "<autogenerated>" {
				continue
			}

			if !strings.HasPrefix(name, "V8") {
				continue
			}

			m := reflect.Zero(prototype).Method(i)
			method := prototype.Method(i)
			if m.Type().ConvertibleTo(getterType) {
				if strings.HasPrefix(name, "V8Get") {
					name = getName(strings.TrimPrefix(name, "V8Get"))
					getters[name] = c.createGetter(ctx, prototype, method.Func)
				}
			} else if m.Type().ConvertibleTo(setterType) {
				if strings.HasPrefix(name, "V8Set") {
					name = getName(strings.TrimPrefix(name, "V8Set"))
					setters[name] = c.createSetter(ctx, prototype, method.Func)
				}
			} else if m.Type().ConvertibleTo(functionType) {
				if strings.HasPrefix(name, "V8Func") {
					name = getName(strings.TrimPrefix(name, "V8Func"))
					if fn, err := c.createFunctionAccessor(ctx, prototype, method.Func, name); err != nil {
						return nil, err
					} else {
						o.SetAccessor(ctx, name, fn, nil)
					}
				}
			}
		}

		for name, getter := range getters {
			setter, _ := setters[name]
			v.SetAccessor(ctx, name, getter, setter)
		}

		// Also export any methods of the struct pointer that match the callback type.
		if prototype.Kind() != reflect.Ptr {
			return nil, c.writePrototypeFields(ctx, v, o, value, reflect.PtrTo(prototype))
		}

		return nil, nil
	})

	return err
}
