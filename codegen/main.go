package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"path"
	"regexp"

	"os"
	"strings"
)

func main() {
	// generate rtti
	// generate typescript types
	// generate adapters for different interfaces
	// magic comments in the parser
	var fileSet token.FileSet

	tslib := os.Getenv("GOTSLIB")
	srcFilename := os.Getenv("GOFILE")

	if wd, err := os.Getwd(); err != nil {
		panic(err)
	} else if f, err := parser.ParseFile(&fileSet, path.Join(wd, srcFilename), nil, parser.ParseComments|parser.AllErrors); err != nil {
		panic(err)
	} else {
		tags := []string{"js", "ts"}

		filename := path.Join(wd, srcFilename)

		basename := strings.TrimSuffix(path.Base(srcFilename), ".go")
		dirname := path.Dir(filename)
		filename = path.Join(dirname, basename+"_runtime.go")

		if body, err := os.ReadFile(srcFilename); err != nil {
			panic(err)
		} else if nodes, err := FindDeclarationCommentTags(string(body), tags, f); err != nil {
			panic(err)
		} else if len(nodes) == 0 {
			return
		} else if code, err := GenerateCode(srcFilename, string(body), f, nodes); err != nil {
			panic(err)
		} else if err := os.WriteFile(filename, []byte(code), 0644); err != nil {
			panic(err)
		} else {
			var pkgName string
			for _, node := range nodes {
				for _, tag := range node.Tags {
					args := strings.Split(tag.Text, " ")

					if tag.Name == "js" {
						if args[0] == "package" {
							pkgName = args[1]
						}
					}
				}
			}

			pkgName = strings.ReplaceAll(pkgName, ":", "/")

			typesFilename := path.Join(tslib, pkgName, basename+".d.ts")

			if err := os.MkdirAll(path.Join(path.Dir(typesFilename)), 0755); err != nil {
				panic(err)
			} else if types, err := GenerateTypes(srcFilename, string(body), f, nodes); err != nil {
				panic(err)
			} else if err := os.WriteFile(typesFilename, []byte(types), 0644); err != nil {
				panic(err)
			}
		}
	}
}

func GenerateCode(filename string, file string, a *ast.File, nodes []*TaggedNode) (string, error) {
	imports := map[string]string{}
	code := ""
	var offset token.Pos = 0

	imports["github.com/grexie/isolates"] = "isolates"

	var addImport func(t ast.Expr)
	addImport = func(t ast.Expr) {
		if starExpr, ok := t.(*ast.StarExpr); ok {
			addImport(starExpr.X)
			return
		}
		if arrayType, ok := t.(*ast.ArrayType); ok {
			addImport(arrayType.Elt)
			return
		}
		if funcType, ok := t.(*ast.FuncType); ok {
			if funcType.Params.NumFields() > 0 {
				for _, t := range funcType.Params.List {
					addImport(t.Type)
				}
			}
			if funcType.Results.NumFields() > 0 {
				for _, t := range funcType.Results.List {
					addImport(t.Type)
				}
			}
			return
		}
		name := file[t.Pos()-offset : t.End()-offset]

		if !strings.Contains(name, ".") {
			return
		}

		for _, imp := range a.Imports {
			var impName string
			if imp.Name != nil {
				impName = imp.Name.Name
				impName = strings.Split(impName, " ")[0]
			} else {
				impName = imp.Path.Value
				impName = impName[1 : len(impName)-1]
				impParts := strings.Split(impName, "/")
				for i, j := 0, len(impParts)-1; i < j; i, j = i+1, j-1 {
					impParts[i], impParts[j] = impParts[j], impParts[i]
				}
				if ok, _ := regexp.Match("v\\d+", []byte(impParts[0])); ok {
					impName = impParts[1]
				} else {
					impName = impParts[0]
				}
			}

			nameParts := strings.Split(name, ".")
			if impName == nameParts[0] {
				imports[imp.Path.Value[1:len(imp.Path.Value)-1]] = impName
			}
		}
	}

	var pkgName string

	for _, node := range nodes {
		for _, tag := range node.Tags {
			args := strings.Split(tag.Text, " ")

			if tag.Name == "js" {
				if args[0] == "package" {
					pkgName = args[1]
				}
			}
		}
	}

	code = code + "var _ = isolates.RegisterRuntime(\"" + pkgName + "\", \"" + filename + "\", func (in isolates.FunctionArgs) (*isolates.Value, error) {\n"
	events := map[string]map[string]*ast.FuncDecl{}
	fns := []string{}

	runtimeStanza1 := ""
	runtimeStanza1 = runtimeStanza1 + "var Module *isolates.Module\n"
	runtimeStanza1 = runtimeStanza1 + "Exports := in.Args[1]\n"
	runtimeStanza1 = runtimeStanza1 + "Require := in.Args[2]\n"
	runtimeStanza1 = runtimeStanza1 + "if m, err := in.Arg(in.ExecutionContext, 0).Unmarshal(in.ExecutionContext, reflect.TypeOf(Module)); err != nil {\n"
	runtimeStanza1 = runtimeStanza1 + "  return nil, err\n"
	runtimeStanza1 = runtimeStanza1 + "} else {\n"
	runtimeStanza1 = runtimeStanza1 + "  Module = m.Interface().(*isolates.Module)\n"
	runtimeStanza1 = runtimeStanza1 + "}\n\n"

	runtimeStanza2 := ""
	runtimeStanza2 = runtimeStanza2 + "rin := isolates.RuntimeFunctionArgs{FunctionArgs: in, Module: Module, Exports: Exports, Require: Require}\n\n"

	for _, node := range nodes {
		exports := []string{}
		exportInstances := []string{}
		decorators := []string{}
		runtimeArgs := false
		importedRuntimeArgs := false

		for _, tag := range node.Tags {
			args := strings.Split(tag.Text, " ")

			if tag.Name == "js" {
				if args[0] == "export" {
					exports = append(exports, args[1])
				}

				if args[0] == "export-instance" {
					exportInstances = append(exports, args[1])
				}

				if args[0] == "decorator" {
					decorators = append(decorators, args[1])
				}
			}
		}

		for _, tag := range node.Tags {
			args := strings.Split(tag.Text, " ")

			if tag.Name == "js" {
				if args[0] == "constructor" {
					if n, ok := node.Node.(*ast.FuncDecl); !ok {
						return "", fmt.Errorf("constructor is not a function: %s", node.String())
					} else {

						argsCode := ""
						invocation := n.Name.Name + "("
						invocationParams := []string{}

						argsOffset := 0
						for i, argNode := range n.Type.Params.List {
							typeName := file[argNode.Type.Pos()-offset : argNode.Type.End()-offset]
							if typeName == "isolates.RuntimeFunctionArgs" {
								invocationParams = append(invocationParams, "rin")
								runtimeArgs = true
								argsCode = argsCode + runtimeStanza2
								argsOffset++
							} else if typeName == "isolates.FunctionArgs" {
								invocationParams = append(invocationParams, "in")
								argsOffset++
							} else if typeName == "context.Context" {
								invocationParams = append(invocationParams, "in.ExecutionContext")
								argsOffset++
							} else if strings.HasPrefix(typeName, "...") {
								typeName = typeName[3:]
								addImport(argNode.Type.(*ast.Ellipsis).Elt)

								argName := fmt.Sprintf("args%d", i-argsOffset)
								if len(argNode.Names) > 0 {
									argName = argNode.Names[0].Name
								}

								argsCode = argsCode + fmt.Sprintf("  %s := make([]%s, len(in.Args)-%d)\n", argName, typeName, i-argsOffset)
								argsCode = argsCode + fmt.Sprintf("  for i, arg := range in.Args[%d:] {\n", i-argsOffset)
								if typeName == "interface{}" || typeName == "*isolates.Value" || typeName == "any" {
									argsCode = argsCode + fmt.Sprintf("    %s[i] = arg\n", argName)
								} else {
									imports["reflect"] = "reflect"
									argsCode = argsCode + fmt.Sprintf("    if v, err := arg.Unmarshal(in.ExecutionContext, reflect.TypeOf(&%s[i]).Elem()); err != nil {\n", argName)
									argsCode = argsCode + "      return nil, err\n"
									argsCode = argsCode + "    } else { \n"
									argsCode = argsCode + fmt.Sprintf("      %s[i] = v.Interface().(%s)\n", argName, typeName)
									argsCode = argsCode + "    }\n"
								}

								argsCode = argsCode + "  }\n\n"

								invocationParams = append(invocationParams, fmt.Sprintf("%s...", argName))
							} else {
								addImport(argNode.Type)
								imports["reflect"] = "reflect"
								argsCode = argsCode + "    var _" + argNode.Names[0].Name + " " + typeName + "\n"
								argsCode = argsCode + fmt.Sprintf("    if v, err := in.Arg(in.ExecutionContext, %d).Unmarshal(in.ExecutionContext, reflect.TypeOf(&_"+argNode.Names[0].Name+").Elem()); err != nil {\n", i-argsOffset)
								argsCode = argsCode + "      return nil, err\n"
								argsNilCheck := ""
								if strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "[]") {
									argsNilCheck = "if v != nil "
								}
								argsCode = argsCode + "    } else " + argsNilCheck + "{\n"
								argsCode = argsCode + "      _" + argNode.Names[0].Name + " = v.Interface().(" + typeName + ")\n"
								argsCode = argsCode + "    }\n\n"
								invocationParams = append(invocationParams, "_"+argNode.Names[0].Name)
							}

						}

						invocation += strings.Join(invocationParams, ", ") + ")"

						typeStart := n.Type.Results.List[0].Type.Pos() - offset
						typeEnd := n.Type.Results.List[0].Type.End() - offset

						typeName := file[typeStart:typeEnd]

						jsTypeName := strings.Trim(typeName, "[]*")
						if len(args) > 1 {
							jsTypeName = args[1]
						}

						// addImport(n.Type)

						if len(n.Type.Results.List) == 2 && file[n.Type.Results.List[1].Type.Pos()-offset:n.Type.Results.List[1].Type.End()-offset] == "error" {
							invocation = "    return " + invocation
						} else if len(n.Type.Results.List) == 1 {
							invocation = "    return " + invocation + ", nil"
						} else {
							panic(fmt.Errorf("cannot export type: %v", n.Name))
						}

						constructorName := "constructor"
						if len(exports) == 0 && len(exportInstances) == 0 {
							constructorName = "_"
						}

						if runtimeArgs && !importedRuntimeArgs {
							importedRuntimeArgs = true
							imports["reflect"] = "reflect"
							code = code + runtimeStanza1 + "\n"
						}

						code = code + "  if " + constructorName + ", err := in.Context.CreateWithName(in.ExecutionContext, \"" + jsTypeName + "\", func (in isolates.FunctionArgs) (" + typeName + ", error) {\n"
						code = code + argsCode
						code = code + invocation + "\n"
						code = code + "  }); err != nil {\n"
						code = code + "    return nil, err\n"

						for _, export := range exports {
							code = code + "  } else if err := in.Args[1].Set(in.ExecutionContext, \"" + export + "\", constructor); err != nil {\n"
							code = code + "    return nil, err\n"
						}

						if len(exportInstances) > 0 {

							code = code + "  } else if instance, err := " + constructorName + ".New(in.ExecutionContext); err != nil {\n"
							code = code + "    return nil, err\n"

							for _, export := range exportInstances {
								code = code + "  } else if err := in.Args[1].Set(in.ExecutionContext, \"" + export + "\", instance); err != nil {\n"
								code = code + "    return nil, err\n"
							}
						}

						code = code + "  }"

						code = code + "\n\n"
					}
				} else if args[0] == "method" {
					if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
						if funcDecl.Recv.NumFields() <= 1 {
							if funcDecl.Type.Results.NumFields() <= 2 {
								var recv ast.Expr

								if funcDecl.Recv.NumFields() == 1 {
									recv = funcDecl.Recv.List[0].Type
									addImport(recv)
								}

								var recvTypeName, recvID string
								if recv != nil {
									typeStart := recv.Pos() - offset
									typeEnd := recv.End() - offset
									recvTypeName = file[typeStart:typeEnd]
									recvID = funcDecl.Recv.List[0].Names[0].Name
								}

								funcName := funcDecl.Name.Name
								funcName = strings.ToLower(funcName[0:1]) + funcName[1:]

								if len(args) > 1 {
									funcName = args[1]
								}

								jsFuncName := funcName

								funcName = strings.ToUpper(funcName[0:1]) + funcName[1:]
								var invocation string
								argsCode := ""

								if recv != nil {
									invocation = recvID + "." + funcDecl.Name.Name + "("
								} else {
									invocation = funcDecl.Name.Name + "("
								}
								invocationParams := []string{}

								argsOffset := 0
								for i, argNode := range funcDecl.Type.Params.List {
									typeName := file[argNode.Type.Pos()-offset : argNode.Type.End()-offset]

									if typeName == "isolates.RuntimeFunctionArgs" {
										invocationParams = append(invocationParams, "rin")
										runtimeArgs = true
										argsCode = argsCode + runtimeStanza2
										argsOffset++
									} else if typeName == "isolates.FunctionArgs" {
										invocationParams = append(invocationParams, "in")
										argsOffset++
									} else if typeName == "context.Context" {
										invocationParams = append(invocationParams, "in.ExecutionContext")
										argsOffset++
									} else if strings.HasPrefix(typeName, "...") {
										typeName = typeName[3:]
										addImport(argNode.Type.(*ast.Ellipsis).Elt)

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + fmt.Sprintf("  %s := make([]%s, len(in.Args)-%d)\n", argName, typeName, i-argsOffset)
										argsCode = argsCode + fmt.Sprintf("  for i, arg := range in.Args[%d:] {\n", i-argsOffset)
										if typeName == "interface{}" || typeName == "*isolates.Value" || typeName == "any" {
											argsCode = argsCode + fmt.Sprintf("    %s[i] = arg\n", argName)
										} else {
											imports["reflect"] = "reflect"
											argsCode = argsCode + fmt.Sprintf("    if v, err := arg.Unmarshal(in.ExecutionContext, reflect.TypeOf(&%s[i]).Elem()); err != nil {\n", argName)
											argsCode = argsCode + "      return nil, err\n"
											argsCode = argsCode + "    } else { \n"
											argsCode = argsCode + fmt.Sprintf("      %s[i] = v.Interface().(%s)\n", argName, typeName)
											argsCode = argsCode + "    }\n"
										}

										argsCode = argsCode + "  }\n\n"

										invocationParams = append(invocationParams, fmt.Sprintf("%s...", argName))
									} else {

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										if typeName == "interface{}" || typeName == "*isolates.Value" || typeName == "any" {
											argsCode = argsCode + fmt.Sprintf("  %s := in.Arg(in.ExecutionContext, %d)\n", argName, i-argsOffset)
										} else {
											imports["reflect"] = "reflect"
											addImport(argNode.Type)
											argsCode = argsCode + "  var " + argName + " " + typeName + "\n"
											argsCode = argsCode + fmt.Sprintf("  if v, __err := in.Arg(in.ExecutionContext, %d).Unmarshal(in.ExecutionContext, reflect.TypeOf(&"+argName+").Elem()); __err != nil {\n", i-argsOffset)
											argsCode = argsCode + "    return nil, __err\n"
											argsNilCheck := ""
											if strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "[]") {
												argsNilCheck = "if v != nil "
											}
											argsCode = argsCode + "  } else " + argsNilCheck + "{\n"
											argsCode = argsCode + "    " + argName + " = v.Interface().(" + typeName + ")\n"
											argsCode = argsCode + "  }\n\n"
										}

										invocationParams = append(invocationParams, argName)
									}
								}

								invocation += strings.Join(invocationParams, ", ") + ")"

								if funcDecl.Type.Results.NumFields() == 2 && file[funcDecl.Type.Results.List[1].Type.Pos()-offset:funcDecl.Type.Results.List[1].Type.End()-offset] == "error" {
									invocation = "  if result, err := " + invocation + "; err != nil {\n"
									invocation = invocation + "    return nil, err\n"
									invocation = invocation + "  } else {\n"
									if len(decorators) > 0 {
										invocation = invocation + fmt.Sprintf("  return result.%s(in)\n", decorators[0])
									} else {
										invocation = invocation + "    return in.Context.Create(in.ExecutionContext, result)\n"
									}
									invocation = invocation + "  }"
								} else if funcDecl.Type.Results.NumFields() == 1 && file[funcDecl.Type.Results.List[0].Type.Pos()-offset:funcDecl.Type.Results.List[0].Type.End()-offset] == "error" {
									invocation = "  if err := " + invocation + "; err != nil {\n"
									invocation = invocation + "    return nil, err\n"
									invocation = invocation + "  } else {\n"
									invocation = invocation + "    return nil, nil\n"
									invocation = invocation + "  }"
								} else if funcDecl.Type.Results.NumFields() == 1 {
									invocation = "  result := " + invocation + "\n"
									if len(decorators) > 0 {
										invocation = invocation + fmt.Sprintf("  return result.%s(in)\n", decorators[0])
									} else {
										invocation = invocation + "  return in.Context.Create(in.ExecutionContext, result)"
									}
								} else {
									invocation = "  " + invocation + "\n" + "  return nil, nil"
								}

								if recv != nil {
									code := ""

									code = code + "func (" + recvID + " " + recvTypeName + ") V8Func" + funcName + "(in isolates.FunctionArgs) (*isolates.Value, error) {\n"

									code = code + argsCode
									code = code + invocation + "\n"
									code = code + "}"

									fns = append(fns, code)
								} else if len(exports) > 0 || len(exportInstances) > 0 {
									fnName := "fn"
									if len(exports) == 0 && len(exportInstances) == 0 {
										fnName = "_"
									}
									re, _ := regexp.Compile("\n  ")

									if runtimeArgs && !importedRuntimeArgs {
										importedRuntimeArgs = true
										imports["reflect"] = "reflect"
										code = code + runtimeStanza1 + "\n"
									}

									code = code + "  {\n"
									code = code + "    fnName := \"" + jsFuncName + "\"\n"
									code = code + "    if " + fnName + ", err := in.Context.CreateFunction(in.ExecutionContext, &fnName, func (in isolates.FunctionArgs) (*isolates.Value, error) {\n"
									code = code + re.ReplaceAllString(argsCode, "\n      ")
									code = code + "    " + re.ReplaceAllString(invocation, "\n      ") + "\n"
									code = code + "    }); err != nil {\n"
									code = code + "      return nil, err\n"

									for _, export := range exports {
										code = code + "    } else if err := in.Args[1].Set(in.ExecutionContext, \"" + export + "\", " + fnName + "); err != nil {\n"
										code = code + "      return nil, err\n"
									}

									if len(exportInstances) > 0 {
										code = code + "    } else if instance, err := " + fnName + ".Call(in.ExecutionContext, nil); err != nil {\n"
										code = code + "      return nil, err\n"

										for _, export := range exportInstances {
											code = code + "    } else if err := in.Args[1].Set(in.ExecutionContext, \"" + export + "\", instance); err != nil {\n"
											code = code + "      return nil, err\n"
										}
									}

									code = code + "    }\n"
									code = code + "  }"

									code = code + "\n\n"
								}
							}
						}
					}
				} else if args[0] == "async-method" {
					if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
						if funcDecl.Recv.NumFields() == 1 {
							if funcDecl.Type.Results.NumFields() <= 2 {
								recv := funcDecl.Recv.List[0].Type
								ret := funcDecl.Type.Results.List[0].Type

								addImport(recv)

								typeStart := recv.Pos() - offset
								typeEnd := recv.End() - offset
								recvTypeName := file[typeStart:typeEnd]
								recvID := funcDecl.Recv.List[0].Names[0].Name

								typeStart = ret.Pos() - offset
								typeEnd = ret.End() - offset
								// retTypeName := file[typeStart:typeEnd]

								code := ""

								funcName := funcDecl.Name.Name

								if len(args) > 1 {
									funcName = args[1]
								}

								funcName = strings.ToUpper(funcName[0:1]) + funcName[1:]

								argsCode := ""
								invocation := recvID + "." + funcDecl.Name.Name + "("
								invocationParams := []string{}

								argsOffset := 0
								for i, argNode := range funcDecl.Type.Params.List {
									typeName := file[argNode.Type.Pos()-offset : argNode.Type.End()-offset]

									if typeName == "isolates.RuntimeFunctionArgs" {
										invocationParams = append(invocationParams, "rin")
										runtimeArgs = true
										argsCode = argsCode + runtimeStanza2
										argsOffset++
									} else if typeName == "isolates.FunctionArgs" {
										invocationParams = append(invocationParams, "in")
										argsOffset++
									} else if typeName == "context.Context" {
										invocationParams = append(invocationParams, "in.ExecutionContext")
										argsOffset++
									} else if strings.HasPrefix(typeName, "...") {
										typeName = typeName[3:]
										addImport(argNode.Type.(*ast.Ellipsis).Elt)

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + fmt.Sprintf("  %s := make([]%s, len(in.Args)-%d)\n", argName, typeName, i-argsOffset)
										argsCode = argsCode + fmt.Sprintf("  for i, arg := range in.Args[%d:] {\n", i-argsOffset)
										if typeName == "interface{}" || typeName == "*isolates.Value" || typeName == "any" {
											argsCode = argsCode + fmt.Sprintf("    %s[i] = arg\n", argName)
										} else {
											imports["reflect"] = "reflect"
											argsCode = argsCode + fmt.Sprintf("    if v, err := arg.Unmarshal(in.ExecutionContext, reflect.TypeOf(&%s[i]).Elem()); err != nil {\n", argName)
											argsCode = argsCode + "      return nil, err\n"
											argsCode = argsCode + "    } else { \n"
											argsCode = argsCode + fmt.Sprintf("      %s[i] = v.Interface().(%s)\n", argName, typeName)
											argsCode = argsCode + "    }\n"
										}

										argsCode = argsCode + "  }\n\n"

										invocationParams = append(invocationParams, fmt.Sprintf("%s...", argName))
									} else {
										addImport(argNode.Type)
										imports["reflect"] = "reflect"

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + "    var " + argName + " " + typeName + "\n"
										argsCode = argsCode + fmt.Sprintf("    if v, err := in.Arg(in.ExecutionContext, %d).Unmarshal(in.ExecutionContext, reflect.TypeOf(&"+argName+").Elem()); err != nil {\n", i-argsOffset)
										argsCode = argsCode + "      resolver.Reject(in.ExecutionContext, err)\n"
										argsCode = argsCode + "      return\n"
										argsNilCheck := ""
										if strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "[]") {
											argsNilCheck = "if v != nil "
										}
										argsCode = argsCode + "    } else " + argsNilCheck + "{\n"
										argsCode = argsCode + "      " + argName + " = v.Interface().(" + typeName + ")\n"
										argsCode = argsCode + "    }\n\n"

										invocationParams = append(invocationParams, argName)
									}
								}

								invocation += strings.Join(invocationParams, ", ") + ")"

								if len(funcDecl.Type.Results.List) == 2 && file[funcDecl.Type.Results.List[1].Type.Pos()-offset:funcDecl.Type.Results.List[1].Type.End()-offset] == "error" {
									invocation = "      if result, err := " + invocation + "; err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else if result, err := in.Context.Create(in.ExecutionContext, result); err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else {\n"
									invocation = invocation + "        resolver.Resolve(in.ExecutionContext, result)\n"
									invocation = invocation + "      }"
								} else if len(funcDecl.Type.Results.List) == 1 && file[funcDecl.Type.Results.List[0].Type.Pos()-offset:funcDecl.Type.Results.List[0].Type.End()-offset] == "error" {
									invocation = "      if err := " + invocation + "; err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else if result, err := in.Context.Undefined(in.ExecutionContext); err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else {\n"
									invocation = invocation + "        resolver.Resolve(in.ExecutionContext, result)\n"
									invocation = invocation + "      }"
								} else if len(funcDecl.Type.Results.List) == 1 {
									invocation = "      result := " + invocation + "\n"
									invocation = invocation + "      if result, err := in.Context.Create(in.ExecutionContext, result); err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else {\n"
									invocation = invocation + "        resolver.Resolve(in.ExecutionContext, result)\n"
									invocation = invocation + "      }"
								} else {
									panic(fmt.Errorf("cannot export type: %v", funcDecl.Type.Results))
								}

								code = code + "func (" + recvID + " " + recvTypeName + ") V8Func" + funcName + "(in isolates.FunctionArgs) (*isolates.Value, error) {\n"
								code = code + "  if resolver, err := in.Context.NewResolver(in.ExecutionContext); err != nil {\n"
								code = code + "    return nil, err\n"
								code = code + "  } else {\n"
								code = code + "    in.Background(func(in isolates.FunctionArgs) {\n"
								code = code + argsCode
								code = code + invocation + "\n"
								code = code + "    })\n\n"
								code = code + "    return resolver.Promise(in.ExecutionContext)\n"
								code = code + "  }\n"
								code = code + "}"

								fns = append(fns, code)
							}
						}
					}
				} else if args[0] == "callback-method" {
					if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
						if funcDecl.Recv.NumFields() == 1 {
							if funcDecl.Type.Results.NumFields() <= 2 {
								recv := funcDecl.Recv.List[0].Type
								ret := funcDecl.Type.Results.List[0].Type

								addImport(recv)

								typeStart := recv.Pos() - offset
								typeEnd := recv.End() - offset
								recvTypeName := file[typeStart:typeEnd]
								recvID := funcDecl.Recv.List[0].Names[0].Name

								typeStart = ret.Pos() - offset
								typeEnd = ret.End() - offset
								// retTypeName := file[typeStart:typeEnd]

								code := ""

								funcName := funcDecl.Name.Name

								if len(args) > 1 {
									funcName = args[1]
								}

								funcName = strings.ToUpper(funcName[0:1]) + funcName[1:]

								argsCode := ""
								invocation := recvID + "." + funcDecl.Name.Name + "("
								invocationParams := []string{}

								argsOffset := 0
								for i, argNode := range funcDecl.Type.Params.List {
									typeName := file[argNode.Type.Pos()-offset : argNode.Type.End()-offset]

									if typeName == "isolates.RuntimeFunctionArgs" {
										invocationParams = append(invocationParams, "rin")
										runtimeArgs = true
										argsCode = argsCode + runtimeStanza2
										argsOffset++
									} else if typeName == "isolates.FunctionArgs" {
										invocationParams = append(invocationParams, "in")
										argsOffset++
									} else if typeName == "context.Context" {
										invocationParams = append(invocationParams, "in.ExecutionContext")
										argsOffset++
									} else if strings.HasPrefix(typeName, "...") {
										typeName = typeName[3:]
										addImport(argNode.Type.(*ast.Ellipsis).Elt)

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + fmt.Sprintf("  %s := make([]%s, len(in.Args)-%d)\n", argName, typeName, i-argsOffset)
										argsCode = argsCode + fmt.Sprintf("  for i, arg := range in.Args[%d:] {\n", i-argsOffset)
										if typeName == "interface{}" || typeName == "*isolates.Value" || typeName == "any" {
											argsCode = argsCode + fmt.Sprintf("    %s[i] = arg\n", argName)
										} else {
											imports["reflect"] = "reflect"
											argsCode = argsCode + fmt.Sprintf("    if v, err := arg.Unmarshal(in.ExecutionContext, reflect.TypeOf(&%s[i]).Elem()); err != nil {\n", argName)
											argsCode = argsCode + "      return nil, err\n"
											argsCode = argsCode + "    } else { \n"
											argsCode = argsCode + fmt.Sprintf("      %s[i] = v.Interface().(%s)\n", argName, typeName)
											argsCode = argsCode + "    }\n"
										}

										argsCode = argsCode + "  }\n\n"

										invocationParams = append(invocationParams, fmt.Sprintf("%s...", argName))
									} else {
										addImport(argNode.Type)
										imports["reflect"] = "reflect"

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + "    var " + argName + " " + typeName + "\n"
										argsCode = argsCode + fmt.Sprintf("    if v, err := in.Arg(in.ExecutionContext, %d).Unmarshal(in.ExecutionContext, reflect.TypeOf(&"+argName+").Elem()); err != nil {\n", i-argsOffset)
										argsCode = argsCode + "      resolver.Reject(in.ExecutionContext, err)\n"
										argsCode = argsCode + "      return\n"
										argsNilCheck := ""
										if strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "[]") {
											argsNilCheck = "if v != nil "
										}
										argsCode = argsCode + "    } else " + argsNilCheck + "{\n"
										argsCode = argsCode + "      " + argName + " = v.Interface().(" + typeName + ")\n"
										argsCode = argsCode + "    }\n\n"

										invocationParams = append(invocationParams, argName)
									}
								}

								invocation += strings.Join(invocationParams, ", ") + ")"

								if len(funcDecl.Type.Results.List) == 2 && file[funcDecl.Type.Results.List[1].Type.Pos()-offset:funcDecl.Type.Results.List[1].Type.End()-offset] == "error" {
									invocation = "      if result, err := " + invocation + "; err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else if result, err := in.Context.Create(in.ExecutionContext, result); err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else {\n"
									invocation = invocation + "        resolver.Resolve(in.ExecutionContext, result)\n"
									invocation = invocation + "      }"
								} else if len(funcDecl.Type.Results.List) == 1 && file[funcDecl.Type.Results.List[0].Type.Pos()-offset:funcDecl.Type.Results.List[0].Type.End()-offset] == "error" {
									invocation = "      if err := " + invocation + "; err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else if result, err := in.Context.Undefined(in.ExecutionContext); err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else {\n"
									invocation = invocation + "        resolver.Resolve(in.ExecutionContext, result)\n"
									invocation = invocation + "      }"
								} else if len(funcDecl.Type.Results.List) == 1 {
									invocation = "      result := " + invocation + "\n"
									invocation = invocation + "      if result, err := in.Context.Create(in.ExecutionContext, result); err != nil {\n"
									invocation = invocation + "        resolver.Reject(in.ExecutionContext, err)\n"
									invocation = invocation + "      } else {\n"
									invocation = invocation + "        resolver.Resolve(in.ExecutionContext, result)\n"
									invocation = invocation + "      }"
								} else {
									panic(fmt.Errorf("cannot export type: %v", funcDecl.Type.Results))
								}

								code = code + "func (" + recvID + " " + recvTypeName + ") V8Func" + funcName + "(in isolates.FunctionArgs) (*isolates.Value, error) {\n"
								code = code + "  if resolver, err := in.Context.NewResolver(in.ExecutionContext); err != nil {\n"
								code = code + "    return nil, err\n"
								code = code + "  } else {\n"
								code = code + "    in.Background(func(in isolates.FunctionArgs) {\n"
								code = code + argsCode
								code = code + invocation + "\n"
								code = code + "    })\n\n"
								code = code + "    if len(in.Args) > 0 {\n"
								code = code + "      callback := in.Arg(in.ExecutionContext, len(in.Args) - 1)\n"
								code = code + "      if callback.IsKind(isolates.KindFunction) {\n"
								code = code + "        return nil, resolver.ToCallback(in.ExecutionContext, callback)\n"
								code = code + "      }\n"
								code = code + "    }\n"
								code = code + "    return nil, nil\n"
								code = code + "  }\n"
								code = code + "}"

								fns = append(fns, code)
							}
						}
					}
				} else if args[0] == "get" {

					if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
						if funcDecl.Recv.NumFields() == 1 {
							if funcDecl.Type.Results.NumFields() <= 2 {
								recv := funcDecl.Recv.List[0].Type
								ret := funcDecl.Type.Results.List[0].Type

								addImport(recv)

								typeStart := recv.Pos()
								typeEnd := recv.End()
								recvTypeName := file[typeStart-offset : typeEnd-offset]
								recvID := funcDecl.Recv.List[0].Names[0].Name

								typeStart = ret.Pos()
								typeEnd = ret.End()
								// retTypeName := file[typeStart:typeEnd]

								code := ""

								funcName := funcDecl.Name.Name

								if len(args) > 1 {
									funcName = args[1]
								}

								funcName = strings.ToUpper(funcName[0:1]) + funcName[1:]

								argsCode := ""
								invocation := recvID + "." + funcDecl.Name.Name + "("
								invocationParams := []string{}

								argsOffset := 0
								for i, argNode := range funcDecl.Type.Params.List {
									typeName := file[argNode.Type.Pos()-offset : argNode.Type.End()-offset]

									if typeName == "isolates.RuntimeFunctionArgs" {
										invocationParams = append(invocationParams, "rin")
										runtimeArgs = true
										argsCode = argsCode + runtimeStanza2
										argsOffset++
									} else if typeName == "isolates.FunctionArgs" {
										invocationParams = append(invocationParams, "in")
										argsOffset++
									} else if typeName == "context.Context" {
										invocationParams = append(invocationParams, "in.ExecutionContext")
										argsOffset++
									} else if strings.HasPrefix(typeName, "...") {
										typeName = typeName[3:]
										addImport(argNode.Type.(*ast.Ellipsis).Elt)

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + fmt.Sprintf("  %s := in.Args[%d:]\n\n", argName, i-argsOffset)
										invocationParams = append(invocationParams, fmt.Sprintf("%s...", argName))
									} else {

										addImport(argNode.Type)
										imports["reflect"] = "reflect"

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + "  var " + argName + " " + typeName + "\n"
										argsCode = argsCode + fmt.Sprintf("  if v, err := in.Arg(in.ExecutionContext, %d).Unmarshal(in.ExecutionContext, reflect.TypeOf(&"+argName+").Elem()); err != nil {\n", i-argsOffset)
										argsCode = argsCode + "    return nil, err\n"
										argsNilCheck := ""
										if strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "[]") {
											argsNilCheck = "if v != nil "
										}
										argsCode = argsCode + "  } else " + argsNilCheck + "{\n"
										argsCode = argsCode + "    " + argName + " = v.Interface().(" + typeName + ")\n"
										argsCode = argsCode + "  }\n\n"

										invocationParams = append(invocationParams, argName)
									}
								}

								invocation += strings.Join(invocationParams, ", ") + ")"

								if len(funcDecl.Type.Results.List) == 2 && file[funcDecl.Type.Results.List[1].Type.Pos()-offset:funcDecl.Type.Results.List[1].Type.End()-offset] == "error" {
									invocation = "  if result, err := " + invocation + "; err != nil {\n"
									invocation = invocation + "    return nil, err\n"
									invocation = invocation + "  } else {\n"
									if len(decorators) > 0 {
										invocation = invocation + fmt.Sprintf("  return result.%s(in)\n", decorators[0])
									} else {
										invocation = invocation + "    return in.Context.Create(in.ExecutionContext, result)\n"
									}
									invocation = invocation + "  }"
								} else if len(funcDecl.Type.Results.List) == 1 {
									invocation = "  result := " + invocation + "\n"
									if len(decorators) > 0 {
										invocation = invocation + fmt.Sprintf("  return result.%s(in)\n", decorators[0])
									} else {
										invocation = invocation + "  return in.Context.Create(in.ExecutionContext, result)"
									}
								} else {
									panic(fmt.Errorf("cannot export type: %v", funcDecl.Type.Results))
								}

								code = code + "func (" + recvID + " " + recvTypeName + ") V8Get" + funcName + "(in isolates.GetterArgs) (*isolates.Value, error) {\n"
								code = code + argsCode
								code = code + invocation + "\n"
								code = code + "}"

								fns = append(fns, code)
							}
						}
					}
				} else if args[0] == "set" {
					if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
						if funcDecl.Recv.NumFields() == 1 {
							if funcDecl.Type.Results.NumFields() == 1 {
								recv := funcDecl.Recv.List[0].Type
								ret := funcDecl.Type.Results.List[0].Type

								addImport(recv)
								typeStart := recv.Pos()
								typeEnd := recv.End()
								recvTypeName := file[typeStart-offset : typeEnd-offset]
								recvID := funcDecl.Recv.List[0].Names[0].Name

								typeStart = ret.Pos()
								typeEnd = ret.End()
								// retTypeName := file[typeStart:typeEnd]

								code := ""

								funcName := funcDecl.Name.Name

								if len(args) > 1 {
									funcName = args[1]
								}

								funcName = strings.ToUpper(funcName[0:1]) + funcName[1:]

								argsCode := ""
								invocation := recvID + "." + funcDecl.Name.Name + "("
								invocationParams := []string{}

								argsOffset := 0
								for i, argNode := range funcDecl.Type.Params.List {
									typeName := file[argNode.Type.Pos()-offset : argNode.Type.End()-offset]

									if typeName == "isolates.RuntimeFunctionArgs" {
										invocationParams = append(invocationParams, "rin")
										runtimeArgs = true
										argsCode = argsCode + runtimeStanza2
										argsOffset++
									} else if typeName == "isolates.FunctionArgs" {
										invocationParams = append(invocationParams, "in")
										argsOffset++
									} else if typeName == "context.Context" {
										invocationParams = append(invocationParams, "in.ExecutionContext")
										argsOffset++
									} else if strings.HasPrefix(typeName, "...") {
										typeName = typeName[3:]
										addImport(argNode.Type.(*ast.Ellipsis).Elt)

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + fmt.Sprintf("  %s := in.Args[%d:]\n\n", argName, i-argsOffset)
										invocationParams = append(invocationParams, fmt.Sprintf("%s...", argName))
									} else {

										addImport(argNode.Type)
										imports["reflect"] = "reflect"

										argName := fmt.Sprintf("args%d", i-argsOffset)
										if len(argNode.Names) > 0 {
											argName = argNode.Names[0].Name
										}

										argsCode = argsCode + "  var " + argName + " " + typeName + "\n"
										argsCode = argsCode + "  if v, err := in.Value.Unmarshal(in.ExecutionContext, reflect.TypeOf(&" + argName + ").Elem()); err != nil {\n"
										argsCode = argsCode + "    return err\n"
										argsNilCheck := ""
										if strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "[]") {
											argsNilCheck = "if v != nil "
										}
										argsCode = argsCode + "  } else " + argsNilCheck + "{\n"
										argsCode = argsCode + "    " + argName + " = v.Interface().(" + typeName + ")\n"
										argsCode = argsCode + "  }\n\n"

										invocationParams = append(invocationParams, argName)
									}
								}

								invocation += strings.Join(invocationParams, ", ") + ")"

								if len(funcDecl.Type.Results.List) == 1 && file[funcDecl.Type.Results.List[0].Type.Pos()-offset:funcDecl.Type.Results.List[0].Type.End()-offset] == "error" {
									invocation = "  if err := " + invocation + "; err != nil {\n"
									invocation = invocation + "    return err\n"
									invocation = invocation + "  } else {\n"
									invocation = invocation + "    return nil\n"
									invocation = invocation + "  }"
								} else if len(funcDecl.Type.Results.List) == 1 {
									invocation = "  " + invocation + "\n"
									invocation = "  return nil"
								} else {
									panic(fmt.Errorf("cannot export type: %v", funcDecl.Type.Results))
								}

								code = code + "func (" + recvID + " " + recvTypeName + ") V8Set" + funcName + "(in isolates.SetterArgs) error {\n"
								code = code + argsCode
								code = code + invocation + "\n"
								code = code + "}"

								fns = append(fns, code)
							}
						}
					}
				} else if args[0] == "event" {
					if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
						if funcDecl.Recv.NumFields() == 1 {
							if funcDecl.Type.Results.NumFields() == 1 {
								recv := funcDecl.Recv.List[0].Type

								typeStart := recv.Pos()
								typeEnd := recv.End()
								recvTypeName := file[typeStart-offset : typeEnd-offset]

								if _, ok := events[recvTypeName]; !ok {
									events[recvTypeName] = map[string]*ast.FuncDecl{}
								}

								events[recvTypeName][args[1]] = funcDecl
							}
						}
					}
				}
			}
		}
	}

	// for recvTypeName, recvEvents := range events {
	// 	first := true
	// 	code := ""
	// 	var recvID string

	// 	for event, funcDecl := range recvEvents {
	// 		if first {
	// 			first = false
	// 			recvID = funcDecl.Recv.List[0].Names[0].Name
	// 			addImport(funcDecl.Recv.List[0].Type)

	// 			code = "func (" + recvID + " " + recvTypeName + ") V8FuncOn(in isolates.FunctionArgs) (*isolates.Value, error) {\n"
	// 			code = code + "  listener := in.Arg(in.ExecutionContext, 1)\n\n"
	// 			code = code + "  if event, err := in.Arg(in.ExecutionContext, 0).String(in.ExecutionContext); err != nil {\n"
	// 			code = code + "    return nil, err\n"
	// 			code = code + "  } else {\n"
	// 			code = code + "    switch(event) {\n"
	// 		}

	// 		argsCode := ""
	// 		invocationParams := []string{"in.ExecutionContext", "in.This"}

	// 		argumentList := []string{}

	// 		for i, argNode := range funcDecl.Type.Params.List[0].Type.(*ast.FuncType).Params.List {
	// 			typeName := file[argNode.Type.Pos()-offset : argNode.Type.End()-offset]
	// 			addImport(argNode.Type)

	// 			argName := fmt.Sprintf("arg%d", i)
	// 			if len(argNode.Names) > 0 {
	// 				argName = argNode.Names[0].Name
	// 			}

	// 			argumentList = append(argumentList, argName+" "+typeName)

	// 			argsCode = argsCode + "          var " + argName + "_v8 *isolates.Value\n"
	// 			argsCode = argsCode + "          if v, err := in.Context.Create(in.ExecutionContext, " + argName + "); err != nil {\n"
	// 			argsCode = argsCode + "            return err\n"
	// 			argsCode = argsCode + "          } else {\n"
	// 			argsCode = argsCode + "            " + argName + "_v8 = v\n"
	// 			argsCode = argsCode + "          }\n\n"

	// 			invocationParams = append(invocationParams, argName+"_v8")
	// 		}

	// 		code = code + "      case \"" + event + "\":\n"
	// 		code = code + "        remover := " + recvID + "." + funcDecl.Name.Name + "(func(" + strings.Join(argumentList, ", ") + ") error {\n"
	// 		code = code + argsCode
	// 		code = code + "          if result, err := listener.Call(" + strings.Join(invocationParams, ", ") + "); err != nil {\n"
	// 		code = code + "            return err\n"
	// 		code = code + "          } else if _, err := result.Await(in.ExecutionContext); err != nil {\n"
	// 		code = code + "            return err\n"
	// 		code = code + "          } else {\n"
	// 		code = code + "            return nil\n"
	// 		code = code + "          }\n"
	// 		code = code + "        })\n"
	// 		code = code + "        return in.Context.Create(in.ExecutionContext, remover)\n"
	// 	}

	// 	imports["fmt"] = "fmt"

	// 	code = code + "    }\n\n"
	// 	code = code + "    return nil, fmt.Errorf(\"unknown event: %s\", event)\n"
	// 	code = code + "  }\n"
	// 	code = code + "}"

	// 	fns = append(fns, code)
	// }

	if len(imports) > 0 {
		code = ")\n\n" + code
		for imp, name := range imports {
			code = "  " + name + " \"" + imp + "\"\n" + code
		}
		code = "import (\n" + code
	}

	code = "package " + a.Name.Name + "\n\n" + code
	code = "// this file is auto-generated by github.com/grexie/isolates, DO NOT EDIT\n\n" + code

	code = code + "  return nil, nil\n"
	code = code + "})\n\n"
	return code + strings.Join(fns, "\n\n"), nil
}

func FindDeclarationCommentTags(file string, tags []string, a *ast.File) ([]*TaggedNode, error) {
	decls := []*TaggedNode{}
	for _, comments := range a.Comments {
		declTags := []Tag{}
		for _, comment := range comments.List {
			for _, tag := range tags {
				text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
				if strings.HasPrefix(text, tag+":") {
					declTags = append(declTags, Tag{
						Name: tag,
						Text: strings.TrimSpace(strings.TrimPrefix(text, tag+":")),
					})
				}
			}
		}

		if len(declTags) > 0 {
			var offset token.Pos = 0
			offset = a.FileStart
			endPos := comments.End()

			ast.Inspect(a, func(node ast.Node) bool {
				if node == nil {
					return true
				}

				if node.Pos()-offset > endPos-offset && strings.TrimSpace(file[endPos-offset:node.Pos()-offset]) == "" {
					d := &TaggedNode{
						Tags: declTags,
						Node: node,
					}
					decls = append(decls, d)

					return false
				}
				return true
			})

		}
	}

	return decls, nil
}

type Tag struct {
	Name string
	Text string
}

func (t *Tag) String() string {
	return fmt.Sprintf("%s:%s", t.Name, t.Text)
}

type TaggedNode struct {
	Tags []Tag
	Node ast.Node
}

func (t *TaggedNode) String() string {
	out := []string{}
	for _, tag := range t.Tags {
		out = append(out, tag.String())
	}

	node := ""

	if v, ok := t.Node.(*ast.File); ok {
		node = fmt.Sprintf("go package %s", v.Name)
	} else if v, ok := t.Node.(*ast.TypeSpec); ok {
		node = fmt.Sprintf("go struct %s", v)
	} else if v, ok := t.Node.(*ast.Field); ok {
		node = fmt.Sprintf("go field %s %s", v.Names[0], v.Type.(*ast.SelectorExpr).Sel)
	} else if v, ok := t.Node.(*ast.FuncDecl); ok {
		node = fmt.Sprintf("go func %s", v.Name)
	} else {
		node = fmt.Sprintf("%s", t.Node)
	}

	return fmt.Sprintf("%s\n%v", strings.Join(out, "\n"), node)
}
