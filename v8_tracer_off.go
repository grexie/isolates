//go:build !v8_tracer

package isolates

type _tracer struct{}

var tracer = &_tracer{}

func (*_tracer) Retain(any)  {}
func (*_tracer) Release(any) {}
