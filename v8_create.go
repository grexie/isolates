package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fpic -std=c++11
import "C"

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
	"unsafe"
)

type Marshaler interface {
	MarshalV8() interface{}
}

type valueRef struct {
	value reflect.Value
	id    ID
}

func (r *valueRef) GetID() ID {
	return r.id
}

func (r *valueRef) SetID(id ID) {
	r.id = id
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
var valueType = reflect.TypeOf((*Value)(nil))
var marshalerType = reflect.TypeOf((*Marshaler)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()
var functionArgsType = reflect.TypeOf((*FunctionArgs)(nil)).Elem()

func (c *Context) Create(v interface{}) (*Value, error) {
	rv := reflect.ValueOf(v)
	if rv.Type() == valueType {
		return v.(*Value), nil
	}
	return c.create(rv)
}

func (c *Context) createImmediateValue(v C.ImmediateValue, kinds kinds) *Value {
	return c.newValue(C.v8_Context_Create(c.pointer, v), C.Kinds(kinds))
}

func marshalValue(v reflect.Value) reflect.Value {
	// if v.Kind() == reflect.Struct {
	// 	return marshalValue(v.Addr())
	// }

	if v.Type().Implements(marshalerType) {
		m := v.MethodByName("MarshalV8")
		v = m.Call([]reflect.Value{})[0]
	}

	return v
}

func (c *Context) create(v reflect.Value) (*Value, error) {
	if !v.IsValid() {
		return c.Undefined(), nil
	}

	v = marshalValue(v)

	if v.Type() == timeType {
		msec := C.double(v.Interface().(time.Time).UnixNano()) / 1e6
		return c.createImmediateValue(C.ImmediateValue{_type: C.tDATE, _float64: msec}, unionKindDate), nil
	}

	switch v.Kind() {
	case reflect.Bool:
		b := C.bool(false)
		if v.Bool() {
			b = C.bool(true)
		}
		return c.createImmediateValue(C.ImmediateValue{_type: C.tBOOL, _bool: b}, mask(KindBoolean)), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		n := C.double(v.Convert(float64Type).Float())
		return c.createImmediateValue(C.ImmediateValue{_type: C.tFLOAT64, _float64: n}, mask(KindNumber)), nil
	case reflect.String:
		s := v.String()
		ps := C.ByteArray{data: C.CString(s), length: C.int(len(s))}
		defer C.free(unsafe.Pointer(ps.data))
		return c.createImmediateValue(C.ImmediateValue{_type: C.tSTRING, _data: ps}, unionKindString), nil
	case reflect.UnsafePointer, reflect.Uintptr:
		return nil, fmt.Errorf("uintptr not supported: %#v", v.Interface())
	case reflect.Complex64, reflect.Complex128:
		return nil, fmt.Errorf("complex not supported: %#v", v.Interface())
	case reflect.Chan:
		return nil, fmt.Errorf("chan not supported: %#v", v.Interface())
	case reflect.Func:
		if v.Type().ConvertibleTo(functionType) {
			return c.NewFunctionTemplate(v.Convert(functionType).Interface().(Function)).GetFunction(), nil
		} else if err := isConstructor(v.Type()); err == nil {
			constructor, err := c.createConstructor(v.Interface())
			return constructor, err
		}
		return nil, fmt.Errorf("func not supported: %#v", v.Interface())
	case reflect.Interface, reflect.Ptr:
		if p, err := c.createPrototype(v.Type()); err != nil {
			return nil, err
		} else if value, err := p.GetFunction().New(); err != nil {
			return nil, err
		} else {
			id := c.values.Ref(&valueRef{v, 0})
			idv, _ := c.Create(id)
			value.SetInternalField(0, idv)
			return value, nil
		}
	case reflect.Map:
		if v.Type().Key() != stringType {
			return nil, fmt.Errorf("map keys must be strings, %s not permissable in v8", v.Type().Key())
		}

		o := c.createImmediateValue(C.ImmediateValue{_type: C.tOBJECT}, mask(KindObject))

		keys := v.MapKeys()
		sort.Sort(stringKeys(keys))
		for _, k := range keys {
			if vk, err := c.create(v.MapIndex(k)); err != nil {
				return nil, fmt.Errorf("map key %q: %v", k.String(), err)
			} else if err := o.Set(k.String(), vk); err != nil {
				return nil, err
			} else {
				vk.release()
			}
		}

		return o, nil
	case reflect.Struct:
		// return nil, fmt.Errorf("struct not supported: %#v", v.Addr().Interface())
		if p, err := c.createPrototype(v.Type()); err != nil {
			return nil, err
		} else if value, err := p.GetFunction().New(); err != nil {
			return nil, err
		} else {
			id := c.values.Ref(&valueRef{v, 0})
			idv, _ := c.Create(id)
			value.SetInternalField(0, idv)
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

			o := c.createImmediateValue(
				C.ImmediateValue{
					_type: C.tARRAYBUFFER,
					_data: C.ByteArray{data: pb, length: C.int(v.Len())},
				},
				unionKindArrayBuffer,
			)

			return o, nil
		} else {
			o := c.createImmediateValue(
				C.ImmediateValue{
					_type: C.tARRAY,
					_data: C.ByteArray{data: nil, length: C.int(v.Len())},
				},
				unionKindArray,
			)

			for i := 0; i < v.Len(); i++ {
				if v, err := c.create(v.Index(i)); err != nil {
					return nil, fmt.Errorf("index %d: %v", i, err)
				} else if err := o.SetIndex(i, v); err != nil {
					return nil, err
				} else {
					v.release()
				}
			}

			return o, nil
		}
	}

	panic(fmt.Sprintf("unsupported kind: %v", v.Kind()))
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

func (c *Context) createConstructor(cons interface{}) (*Value, error) {
	cv := reflect.ValueOf(cons)
	constructor := cv.Type()
	prototype := constructor.Out(0)

	if prototype.Kind() == reflect.Ptr || prototype.Kind() == reflect.Interface {
		prototype = prototype.Elem()
	}

	if fn, ok := c.constructors[constructor]; ok {
		return fn.GetFunction(), nil
	} else if pfn, err := c.createPrototype(prototype); err != nil {
		return nil, err
	} else {
		pfnv := pfn.GetFunction()
		fn := c.NewFunctionTemplate(func(in FunctionArgs) (*Value, error) {
			pfnv.Call(in.This)
			r := cv.Call([]reflect.Value{reflect.ValueOf(in)})

			if r[1].Interface() != nil {
				return nil, r[1].Interface().(error)
			}

			in.This.SetReceiver(&r[0])
			return in.This, nil
		})
		fn.SetName(prototype.Name())
		fn.GetInstanceTemplate().SetInternalFieldCount(1)

		if err := c.writePrototypeFields(fn.GetInstanceTemplate(), fn.GetPrototypeTemplate(), prototype); err != nil {
			return nil, err
		}

		c.constructors[constructor] = fn
		return fn.GetFunction(), nil
	}
}

func (c *Context) createPrototype(prototype reflect.Type) (*FunctionTemplate, error) {
	if prototype.Kind() == reflect.Ptr || prototype.Kind() == reflect.Interface {
		prototype = prototype.Elem()
	}

	if fn, ok := c.constructors[prototype]; ok {
		return fn, nil
	} else {
		if prototype.Kind() != reflect.Interface && prototype.Kind() != reflect.Struct {
			return nil, fmt.Errorf("prototype must be an interface")
		}

		fn := c.NewFunctionTemplate(func(in FunctionArgs) (*Value, error) {
			return in.This, nil
		})
		fn.SetName(prototype.Name())
		fn.GetInstanceTemplate().SetInternalFieldCount(1)

		if err := c.writePrototypeFields(fn.GetInstanceTemplate(), fn.GetPrototypeTemplate(), prototype); err != nil {
			return nil, err
		}

		c.constructors[prototype] = fn
		return fn, nil
	}
}

func (c *Context) writePrototypeFields(v *ObjectTemplate, o *ObjectTemplate, prototype reflect.Type) error {
	getters := map[string]Getter{}
	setters := map[string]Setter{}

	if prototype.Kind() != reflect.Ptr {
		for i := 0; i < prototype.NumField(); i++ {
			f := prototype.Field(i)

			// Inline embedded fields.
			if f.Anonymous {
				sub := reflect.Zero(prototype).Field(i)
				for sub.Kind() == reflect.Ptr && !sub.IsNil() {
					sub = sub.Elem()
				}

				if sub.Kind() == reflect.Struct {
					if err := c.writePrototypeFields(v, o, sub.Type()); err != nil {
						return fmt.Errorf("Writing embedded field %q: %v", f.Name, err)
					}
					continue
				}
			}

			name := f.Tag.Get("v8")
			if name == "" {
				continue
			}

			func(f reflect.StructField, i int) {
				getters[name] = func(in GetterArgs) (*Value, error) {
					r := in.Holder.Receiver(prototype)

					if r == nil {
						return nil, fmt.Errorf("receiver not found")
					} else if r.Kind() == reflect.Ptr {
						v := r.Elem()
						r = &v
					}

					fval := r.FieldByName(f.Name)
					if v, err := c.create(fval); err != nil {
						return nil, fmt.Errorf("field %q: %v", f.Name, err)
					} else {
						return v, nil
					}
				}
				setters[name] = func(in SetterArgs) error {
					r := in.Holder.Receiver(prototype)

					if r == nil {
						return fmt.Errorf("receiver not found")
					} else if r.Kind() == reflect.Ptr {
						v := r.Elem()
						r = &v
					}

					fval := r.FieldByName(f.Name)
					if v := in.Value.Unmarshal(fval.Type()); v == nil {
						return nil
					} else {
						fval.Set(*v)
						return nil
					}
				}
			}(f, i)
		}
	}

	for i := 0; i < prototype.NumMethod(); i++ {
		name := prototype.Method(i).Name
		if !strings.HasPrefix(name, "V8") {
			continue
		}

		m := reflect.Zero(prototype).Method(i)
		method := prototype.Method(i)
		if m.Type().ConvertibleTo(getterType) {
			if strings.HasPrefix(name, "V8Get") {
				name = getName(strings.TrimPrefix(name, "V8Get"))
				getters[name] = func(in GetterArgs) (*Value, error) {
					r := in.Holder.Receiver(prototype)

					if r == nil {
						return nil, fmt.Errorf("receiver not found")
					}

					v := method.Func.Call([]reflect.Value{*r, reflect.ValueOf(in)})

					if v1, ok := v[1].Interface().(error); ok {
						return nil, v1
					} else if v0, ok := v[0].Interface().(*Value); ok {
						return v0, nil
					} else {
						return nil, nil
					}
				}
			}
		} else if m.Type().ConvertibleTo(setterType) {
			if strings.HasPrefix(name, "V8Set") {
				name = getName(strings.TrimPrefix(name, "V8Set"))
				setters[name] = func(in SetterArgs) error {
					r := in.Holder.Receiver(prototype)

					if r == nil {
						return fmt.Errorf("receiver not found")
					}

					v := method.Func.Call([]reflect.Value{*r, reflect.ValueOf(in)})

					if v1, ok := v[1].Interface().(error); ok {
						return v1
					} else {
						return nil
					}
				}
			}
		} else if m.Type().ConvertibleTo(functionType) {
			if strings.HasPrefix(name, "V8Func") {
				name = getName(strings.TrimPrefix(name, "V8Func"))
				(func(name string, method reflect.Method) {
					fn := c.NewFunctionTemplate(func(in FunctionArgs) (*Value, error) {
						r := in.Holder.Receiver(prototype)

						if r == nil {
							return nil, fmt.Errorf("receiver not found")
						}

						v := method.Func.Call([]reflect.Value{*r, reflect.ValueOf(in)})

						if v1, ok := v[1].Interface().(error); ok {
							return nil, v1
						} else if v0, ok := v[0].Interface().(*Value); ok {
							return v0, nil
						} else {
							return nil, nil
						}
					}).GetFunction()
					o.SetAccessor(name, func(in GetterArgs) (*Value, error) {
						return fn, nil
					}, nil)
				})(name, method)
			}
		}
	}

	for name, getter := range getters {
		setter, _ := setters[name]
		v.SetAccessor(name, getter, setter)
	}

	// Also export any methods of the struct pointer that match the callback type.
	if prototype.Kind() != reflect.Ptr {
		return c.writePrototypeFields(v, o, reflect.PtrTo(prototype))
	}

	return nil
}
