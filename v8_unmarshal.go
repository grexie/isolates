package v8

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++14
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"fmt"
	"reflect"
)

func (v *Value) Unmarshal(t reflect.Type) (*reflect.Value, error) {
	if t == valueType {
		v := reflect.ValueOf(v)
		return &v, nil
	}

	switch t.Kind() {
	case reflect.Bool:
		if value, err := v.Bool(); err != nil {
			return nil, err
		} else {
			v := reflect.ValueOf(value).Convert(t)
			return &v, nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value, err := v.Int64(); err != nil {
			return nil, err
		} else {
			v := reflect.ValueOf(value).Convert(t)
			return &v, nil
		}
	case reflect.Float32, reflect.Float64:
		if value, err := v.Float64(); err != nil {
			return nil, err
		} else {
			v := reflect.ValueOf(value).Convert(t)
			return &v, nil
		}
	case reflect.Array, reflect.Slice:
	case reflect.Func:
	case reflect.Ptr, reflect.Interface:
	case reflect.Map:
	case reflect.String:
		v := reflect.ValueOf(v.String()).Convert(t)
		return &v, nil
	case reflect.Struct:
	}

	panic(fmt.Sprintf("unsupported kind: %v", t.Kind()))
}
