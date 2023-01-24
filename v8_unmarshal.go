package isolates

// #include "v8_c_bridge.h"
// #cgo CXXFLAGS: -I${SRCDIR} -I${SRCDIR}/include -g3 -fno-rtti -fpic -std=c++11
// #cgo LDFLAGS: -pthread -L${SRCDIR}/libv8 -lv8_base -lv8_init -lv8_initializers -lv8_libbase -lv8_libplatform -lv8_libsampler -lv8_nosnapshot
import "C"

import (
	"context"
	"fmt"
	"reflect"
)

func (v *Value) Unmarshal(ctx context.Context, t reflect.Type) (*reflect.Value, error) {
	if t == valueType {
		v := reflect.ValueOf(v)
		return &v, nil
	}

	switch t.Kind() {
	case reflect.Bool:
		if value, err := v.Bool(ctx); err != nil {
			return nil, err
		} else {
			v := reflect.ValueOf(value).Convert(t)
			return &v, nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value, err := v.Int64(ctx); err != nil {
			return nil, err
		} else {
			v := reflect.ValueOf(value).Convert(t)
			return &v, nil
		}
	case reflect.Float32, reflect.Float64:
		if value, err := v.Float64(ctx); err != nil {
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
		if string, err := v.String(ctx); err != nil {
			return nil, err
		} else {
			v := reflect.ValueOf(string).Convert(t)
			return &v, nil
		}
	case reflect.Struct:
	}

	panic(fmt.Sprintf("unsupported kind: %v", t.Kind()))
}
