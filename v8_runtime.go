package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"fmt"
	"io"
	"sync"
)

type Module struct {
	sync.Mutex

	Exports *Value
	Error   error
}

type Stats interface {
	IsFile() bool
	IsDirectory() bool
	Modified() int64
	Size() int64
}

type DirEntry interface {
	IsFile() bool
	IsDirectory() bool
	Name() string
}

type File interface {
	Close() error
	Reader() (io.ReadCloser, error)
	ReadAll() ([]byte, error)
}

type FS interface {
	ReadDir(path string) ([]DirEntry, error)
	ReadFile(path string) ([]byte, error)
	Open(path string) (File, error)
	Stat(path string) (Stats, error)
}

type RuntimeFunctionArgs struct {
	FunctionArgs
	*Module
	Require *Value
}

var registeredRuntimes = make(map[string][]*RuntimeModule)

type RuntimeModule struct {
	factory func(in FunctionArgs) (*Value, error)
	fs      FS
	path    string
}

func RegisterRuntime(name string, factory func(in FunctionArgs) (*Value, error)) *RuntimeModule {
	m := &RuntimeModule{
		factory: factory,
	}

	if _, ok := registeredRuntimes[name]; !ok {
		registeredRuntimes[name] = []*RuntimeModule{}
	}

	registeredRuntimes[name] = append(registeredRuntimes[name], m)

	return m
}

func RegisterRuntimeLibrary(name string, fs FS, path string) *RuntimeModule {
	m := &RuntimeModule{
		fs:   fs,
		path: path,
	}

	if _, ok := registeredRuntimes[name]; !ok {
		registeredRuntimes[name] = []*RuntimeModule{}
	}

	registeredRuntimes[name] = append(registeredRuntimes[name], m)

	return m
}

func wrapRuntimeScript(bytes []byte) string {
	return "(function (module, exports, require) {\n" + string(bytes) + "\n})"
}

func wrapRuntimeRunnerScript(bytes []byte) string {
	return "(function (require) {\n" + string(bytes) + "\n})"
}

func (c *Context) CreateRequire(ctx context.Context, runtimes map[string]bool) (*Value, error) {
	var mutex sync.Mutex
	var modules = map[string]*Module{}

	var require *Value
	var err error

	if require, err = c.Create(ctx, func(in FunctionArgs) (*Value, error) {
		mutex.Lock()
		if id, err := in.Arg(in.ExecutionContext, 0).String(in.ExecutionContext); err != nil {
			mutex.Unlock()
			return nil, err
		} else if runtimeAllowed, ok := runtimes[id]; ok && runtimeAllowed != false {
			mutex.Unlock()
			return nil, fmt.Errorf("module " + id + " not found")
		} else if module, ok := modules[id]; ok {
			module.Lock()
			defer module.Unlock()
			mutex.Unlock()
			return module.Exports, module.Error
		} else if runtimes, ok := registeredRuntimes[id]; !ok {
			mutex.Unlock()
			return nil, fmt.Errorf("module " + id + " not found")
		} else if exports, err := in.Context.Create(in.ExecutionContext, &map[string]interface{}{}); err != nil {
			mutex.Unlock()
			return nil, err
		} else {
			module := &Module{
				Exports: exports,
			}
			modules[id] = module
			module.Lock()
			defer module.Unlock()
			mutex.Unlock()

			if moduleValue, err := in.Context.Create(in.ExecutionContext, module); err != nil {
				return nil, err
			} else {
				for _, runtime := range runtimes {
					if runtime.factory != nil {
						if factory, err := in.Context.Create(in.ExecutionContext, runtime.factory); err != nil {
							module.Error = err
							break
						} else if _, err := factory.Call(in.ExecutionContext, in.Context.global, moduleValue, module.Exports, require); err != nil {
							module.Error = err
							break
						}
					} else if runtime.fs != nil {
						if bytes, err := runtime.fs.ReadFile(runtime.path); err != nil {
							module.Error = err
							break
						} else if value, err := in.Context.Run(in.ExecutionContext, wrapRuntimeScript(bytes), runtime.path); err != nil {
							module.Error = err
							break
						} else if _, err := value.Call(in.ExecutionContext, in.Context.global, moduleValue, module.Exports, require); err != nil {
							module.Error = err
							break
						}
					}
				}

				if global, err := in.Context.Global(in.ExecutionContext); err != nil {
					module.Error = err
				} else if Object, err := global.Get(in.ExecutionContext, "Object"); err != nil {
					module.Error = err
				} else if assign, err := Object.Get(in.ExecutionContext, "assign"); err != nil {
					module.Error = err
				} else if _default, err := module.Exports.Get(in.ExecutionContext, "default"); err != nil {
					module.Error = err
				} else if !_default.IsKind(KindUndefined) {
					fmt.Println("module", id, "has default")
					if err := moduleValue.Set(in.ExecutionContext, "exports", _default); err != nil {
						module.Error = err
					} else if _, err := assign.Call(in.ExecutionContext, Object, _default, exports); err != nil {
						module.Error = err
					}
				}

				return module.Exports, module.Error
			}
		}
	}); err != nil {
		return nil, err
	} else {
		return require, nil
	}
}

func (c *Context) RunWithRuntime(ctx context.Context, script string, path string, env func(require *Value) error, globals string, security map[string]bool) (*Value, error) {
	if require, err := c.CreateRequire(ctx, security); err != nil {
		return nil, err
	} else if err := env(require); err != nil {
		return nil, err
	} else {
		if globals != "" {
			if globalsV, err := c.Create(ctx, globals); err != nil {
				return nil, err
			} else if _, err := require.Call(ctx, nil, globalsV); err != nil {
				return nil, err
			}
		}

		if value, err := c.Run(ctx, wrapRuntimeRunnerScript([]byte(script)), path); err != nil {
			return nil, err
		} else if value, err := value.Call(ctx, c.global, require); err != nil {
			return nil, err
		} else {
			return value, nil
		}
	}
}

func (m *Module) V8GetExports(in GetterArgs) (*Value, error) {
	return m.Exports, m.Error
}

func (m *Module) V8SetExports(in SetterArgs) error {
	m.Exports = in.Value
	return nil
}
