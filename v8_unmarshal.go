package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"fmt"
	"reflect"
	"unsafe"
)

func (v *Value) Unmarshal(ctx context.Context, t reflect.Type) (*reflect.Value, error) {
	rv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
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
			if reflect.TypeOf([]byte{}).ConvertibleTo(t) {
				if bytes, err := v.Bytes(ctx); err != nil {
					return nil, err
				} else {
					v := reflect.ValueOf(bytes).Convert(t)
					return &v, nil
				}
			}

			if lengthV, err := v.Get(ctx, "length"); err != nil {
				return nil, err
			} else if length, err := lengthV.Int64(ctx); err != nil {
				return nil, err
			} else {
				rv := reflect.MakeSlice(t, int(length), int(length))
				for i := 0; int64(i) < length; i++ {
					if itemV, err := v.GetIndex(ctx, i); err != nil {
						return nil, err
					} else if itemR, err := itemV.Unmarshal(ctx, t.Elem()); err != nil {
						return nil, err
					} else {
						rv.Index(i).Set(*itemR)
					}
				}
				return &rv, nil
			}
		case reflect.Func:
		case reflect.Ptr, reflect.Interface:
			return v.Receiver(ctx, t)
		case reflect.Map:
		case reflect.String:
			if string, err := v.String(ctx); err != nil {
				return nil, err
			} else {
				v := reflect.ValueOf(string).Convert(t)
				return &v, nil
			}
		case reflect.Struct:
			break
		case reflect.UnsafePointer:
			if string, err := v.String(ctx); err != nil {
				return nil, err
			} else {
				var u unsafe.Pointer
				fmt.Sscanf(string, "%p", &u)
				v := reflect.ValueOf(u).Convert(t)
				return &v, nil
			}
		}

		panic(fmt.Sprintf("unsupported kind: %v", t.Kind()))
	})

	if err != nil {
		return nil, err
	} else {
		return rv.(*reflect.Value), nil
	}
}
