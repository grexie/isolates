package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"time"
	"unsafe"
)

func (v *Value) IsError(ctx context.Context, err error) bool {
	if goerrrv, _err := v.Unmarshal(ctx, errorType); _err != nil {
		return false
	} else {
		goerr := goerrrv.Interface().(error)
		return errors.Is(err, goerr)
	}
}

func IsError(ctx context.Context, err error, target error) bool {
	if v, ok := target.(*Value); ok {
		return v.IsError(ctx, err)
	} else {
		return errors.Is(err, target)
	}
}

func (v *Value) Unmarshal(ctx context.Context, t reflect.Type) (*reflect.Value, error) {
	rv, err := v.context.isolate.Sync(ctx, func(ctx context.Context) (interface{}, error) {
		if t == valueType || t == anyType {
			v := reflect.ValueOf(v)
			return &v, nil
		}

		if t == errorType {
			if v.Receiver(ctx).CanConvert(errorType) {
				rv := v.Receiver(ctx)
				log.Println(rv)
				return &rv, nil
			}
			if s, err := v.StringValue(ctx); err != nil {
				return nil, err
			} else {
				rv := reflect.ValueOf(errors.New(s))
				return &rv, nil
			}
		}

		if t == timeType {
			if msec, err := v.Int64(ctx); err != nil {
				return nil, err
			} else {
				rv := reflect.ValueOf(time.UnixMilli(msec))
				return &rv, nil
			}
		}

		if t == durationType {
			if msec, err := v.Float64(ctx); err != nil {
				return nil, err
			} else {
				rv := reflect.ValueOf(time.Duration(msec * float64(time.Millisecond)))
				return &rv, nil
			}
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
			r := v.Receiver(ctx)

			if r.Kind() != reflect.Pointer && r.CanAddr() {
				r = r.Addr()
			}

			return &r, nil
		case reflect.Map:
			if keys, err := v.Keys(ctx); err != nil {
				return nil, err
			} else {
				rv := reflect.MakeMap(t)
				for _, k := range keys {
					if itemV, err := v.Get(ctx, k); err != nil {
						return nil, err
					} else if itemR, err := itemV.Unmarshal(ctx, t.Elem()); err != nil {
						return nil, err
					} else {
						rv.SetMapIndex(reflect.ValueOf(k), *itemR)
					}
				}
				return &rv, nil
			}
		case reflect.String:
			if string, err := v.StringValue(ctx); err != nil {
				return nil, err
			} else {
				v := reflect.ValueOf(string).Convert(t)
				return &v, nil
			}
		case reflect.Struct:
			if rv := v.Receiver(ctx); !isZero(rv) {
				if rv.Kind() == reflect.Pointer {
					rv = rv.Elem()
				}

				return &rv, nil
			} else {
				rv := reflect.New(t).Elem()

				for i := 0; i < t.NumField(); i++ {
					f := t.Field(i)

					name := f.Tag.Get("v8")
					if name == "" {
						continue
					}

					ft := f.Type

					if ft.Kind() == reflect.Pointer && ft.Elem().Kind() != reflect.Struct {
						ft = ft.Elem()
					}

					if valuev, err := v.Get(ctx, name); err != nil {
						return nil, err
					} else if valuerv, err := valuev.Unmarshal(ctx, ft); err != nil {
						return nil, err
					} else if !isZero(*valuerv) {
						if f.Type.Kind() == reflect.Pointer && ft.Kind() != reflect.Pointer {
							valuervp := reflect.New(ft)
							valuervp.Elem().Set(*valuerv)
							valuerv = &valuervp
						}

						rv.FieldByIndex(f.Index).Set(*valuerv)
					}
				}

				if method, ok := reflect.PointerTo(t).MethodByName("V8Construct"); ok {
					v.SetReceiver(ctx, rv)

					mrv := rv

					if method.Type.In(0).Kind() == reflect.Pointer && rv.Kind() != reflect.Pointer {
						mrv = mrv.Addr()
					} else if method.Type.In(0).Kind() != reflect.Pointer && rv.Kind() == reflect.Pointer {
						mrv = mrv.Elem()
					}

					method.Func.Call([]reflect.Value{mrv, reflect.ValueOf(FunctionArgs{
						ExecutionContext: ctx,
						Context:          v.context,
						This:             v,
						IsConstructCall:  true,
						Args:             []*Value{},
						Holder:           v,
					})})
				}

				return &rv, nil
			}
		case reflect.UnsafePointer:
			if string, err := v.StringValue(ctx); err != nil {
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
