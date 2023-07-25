package isolates

import (
	"context"
	"errors"
)

var ErrNoContext = errors.New("isolates.ExecutionContext: no context available")

func (c *ExecutionContext) New(constructor any, args ...any) (any, error) {
	if c.context == nil {
		return nil, ErrNoContext
	}

	return c.context.New(c.ctx, constructor, args...)
}

func (c *ExecutionContext) Background(callback func(context.Context)) {
	if c.isolate == nil {
		panic("isolate not found on context: is this an ExecutionContext?")
	}

	c.isolate.Background(c.ctx, callback)
}
