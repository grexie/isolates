package isolates

//#include "v8_c_bridge.h"
//#cgo CXXFLAGS: -I/usr/local/include/v8 -std=c++17
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_path "path"
	"reflect"
	"regexp"
	"strings"

	refutils "github.com/grexie/refutils"
)

var ErrNotResolved = errors.New("unable to resolve")

type Module struct {
	refutils.RefHolder

	Context  *Context
	ID       string
	FS       any
	Filename string
	Dirname  string
	Main     bool

	Require *Value
	Error   error
}

type RuntimeFunctionArgs struct {
	FunctionArgs
	Module  *Module
	Require *Value
	Exports *Value
}

var registeredRuntimes = make(map[string][]*ResolveResult)

type ResolveResult struct {
	factory func(in FunctionArgs) (*Value, error)
	fs      any
	path    string
}

func RegisterRuntime(name string, path string, factory func(in FunctionArgs) (*Value, error)) *ResolveResult {
	m := &ResolveResult{
		factory: factory,
		path:    path,
	}

	if _, ok := registeredRuntimes[name]; !ok {
		registeredRuntimes[name] = []*ResolveResult{}
	}

	registeredRuntimes[name] = append(registeredRuntimes[name], m)

	return m
}

var _ = RegisterRuntime("module", "module", func(in FunctionArgs) (*Value, error) {
	if constructor, err := in.Context.CreateWithName(in.ExecutionContext, "Module", func(in FunctionArgs) (*Module, error) {
		return &Module{}, nil
	}); err != nil {
		return nil, err
	} else if err := in.Args[1].Set(in.ExecutionContext, "Module", constructor); err != nil {
		return nil, err
	} else if preloadModules, err := in.Context.CreateWithName(in.ExecutionContext, "_preloadModules", func(in FunctionArgs) (*Value, error) {
		return nil, nil
	}); err != nil {
		return nil, err
	} else if err := constructor.Set(in.ExecutionContext, "_preloadModules", preloadModules); err != nil {
		return nil, err
	}

	return nil, nil
})

var _ = RegisterRuntime("vm", "vm", func(in FunctionArgs) (*Value, error) {
	return nil, nil
})

func RegisterRuntimeLibrary(name string, fs any, path string) *ResolveResult {
	m := &ResolveResult{
		fs:   fs,
		path: path,
	}

	if _, ok := registeredRuntimes[name]; !ok {
		registeredRuntimes[name] = []*ResolveResult{}
	}

	registeredRuntimes[name] = append(registeredRuntimes[name], m)

	return m
}

func wrapScript(bytes []byte) string {
	return "(function (module, exports, require, __filename, __dirname) {\n" + string(bytes) + "\n})"
}

var bomRe = regexp.MustCompile("^#.*(\n|\r\n)")

func stripBOM(content string) string {
	return bomRe.ReplaceAllString(content, "")
}

var extensions = []string{".js", ".cjs"}
var conditions = []string{"solid", "node", "require", "default"}

func resolve(ctx context.Context, fs *Value, context string, id string, conditions []string, extensions *Value) ([]*ResolveResult, error) {
	if runtime, ok := registeredRuntimes[id]; ok {
		return runtime, nil
	}

	if !_path.IsAbs(id) && (strings.HasPrefix(id, "./") || strings.HasPrefix(id, "../") || id == "." || id == "..") {
		id = _path.Join(context, id)
	}

	var extensionsList []string
	var err error

	if extensionsList, err = extensions.Keys(ctx); err != nil {
		return nil, err
	}

	resolveFromDescriptionFile := func(path string, packagePath string) ([]*ResolveResult, error) {
		var descriptionFileData any
		descriptionFile := _path.Join(path, "package.json")
		var main string
		var descriptionFileDataMap map[string]any
		var ok bool

		if stats, err := fs.CallMethod(ctx, "statSync", descriptionFile); err != nil {
			return nil, ErrNotResolved
		} else if isFile, err := stats.CallMethod(ctx, "isFile"); err != nil {
			return nil, err
		} else if isFile, err := isFile.Bool(ctx); err != nil {
			return nil, err
		} else if isFile {
			var extension func(p string, try bool) (string, bool)
			extension = func(p string, try bool) (string, bool) {
				p2 := _path.Join(_path.Dir(descriptionFile), p)
				if stats, err := fs.CallMethod(ctx, "statSync", p2); err != nil {
					if try {
						for _, ext := range extensionsList {
							if m, ok := extension(p+ext, false); ok {
								return m, true
							}
						}
					}
				} else if isFile, err := stats.CallMethod(ctx, "isFile"); err != nil {
					return "", false
				} else if isFile, err := isFile.Bool(ctx); err != nil {
					return "", false
				} else if isFile {
					return p2, true
				} else if isDirectory, err := stats.CallMethod(ctx, "isDirectory"); err != nil {
					return "", false
				} else if isDirectory, err := isDirectory.Bool(ctx); err != nil {
					return "", false
				} else if isDirectory {
					return extension(_path.Join(p, "index"), true)
				}

				return "", false
			}

			if buffer, err := fs.CallMethod(ctx, "readFileSync", descriptionFile); err != nil {
				return nil, err
			} else if buffer, err := buffer.Get(ctx, "buffer"); err != nil {
				return nil, err
			} else if buffer, err := buffer.Unmarshal(ctx, reflect.TypeOf([]byte{})); err != nil {
				return nil, err
			} else if err := json.Unmarshal(buffer.Interface().([]byte), &descriptionFileData); err != nil {
				// NOOP
			} else if descriptionFileDataMap, ok = descriptionFileData.(map[string]any); !ok {
				// NOOP
			} else if exports, ok := descriptionFileDataMap["exports"]; !ok {
				// NOOP
			} else if exports, ok := exports.(map[string]any); !ok {
				// NOOP
			} else {
				var condition func(c any) (string, bool)
				condition = func(c any) (string, bool) {
					if conds, ok := c.(map[string]any); ok {
						for _, conddst := range conditions {
							for condsrc, c := range conds {
								if conddst == condsrc {
									if m, ok := condition(c); ok {
										return m, true
									}
								}
							}
						}
						return "", false
					} else if files, ok := c.([]string); ok {
						for _, file := range files {
							if f, ok := extension(file, true); ok {
								return f, true
							}
						}
						return "", false
					} else if file, ok := c.(string); ok {
						return extension(file, true)
					} else {
						return "", false
					}
				}

				for p, c := range exports {
					p = strings.TrimSuffix(p, "/")
					if p == packagePath {
						if main, ok = condition(c); ok {
							return []*ResolveResult{{fs: fs, path: main}}, nil
						}
					}
				}
			}

			if packagePath == "." {
				if m, ok := descriptionFileDataMap["main"]; !ok {
					if main, ok := extension(packagePath, true); ok {
						return []*ResolveResult{{fs: fs, path: main}}, nil
					}
				} else if main, ok := m.(string); ok {
					if main, ok := extension(main, true); ok {
						return []*ResolveResult{{fs: fs, path: main}}, nil
					}
				}
			} else {
				if main, ok := extension(packagePath, true); ok {
					return []*ResolveResult{{fs: fs, path: main}}, nil
				}
			}
		}

		return nil, ErrNotResolved
	}

	if _path.IsAbs(id) {
		for _, extension := range extensionsList {
			if stats, err := fs.CallMethod(ctx, "statSync", id+extension); err == nil {
				if isFile, err := stats.CallMethod(ctx, "isFile"); err != nil {
					return nil, err
				} else if isFile, err := isFile.Bool(ctx); err != nil {
					return nil, err
				} else if isFile {
					return []*ResolveResult{{fs: fs, path: id + extension}}, nil
				}
			}
		}

		if stats, err := fs.CallMethod(ctx, "statSync", id); err == nil {
			if isDirectory, err := stats.CallMethod(ctx, "isDirectory"); err != nil {
				return nil, err
			} else if isDirectory, err := isDirectory.Bool(ctx); err != nil {
				return nil, err
			} else if isDirectory {
				if result, err := resolveFromDescriptionFile(id, "."); err != nil {
					return resolve(ctx, fs, context, _path.Join(id, "index"), conditions, extensions)
				} else {
					return result, nil
				}
			} else {
				return []*ResolveResult{{fs: fs, path: id}}, nil
			}
		}
	} else {
		for searchPath := context; ; searchPath = _path.Dir(searchPath) {
			pathComponents := strings.Split(id, "/")
			var packagePath string
			path := _path.Join(searchPath, "node_modules")
			if strings.HasPrefix(pathComponents[0], "@") {
				path = _path.Join(path, pathComponents[0], pathComponents[1])
				packagePath = strings.Join(append([]string{"."}, pathComponents[2:]...), "/")
			} else {
				path = _path.Join(path, pathComponents[0])
				packagePath = strings.Join(append([]string{"."}, pathComponents[1:]...), "/")
			}
			packagePath = strings.TrimSuffix(packagePath, "/")

			if result, err := resolveFromDescriptionFile(path, packagePath); err == nil {
				return result, nil
			}

			if searchPath == "/" {
				break
			}
		}
	}

	return nil, fmt.Errorf("unable to resolve: %s in %s", id, context)
}

func (c *Context) CreateRequire(ctx context.Context, fs any, path string, extensions *Value, modules map[string]*Module, runtimes map[string]bool) (*Value, error) {
	if fs, err := c.Create(ctx, fs); err != nil {
		return nil, err
	} else if require, err := c.Create(ctx, func(in FunctionArgs) (*Value, error) {
		if id, err := in.Arg(in.ExecutionContext, 0).StringValue(in.ExecutionContext); err != nil {
			return nil, err
		} else if runtimeAllowed, ok := runtimes[id]; ok && runtimeAllowed != false {
			return nil, fmt.Errorf("unable to resolve: %s in %s", id, _path.Dir(path))
		} else if resolved, err := resolve(in.ExecutionContext, fs, _path.Dir(path), id, conditions, extensions); err != nil {
			return nil, err
		} else {
			realpath := resolved[0].path

			if resolved[0].fs != nil {
				if fs, err := c.Create(in.ExecutionContext, resolved[0].fs); err != nil {
					return nil, err
				} else if rp, err := fs.CallMethod(in.ExecutionContext, "realpathSync", resolved[0].path); err != nil {
					return nil, err
				} else if rp, err := rp.StringValue(in.ExecutionContext); err != nil {
					return nil, err
				} else {
					realpath = rp
				}
			}

			var filename string
			var dirname string

			if resolved[0].factory != nil {
				filename = realpath
				dirname = _path.Dir(filename)
			} else {
				filename = realpath
				id = filename
				dirname = _path.Dir(filename)
			}

			if module, ok := modules[id]; ok {
				if module.Error != nil {
					return nil, module.Error
				} else if moduleValue, err := in.Context.Create(in.ExecutionContext, module); err != nil {
					return nil, err
				} else if exports, err := moduleValue.Get(in.ExecutionContext, "exports"); err != nil {
					return nil, err
				} else {
					return exports, nil
				}
			} else if require, err := c.CreateRequire(in.ExecutionContext, fs, filename, extensions, modules, runtimes); err != nil {
				return nil, err
			} else if exports, err := in.Context.NewObject(in.ExecutionContext); err != nil {
				return nil, err
			} else {
				module := &Module{
					Context:  in.Context,
					ID:       id,
					FS:       resolved[0].fs,
					Filename: filename,
					Dirname:  dirname,
					Require:  require,
				}

				modules[id] = module

				if moduleValue, err := in.Context.Create(in.ExecutionContext, module); err != nil {
					return nil, err
				} else if err := moduleValue.RebindAll(in.ExecutionContext); err != nil {
					return nil, err
				} else if err := moduleValue.Set(in.ExecutionContext, "exports", exports); err != nil {
					return nil, err
				} else {
					for _, r := range resolved {
						if r.factory != nil {
							if exports, err := moduleValue.Get(in.ExecutionContext, "exports"); err != nil {
								return nil, err
							} else if global, err := in.Context.Global(in.ExecutionContext); err != nil {
								module.Error = err
							} else if factory, err := in.Context.Create(in.ExecutionContext, r.factory); err != nil {
								module.Error = err
								break
							} else if _, err := factory.Call(in.ExecutionContext, global, moduleValue, exports, module.Require, module.Filename, module.Dirname); err != nil {
								module.Error = err
								break
							} else if exports, err := moduleValue.Get(in.ExecutionContext, "exports"); err != nil {
								module.Error = err
								break
							} else if exports.IsNil() {
								module.Error = errors.New("exports is nil")
								break
							} else if _default, err := exports.Get(in.ExecutionContext, "default"); err != nil {
								module.Error = err
								break
							} else if _default.IsKind(KindObject) && !_default.IsKind(KindUndefined) && !_default.IsKind(KindNull) {
								if err := moduleValue.Set(in.ExecutionContext, "exports", _default); err != nil {
									module.Error = err
									break
								} else if _, err := in.Context.Assign(in.ExecutionContext, _default, exports); err != nil {
									module.Error = err
									break
								}
							}
						} else if r.fs != nil {
							ext := _path.Ext(r.path)
							if ext == "" {
								ext = ".js"
							}
							if extension, err := extensions.Get(in.ExecutionContext, ext); err != nil {
								module.Error = err
								break
							} else if !extension.IsKind(KindFunction) {
								module.Error = ErrNotResolved
								break
							} else if _, err := extension.Call(in.ExecutionContext, nil, moduleValue, module.Filename); err != nil {
								module.Error = err
								break
							}
						}
					}

					if module.Error != nil {
						return nil, module.Error
					} else if exports, err := moduleValue.Get(in.ExecutionContext, "exports"); err != nil {
						return nil, err
					} else {
						return exports, nil
					}
				}
			}
		}
	}); err != nil {
		return nil, err
	} else if resolve, err := c.Create(ctx, func(in FunctionArgs) (*Value, error) {
		if id, err := in.Arg(in.ExecutionContext, 0).StringValue(in.ExecutionContext); err != nil {
			return nil, err
		} else if resolved, err := resolve(in.ExecutionContext, fs, path, id, conditions, extensions); err != nil {
			return nil, err
		} else {
			if resolved[0].factory != nil {
				return in.Arg(in.ExecutionContext, 0), nil
			}

			return in.Context.Create(in.ExecutionContext, resolved[0].path)
		}
	}); err != nil {
		return nil, err
	} else if err := require.Set(ctx, "resolve", resolve); err != nil {
		return nil, err
	} else if err := require.Set(ctx, "extensions", extensions); err != nil {
		return nil, err
	} else {
		return require, nil
	}
}

func (c *Context) RunWithRuntime(ctx context.Context, fs any, path string, env func(RuntimeFunctionArgs) error, security map[string]bool) (*Value, error) {
	modules := map[string]*Module{}

	if extensions, err := c.NewObject(ctx); err != nil {
		return nil, err
	} else if js, err := c.Create(ctx, func(in FunctionArgs) (*Value, error) {
		module := in.Arg(in.ExecutionContext, 0)

		if fs, err := module.Get(in.ExecutionContext, "fs"); err != nil {
			return nil, err
		} else if filename, err := in.Arg(in.ExecutionContext, 1).StringValue(in.ExecutionContext); err != nil {
			return nil, err
		} else if buffer, err := fs.CallMethod(in.ExecutionContext, "readFileSync", filename); err != nil {
			return nil, err
		} else if string, err := buffer.CallMethod(in.ExecutionContext, "toString", "utf8"); err != nil {
			return nil, err
		} else {
			if _, err := module.CallMethod(in.ExecutionContext, "_compile", string, filename); err != nil {
				return nil, err
			} else {
				return nil, nil
			}
		}
	}); err != nil {
		return nil, err
	} else if json, err := c.Create(ctx, func(in FunctionArgs) (*Value, error) {
		module := in.Arg(in.ExecutionContext, 0)

		if fs, err := module.Get(in.ExecutionContext, "fs"); err != nil {
			return nil, err
		} else if filename, err := in.Arg(in.ExecutionContext, 1).StringValue(in.ExecutionContext); err != nil {
			return nil, err
		} else if buffer, err := fs.CallMethod(in.ExecutionContext, "readFileSync", filename); err != nil {
			return nil, err
		} else if string, err := buffer.CallMethod(in.ExecutionContext, "toString", "utf8"); err != nil {
			return nil, err
		} else if json, err := in.Context.ParseJSON(in.ExecutionContext, string.String()); err != nil {
			return nil, err
		} else if err := module.Set(in.ExecutionContext, "exports", json); err != nil {
			return nil, err
		} else {
			return nil, nil
		}
	}); err != nil {
		return nil, err
	} else if err := extensions.Set(ctx, ".js", js); err != nil {
		return nil, err
	} else if err := extensions.Set(ctx, ".cjs", js); err != nil {
		return nil, err
	} else if err := extensions.Set(ctx, ".json", json); err != nil {
		return nil, err
	} else if global, err := c.Global(ctx); err != nil {
		return nil, err
	} else if require, err := c.CreateRequire(ctx, fs, path, extensions, modules, security); err != nil {
		return nil, err
	} else {
		in := RuntimeFunctionArgs{
			FunctionArgs: FunctionArgs{
				ExecutionContext: ctx,
				Context:          c,
				This:             global,
				IsConstructCall:  false,
				Args:             []*Value{},
				Holder:           nil,
			},
			Require: require,
		}

		if err := env(in); err != nil {
			return nil, err
		}

		return require.Call(ctx, nil, path)
	}
}

func (m *Module) V8GetId(in GetterArgs) (*Value, error) {
	return in.Context.Create(in.ExecutionContext, m.ID)
}

func (m *Module) V8GetFs(in GetterArgs) (*Value, error) {
	return in.Context.Create(in.ExecutionContext, m.FS)
}

func (m *Module) V8GetFilename(in GetterArgs) (*Value, error) {
	return in.Context.Create(in.ExecutionContext, m.Filename)
}

func (m *Module) V8GetDirname(in GetterArgs) (*Value, error) {
	return in.Context.Create(in.ExecutionContext, m.Dirname)
}

func (m *Module) V8GetRequire(in GetterArgs) (*Value, error) {
	return m.Require, nil
}

func (m *Module) V8Func_compile(in FunctionArgs) (*Value, error) {
	if global, err := in.Context.Global(in.ExecutionContext); err != nil {
		return nil, err
	} else if content, err := in.Arg(in.ExecutionContext, 0).StringValue(in.ExecutionContext); err != nil {
		return nil, err
	} else if filename, err := in.Arg(in.ExecutionContext, 1).StringValue(in.ExecutionContext); err != nil {
		return nil, err
	} else if value, err := in.Context.Run(in.ExecutionContext, wrapScript([]byte(stripBOM(content))), filename, m); err != nil {
		return nil, err
	} else if exports, err := in.This.Get(in.ExecutionContext, "exports"); err != nil {
		return nil, err
	} else if _, err := value.Call(in.ExecutionContext, global, m, exports, m.Require, m.Filename, m.Dirname); err != nil {
		return nil, err
	} else {
		return nil, nil
	}
}

func (m *Module) ImportModuleDynamically(ctx context.Context, specifier string, resourceName string, importAssertions []*Value) (*Value, error) {
	if strings.HasPrefix(specifier, "file://") {
		specifier = specifier[7:]
	}
	return m.Require.Call(ctx, nil, specifier)
}
