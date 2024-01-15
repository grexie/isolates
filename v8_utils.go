package isolates

import (
	"context"
	"errors"
	"fmt"
	"os"
)

type Console interface {
	Assert(args ...any)
	Clear()
	Count(args ...any)
	CountReset(args ...any)
	Debug(args ...any)
	Dir(args ...any)
	DirXML(args ...any)
	Error(args ...any)
	Group(args ...any)
	GroupCollapsed(args ...any)
	GroupEnd(args ...any)
	Info(args ...any)
	Log(args ...any)
	Table(args ...any)
	Time(args ...any)
	TimeEnd(args ...any)
	TimeLog(args ...any)
	Trace(args ...any)
	Warn(args ...any)
	Profile(args ...any)
	ProfileEnd(args ...any)
	TimeStamp(args ...any)
}

var _ Console = &ExecutionContext{}

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

func (c *ExecutionContext) Sync(callback func(context.Context) (any, error)) (any, error) {
	return c.isolate.Sync(c.ctx, callback)
}

func (c *ExecutionContext) Data(key any) (any, bool) {
	if c.context == nil {
		return nil, false
	}

	return c.context.Data(key)
}

func (c *ExecutionContext) SetData(key any, value any) {
	if c.context == nil {
		return
	}

	c.context.SetData(key, value)
}

// Assert implements Console.
func (c *ExecutionContext) Assert(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "assert", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Clear implements Console.
func (c *ExecutionContext) Clear() {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "clear"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Count implements Console.
func (c *ExecutionContext) Count(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "count", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// CountReset implements Console.
func (c *ExecutionContext) CountReset(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "countReset", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Debug implements Console.
func (c *ExecutionContext) Debug(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "debug", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Dir implements Console.
func (c *ExecutionContext) Dir(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "dir", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// DirXML implements Console.
func (c *ExecutionContext) DirXML(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "dirXML", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Error implements Console.
func (c *ExecutionContext) Error(args ...any) {
	if c.context == nil {
		fmt.Println(args...)
		return
	}
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "error", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Group implements Console.
func (c *ExecutionContext) Group(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "group", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// GroupCollapsed implements Console.
func (c *ExecutionContext) GroupCollapsed(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "groupCollapsed", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// GroupEnd implements Console.
func (c *ExecutionContext) GroupEnd(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "groupEnd", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Info implements Console.
func (c *ExecutionContext) Info(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "info", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Log implements Console.
func (c *ExecutionContext) Log(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "log", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Profile implements Console.
func (c *ExecutionContext) Profile(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "profile", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// ProfileEnd implements Console.
func (c *ExecutionContext) ProfileEnd(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "profileEnd", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Table implements Console.
func (c *ExecutionContext) Table(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "table", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Time implements Console.
func (c *ExecutionContext) Time(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "time", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// TimeEnd implements Console.
func (c *ExecutionContext) TimeEnd(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "timeEnd", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// TimeLog implements Console.
func (c *ExecutionContext) TimeLog(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "timeLog", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// TimeStamp implements Console.
func (c *ExecutionContext) TimeStamp(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "timeStamp", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Trace implements Console.
func (c *ExecutionContext) Trace(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "trace", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// Warn implements Console.
func (c *ExecutionContext) Warn(args ...any) {
	if global, err := c.context.Global(c.ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if console, err := global.Get(c.ctx, "console"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else if _, err := console.CallMethod(c.ctx, "warn", args...); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
