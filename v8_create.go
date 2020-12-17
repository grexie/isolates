package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++14
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unsafe"

	refutils "github.com/behrsin/go-refutils"
)

type Marshaler interface {
	MarshalV8() interface{}
}

type valueRef struct {
	refutils.RefHolder

	value reflect.Value
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

// https://stackoverflow.com/a/23555352
func isZero(v reflect.Value) bool {
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
	return v.Interface() == z.Interface()
}

func (c *Context) Create(v interface{}) (*Value, error) {
	rv := reflect.ValueOf(v)
	value, err := c.create(rv)
	return value, err
}

func (c *Context) createImmediateValue(v C.ImmediateValue, kinds kinds) (*Value, error) {
	if err := c.isolate.lock(); err != nil {
		return nil, err
	} else {
		defer c.isolate.unlock()
	}

	return c.newValue(C.v8_Context_Create(c.pointer, v), C.Kinds(kinds)), nil
}

func marshalValue(val reflect.Value) reflect.Value {
	if val.Type().Implements(reflect.TypeOf((*Marshaler)(nil)).Elem()) {
		m := val.MethodByName("MarshalV8")
		val = m.Call([]reflect.Value{})[0]
	} else if val.Type().Kind() != reflect.Ptr && reflect.PtrTo(val.Type()).Implements(reflect.TypeOf((*Marshaler)(nil)).Elem()) {
		m := val.Addr().MethodByName("MarshalV8")
		val = m.Call([]reflect.Value{})[0]
	}
	return val
}

func (c *Context) create(v reflect.Value) (*Value, error) {
	if !v.IsValid() {
		return c.Undefined()
	}

	if v.Type() == valueType {
		return v.Interface().(*Value), nil
	}

	v = marshalValue(v)

	if v.Type() == errorType {
		if global, err := c.Global(); err != nil {
			return nil, err
		} else if errorClass, err := global.Get("Error"); err != nil {
			return nil, err
		} else if message, err := c.Create(fmt.Sprintf("%v", v.Interface())); err != nil {
			return nil, err
		} else if errorObject, err := errorClass.New(message); err != nil {
			return nil, err
		} else {
			return errorObject, nil
		}
	}

	if v.Type() == timeType {
		msec := C.double(v.Interface().(time.Time).UnixNano()) / 1e6
		return c.createImmediateValue(C.ImmediateValue{_type: C.tDATE, _float64: msec}, unionKindDate)
	}

	switch v.Kind() {
	case reflect.Bool:
		b := C.bool(false)
		if v.Bool() {
			b = C.bool(true)
		}
		return c.createImmediateValue(C.ImmediateValue{_type: C.tBOOL, _bool: b}, mask(KindBoolean))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		n := C.double(v.Convert(float64Type).Float())
		return c.createImmediateValue(C.ImmediateValue{_type: C.tFLOAT64, _float64: n}, mask(KindNumber))
	case reflect.String:
		s := v.String()
		ps := C.ByteArray{data: C.CString(s), length: C.int(len(s))}
		defer C.free(unsafe.Pointer(ps.data))
		return c.createImmediateValue(C.ImmediateValue{_type: C.tSTRING, _data: ps}, unionKindString)
	case reflect.UnsafePointer, reflect.Uintptr:
		return nil, fmt.Errorf("uintptr not supported: %#v", v.Interface())
	case reflect.Complex64, reflect.Complex128:
		return nil, fmt.Errorf("complex not supported: %#v", v.Interface())
	case reflect.Chan:
		return nil, fmt.Errorf("chan not supported: %#v", v.Interface())
	case reflect.Func:
		if v.Type().ConvertibleTo(functionType) {
			if ft, err := c.NewFunctionTemplate(v.Convert(functionType).Interface().(Function)); err != nil {
				return nil, err
			} else {
				ft.SetName(runtime.FuncForPC(uintptr(v.Pointer())).Name())
				return ft.GetFunction()
			}
		} else if err := isConstructor(v.Type()); err == nil {
			return c.createConstructor(v.Interface())
		}
		return nil, fmt.Errorf("func not supported: %#v", v.Interface())
	case reflect.Interface, reflect.Ptr:
		return c.create(v.Elem())
	case reflect.Map:
		if v.Type().Key() != stringType {
			return nil, fmt.Errorf("map keys must be strings, %s not permissable in v8", v.Type().Key())
		}

		if o, err := c.createImmediateValue(C.ImmediateValue{_type: C.tOBJECT}, mask(KindObject)); err != nil {
			return nil, err
		} else {
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
		}
	case reflect.Struct:
		if value, ok := c.objects[uintptr(v.Addr().Pointer())]; ok {
			return value, nil
		} else if p, err := c.createPrototype(v, v.Type()); err != nil {
			return nil, err
		} else if fn, err := p.GetFunction(); err != nil {
			return nil, err
		} else if value, err := fn.New(); err != nil {
			return nil, err
		} else {
			c.objects[uintptr(v.Addr().Pointer())] = value
			value.SetReceiver(&v)
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

			return c.createImmediateValue(
				C.ImmediateValue{
					_type: C.tARRAYBUFFER,
					_data: C.ByteArray{data: pb, length: C.int(v.Len())},
				},
				unionKindArrayBuffer,
			)
		} else {
			if o, err := c.createImmediateValue(
				C.ImmediateValue{
					_type: C.tARRAY,
					_data: C.ByteArray{data: nil, length: C.int(v.Len())},
				},
				unionKindArray,
			); err != nil {
				return nil, err
			} else {
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
		return fn.GetFunction()
	} else if pfn, err := c.createPrototype(reflect.Zero(prototype), prototype); err != nil {
		return nil, err
	} else if pfnv, err := pfn.GetFunction(); err != nil {
		return nil, err
	} else if fn, err := c.NewFunctionTemplate(func(in FunctionArgs) (*Value, error) {
		pfnv.Call(in.This)
		r := cv.Call([]reflect.Value{reflect.ValueOf(in)})

		if r[1].Interface() != nil {
			return nil, r[1].Interface().(error)
		}

		in.This.SetReceiver(&r[0])
		return in.This, nil
	}); err != nil {
		return nil, err
	} else if err := fn.SetName(prototype.Name()); err != nil {
		return nil, err
	} else if instance, err := fn.GetInstanceTemplate(); err != nil {
		return nil, err
	} else if err := instance.SetInternalFieldCount(1); err != nil {
		return nil, err
	} else if proto, err := fn.GetPrototypeTemplate(); err != nil {
		return nil, err
	} else if err := c.writePrototypeFields(instance, proto, reflect.Zero(prototype), prototype); err != nil {
		return nil, err
	} else {
		c.constructors[constructor] = fn
		return fn.GetFunction()
	}
}

func (c *Context) createPrototype(v reflect.Value, prototype reflect.Type) (*FunctionTemplate, error) {
	if prototype.Kind() == reflect.Ptr || prototype.Kind() == reflect.Interface {
		prototype = prototype.Elem()
	}

	if fn, ok := c.constructors[prototype]; ok {
		return fn, nil
	} else if prototype.Kind() != reflect.Interface && prototype.Kind() != reflect.Struct {
		return nil, fmt.Errorf("prototype must be an interface: %+v", prototype)
	} else if fn, err := c.NewFunctionTemplate(func(in FunctionArgs) (*Value, error) {
		return in.This, nil
	}); err != nil {
		return nil, err
	} else if err := fn.SetName(prototype.Name()); err != nil {
		return nil, err
	} else if instance, err := fn.GetInstanceTemplate(); err != nil {
		return nil, err
	} else if err := instance.SetInternalFieldCount(1); err != nil {
		return nil, err
	} else if proto, err := fn.GetPrototypeTemplate(); err != nil {
		return nil, err
	} else if err := c.writePrototypeFields(instance, proto, v, prototype); err != nil {
		return nil, err
	} else {
		c.constructors[prototype] = fn
		return fn, nil
	}
}

func (c *Context) createFieldGetter(t reflect.Type, name string) Getter {
	return func(in GetterArgs) (*Value, error) {
		r := in.Holder.Receiver(t)

		if r == nil {
			return nil, fmt.Errorf("receiver not found")
		}

		for r.Kind() == reflect.Ptr && !r.IsNil() {
			v := r.Elem()
			r = &v
		}

		fval := r.FieldByName(name)
		if v, err := in.Context.create(fval); err != nil {
			return nil, fmt.Errorf("field %q: %v", name, err)
		} else {
			return v, nil
		}
	}
}

func (c *Context) createFieldSetter(t reflect.Type, name string) Setter {
	return func(in SetterArgs) error {
		r := in.Holder.Receiver(t)

		if r == nil {
			return fmt.Errorf("receiver not found")
		} else if r.Kind() == reflect.Ptr {
			v := r.Elem()
			r = &v
		}

		fval := r.FieldByName(name)
		if v, err := in.Value.Unmarshal(fval.Type()); err != nil {
			return err
		} else {
			fval.Set(*v)
			return nil
		}
	}
}

func (c *Context) createGetter(t reflect.Type, method reflect.Value) Getter {
	return func(in GetterArgs) (*Value, error) {
		r := in.Holder.Receiver(t)

		if r == nil {
			return nil, fmt.Errorf("receiver not found")
		}

		v := method.Call([]reflect.Value{*r, reflect.ValueOf(in)})

		if v1, ok := v[1].Interface().(error); ok {
			return nil, v1
		} else if v0, ok := v[0].Interface().(*Value); ok {
			return v0, nil
		} else {
			return nil, nil
		}
	}
}

func (c *Context) createSetter(t reflect.Type, method reflect.Value) Setter {
	return func(in SetterArgs) error {
		r := in.Holder.Receiver(t)

		if r == nil {
			return fmt.Errorf("receiver not found")
		}

		v := method.Call([]reflect.Value{*r, reflect.ValueOf(in)})

		if v1, ok := v[1].Interface().(error); ok {
			return v1
		} else {
			return nil
		}
	}
}

func (c *Context) createFunctionAccessor(t reflect.Type, method reflect.Value, name string) (Getter, error) {
	if ft, err := c.NewFunctionTemplate(func(in FunctionArgs) (*Value, error) {
		r := in.Holder.Receiver(t)

		if r == nil {
			return nil, fmt.Errorf("receiver not found")
		}

		v := method.Call([]reflect.Value{*r, reflect.ValueOf(in)})

		if v1, ok := v[1].Interface().(error); ok {
			return nil, v1
		} else if v0, ok := v[0].Interface().(*Value); ok {
			return v0, nil
		} else {
			return nil, nil
		}
	}); err != nil {
		return nil, err
	} else if err := ft.SetName(name); err != nil {
		return nil, err
	} else if fn, err := ft.GetFunction(); err != nil {
		return nil, err
	} else {
		return func(in GetterArgs) (*Value, error) {
			return fn, nil
		}, nil
	}
}

func (c *Context) writePrototypeFields(v *ObjectTemplate, o *ObjectTemplate, value reflect.Value, prototype reflect.Type) error {
	getters := map[string]Getter{}
	setters := map[string]Setter{}

	if prototype.Kind() != reflect.Ptr {
		for i := 0; i < prototype.NumField(); i++ {
			f := prototype.Field(i)

			// Inline embedded fields.
			if f.Anonymous {
				sub := value.Field(i)
				for sub.Kind() == reflect.Ptr && !sub.IsNil() {
					sub = sub.Elem()
				}

				if sub.Kind() == reflect.Struct {
					if err := c.writePrototypeFields(v, o, value, sub.Type()); err != nil {
						return fmt.Errorf("Writing embedded field %q: %v", f.Name, err)
					}
					continue
				}
			}

			name := f.Tag.Get("v8")
			if name == "" {
				continue
			}

			for value.Kind() == reflect.Ptr && !value.IsNil() {
				value = value.Elem()
			}
			t := value.Type()

			getters[name] = c.createFieldGetter(t, f.Name)
			setters[name] = c.createFieldSetter(prototype, f.Name)
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
				getters[name] = c.createGetter(prototype, method.Func)
			}
		} else if m.Type().ConvertibleTo(setterType) {
			if strings.HasPrefix(name, "V8Set") {
				name = getName(strings.TrimPrefix(name, "V8Set"))
				setters[name] = c.createSetter(prototype, method.Func)
			}
		} else if m.Type().ConvertibleTo(functionType) {
			if strings.HasPrefix(name, "V8Func") {
				name = getName(strings.TrimPrefix(name, "V8Func"))
				if fn, err := c.createFunctionAccessor(prototype, method.Func, name); err != nil {
					return err
				} else {
					o.SetAccessor(name, fn, nil)
				}
			}
		}
	}

	for name, getter := range getters {
		setter, _ := setters[name]
		v.SetAccessor(name, getter, setter)
	}

	// Also export any methods of the struct pointer that match the callback type.
	if prototype.Kind() != reflect.Ptr {
		return c.writePrototypeFields(v, o, value, reflect.PtrTo(prototype))
	}

	return nil
}
