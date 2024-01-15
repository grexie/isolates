package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path"
	"strings"
)

const (
	kTypeMethod int = iota
	kTypeAsyncMethod
	kTypeCallbackMethod
)

type accessor struct {
	Name   string
	Type   string
	Getter bool
	Setter bool
}

type definition struct {
	filename string
	node     *TaggedNode
	file     string
	a        *ast.File
	nodes    []*TaggedNode
}

type context struct {
	filename     string
	file         *ast.File
	parseResults map[string]map[string]*ast.Package
	imports      map[string]map[string]bool
	files        map[string]string
	nodes        map[string][]*TaggedNode
	definitions  map[string]*definition
}

func (c *context) ConvertToJavaScript(t string) (string, bool, error) {
	origType := t

	var dir string

	t = strings.TrimPrefix(t, "*")

	switch t {
	case "any":
		return "any", true, nil
	case "isolates.Value":
		return "any", true, nil
	case "bool":
		return "boolean", true, nil
	case "int":
		return "number", true, nil
	case "int32":
		return "number", true, nil
	case "int64":
		return "number", true, nil
	case "uint":
		return "number", true, nil
	case "uint32":
		return "number", true, nil
	case "uint64":
		return "number", true, nil
	case "float":
		return "number", true, nil
	case "float32":
		return "number", true, nil
	case "float64":
		return "number", true, nil
	case "string":
		return "string", true, nil
	case "uintptr":
		return "number", true, nil
	case "time.Time":
		return "Date", true, nil
	case "time.Duration":
		return "number", true, nil
	}

	if strings.HasPrefix(t, "map[string]") {
		if valueType, _, err := c.ConvertToJavaScript(strings.TrimPrefix(t, "map[string]")); err != nil {
			return "", false, err
		} else {
			return fmt.Sprintf("Record<%s, %s>", "string", valueType), true, nil
		}
	}

	if strings.HasPrefix(t, "[]") {
		if valueType, _, err := c.ConvertToJavaScript(strings.TrimPrefix(t, "[]")); err != nil {
			return "", false, err
		} else {
			return fmt.Sprintf("%s[]", valueType), true, nil
		}
	}

	if strings.Contains(t, ".") {
		for _, imp := range c.file.Imports {
			var impName string
			if imp.Name != nil {
				impName = imp.Name.Name
			} else {
				impPath := strings.Split(strings.Trim(imp.Path.Value, "\""), "/")
				impName = impPath[len(impPath)-1]
			}

			if impName == strings.Split(t, ".")[0] {
				if p, err := build.Default.Import(strings.Trim(imp.Path.Value, "\""), c.filename, build.FindOnly); err != nil {
					return "", false, err
				} else {
					dir = p.Dir
				}
			}
		}

		t = strings.Split(t, ".")[1]
	} else {
		dir = path.Dir(c.filename)
	}

	if dir == "" {
		dir = "."
	}

	if _, ok := c.parseResults[dir]; !ok {
		var fileSet token.FileSet
		if result, err := parser.ParseDir(&fileSet, dir, nil, parser.ParseComments|parser.AllErrors); err != nil {
			return "", false, err
		} else {
			c.parseResults[dir] = result
		}
	}

	for _, pkg := range c.parseResults[dir] {
		for filename, file := range pkg.Files {
			if _, ok := c.files[filename]; !ok {
				if body, err := os.ReadFile(filename); err != nil {
					return "", false, err
				} else if nodes, err := FindDeclarationCommentTags(string(body), []string{"js", "ts"}, file); err != nil {
					return "", false, err
				} else {
					c.files[filename] = string(body)
					c.nodes[filename] = nodes
				}
			}

			body := c.files[filename]
			nodes := c.nodes[filename]

			var pkgName string

			for _, node := range nodes {
				var isAlias bool
				var isConstructor bool
				var constructorName string
				var exportName *string
				var isPrimitive bool

				for _, tag := range node.Tags {
					args := strings.Split(tag.Text, " ")

					if tag.Name == "ts" {
						if args[0] == "export" {
							if args[1] == "type" || args[1] == "interface" {
								if decl, ok := node.Node.(*ast.GenDecl); ok && len(decl.Specs) == 1 {
									if decl, ok := decl.Specs[0].(*ast.TypeSpec); ok && decl.Name.Name == t {
										constructorName = args[2]
										isConstructor = true
										isAlias = false
										isPrimitive = false
									}
								}
							}
						}
					}

					if tag.Name == "js" {
						if args[0] == "package" {
							pkgName = args[1]
						}

						if args[0] == "alias" {
							if decl, ok := node.Node.(*ast.GenDecl); ok && len(decl.Specs) == 1 {
								if decl, ok := decl.Specs[0].(*ast.TypeSpec); ok && decl.Name.Name == t {
									resolvedTypeName := args[1]
									context := *c
									context.filename = filename
									context.file = file
									if c, p, err := context.ConvertToJavaScript(resolvedTypeName); err != nil {
										return "", false, err
									} else {
										constructorName = c
										isConstructor = !p
										isAlias = true
										isPrimitive = p
									}
								}
							}
						}

						if args[0] == "class" {
							if len(args) > 1 {
								constructorName = args[1]
								isConstructor = true
								isAlias = false
								isPrimitive = false
							} else {
								if decl, ok := node.Node.(*ast.GenDecl); ok && len(decl.Specs) == 1 {
									if decl, ok := decl.Specs[0].(*ast.TypeSpec); ok && decl.Name.Name == t {
										constructorName = t
										isConstructor = true
										isAlias = false
										isPrimitive = false
									}
								}
							}
						}

						if args[0] == "export" && args[1] != "default" {
							exportName = &args[1]
						}
					}
				}

				if !isConstructor {
					if funcDecl, ok := node.Node.(*ast.FuncDecl); ok && funcDecl.Type.Results.NumFields() >= 1 {
						constructorName = strings.Trim(body[funcDecl.Type.Results.List[0].Type.Pos()-file.FileStart:funcDecl.Type.Results.List[0].Type.End()-file.FileStart], "[]*")

						if constructorName != t {
							continue
						}

						for _, tag := range node.Tags {
							args := strings.Split(tag.Text, " ")

							if tag.Name == "js" {
								if args[0] == "constructor" {
									if len(args) > 1 {
										constructorName = args[1]
									}
									isConstructor = true
								}
							}
						}
					}

					if exportName != nil {
						constructorName = *exportName
					}
				}

				if (isConstructor || isAlias && !isPrimitive) && c.filename != filename {
					if _, ok := c.imports[pkgName]; !ok {
						c.imports[pkgName] = map[string]bool{}
					}

					c.imports[pkgName][constructorName] = true
				}

				if isConstructor || isAlias && !isPrimitive {
					c.definitions[constructorName] = &definition{
						filename: filename,
						node:     node,
						file:     body,
						a:        file,
						nodes:    nodes,
					}
				}

				if isConstructor || isAlias {
					return constructorName, isPrimitive, nil
				}
			}
		}
	}

	for _, pkg := range c.parseResults[dir] {
		for filename, file := range pkg.Files {
			var resolvedType ast.Node

			ast.Inspect(file, func(n ast.Node) bool {
				if decl, ok := n.(*ast.TypeSpec); ok {
					if decl.Name.Name == t {
						if _, ok := decl.Type.(*ast.StructType); ok {
							return false
						} else if _, ok := decl.Type.(*ast.InterfaceType); ok {
							return false
						} else {
							resolvedType = decl.Type
						}
						return false
					}
				}
				return true
			})

			if resolvedType != nil {
				resolvedTypeName := c.files[filename][resolvedType.Pos()-file.FileStart : resolvedType.End()-file.FileStart]
				context := *c
				context.filename = filename
				context.file = file
				return context.ConvertToJavaScript(resolvedTypeName)
			}
		}
	}

	return "", false, fmt.Errorf("unable to resolve symbol: " + origType + " in: " + c.filename)
}

func GenerateTypes(filename string, file string, a *ast.File, nodes []*TaggedNode) (string, error) {
	context := context{
		filename:     filename,
		file:         a,
		parseResults: map[string]map[string]*ast.Package{},
		imports:      map[string]map[string]bool{},
		files:        map[string]string{},
		nodes:        map[string][]*TaggedNode{},
		definitions:  map[string]*definition{},
	}

	var pkgName string
	var pkgNode ast.Node
	for _, node := range nodes {
		for _, tag := range node.Tags {
			args := strings.Split(tag.Text, " ")

			if tag.Name == "js" {
				if args[0] == "package" {
					pkgName = args[1]
					pkgNode = node.Node
				}
			}
		}
	}

	code := fmt.Sprintf("/**\n * @moduleName %s\n@packageDescription\n@module %s*/\n", pkgName, pkgName)

	code += "\n/*\n"
	code += " * Solide JavaScript Engine\n"
	code += " * \n"
	code += " * Copyright (C) 2010 - 2023 Grexie\n"
	code += " * All Rights Reserved\n"
	code += " */\n\n"

	line, col := findLoc(file, pkgNode, a)
	wd, _ := os.Getwd()
	code += fmt.Sprintf("  /** @filename %s @line %d @column %d */\n", strings.TrimPrefix(path.Join(wd, filename), os.Getenv("GOTSROOT")+"/"), line, col)
	code += fmt.Sprintf("declare module \"%s\" {\n", pkgName)

	moduleCode := ""

	for _, node := range nodes {
		for _, tag := range node.Tags {
			args := strings.Split(tag.Text, " ")

			if tag.Name == "ts" {
				if _, ok := node.Node.(*ast.FuncDecl); !ok {
					if c, ok := emitTypeScript(context, filename, file, a, node); !ok {
						return "", fmt.Errorf("unable to write node")
					} else {
						code += c
					}
				}
			}

			if tag.Name == "js" {
				if args[0] == "constructor" {
					if c, err := parseConstructor(context, filename, node, file, a, nodes, nil, false); err != nil {
						return "", err
					} else {
						moduleCode += c
					}
				}

				if args[0] == "class" {
					if c, err := parseConstructor(context, filename, node, file, a, nodes, nil, false); err != nil {
						return "", err
					} else {
						moduleCode += c
					}
				}

				if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
					if funcDecl.Recv.NumFields() == 0 {
						if args[0] == "method" {
							if c, _, err := parseMethod(context, filename, node, file, a, nodes, kTypeMethod, false); err != nil {
								return "", err
							} else {
								moduleCode += c
							}
						}

						if args[0] == "async-method" {
							if c, _, err := parseMethod(context, filename, node, file, a, nodes, kTypeAsyncMethod, false); err != nil {
								return "", err
							} else {
								moduleCode += c
							}
						}

						if args[0] == "callback-method" {
							if c, _, err := parseMethod(context, filename, node, file, a, nodes, kTypeCallbackMethod, false); err != nil {
								return "", err
							} else {
								moduleCode += c
							}
						}
					}
				}
			}
		}

	}

	for file, imp := range context.imports {
		out := []string{}
		for i := range imp {
			out = append(out, i)
		}
		code += fmt.Sprintf("  import type { %s } from \"%s\";\n", strings.Join(out, ", "), file)

	}
	code += "\n"

	code += moduleCode
	code += "}\n"

	return code, nil
}

func findLoc(file string, a ast.Node, f *ast.File) (int, int) {
	lines := strings.Split(file[:a.Pos()-f.FileStart], "\n")
	return len(lines), len(lines[len(lines)-1])
}

func parseConstructor(context context, filename string, node *TaggedNode, file string, a *ast.File, nodes []*TaggedNode, keys map[string]bool, isInterfaceImplementation bool) (string, error) {
	code := ""

	var exports []string
	var constructorName *string
	var constructorDecl *ast.FuncDecl
	var constructorType string
	var isConstructor bool

	constructorDecl, isConstructor = node.Node.(*ast.FuncDecl)

	if isConstructor {
		constructorType = strings.Trim(file[constructorDecl.Type.Results.List[0].Type.Pos()-a.FileStart:constructorDecl.Type.Results.List[0].Type.End()-a.FileStart], "[]*")
	} else {
		constructorType = node.Node.(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Name.Name
		constructorName = &constructorType
	}

	for _, tag := range node.Tags {
		args := strings.Split(tag.Text, " ")

		if tag.Name == "js" {
			if args[0] == "constructor" {
				if len(args) > 1 {
					constructorName = &args[1]
				}
			}

			if args[0] == "class" {
				if len(args) > 1 {
					constructorName = &args[1]
				}
			}

			if args[0] == "export" {
				exports = append(exports, args[1])
			}
		}
	}

	var constructorTypeDecl ast.Expr

	if isConstructor {
		ctype := strings.Trim(file[constructorDecl.Type.Results.List[0].Type.Pos()-a.FileStart:constructorDecl.Type.Results.List[0].Type.End()-a.FileStart], "[]*")
		ast.Inspect(a, func(n ast.Node) bool {
			if decl, ok := n.(*ast.TypeSpec); ok && decl.Name.Name == ctype {
				if decl, ok := decl.Type.(*ast.StructType); ok {
					constructorTypeDecl = decl
				}
				return false
			}

			return true
		})
	} else {
		spec := node.Node.(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
		constructorTypeDecl = spec.Type
	}

	if constructorName == nil {
		funcDecl := node.Node.(*ast.FuncDecl)
		ctype := file[funcDecl.Type.Results.List[0].Type.Pos()-a.FileStart : funcDecl.Type.Results.List[0].Type.End()-a.FileStart]
		ctype = strings.Trim(ctype, "[]*")
		constructorName = &ctype
	}

	line, col := findLoc(file, node.Node, a)
	wd, _ := os.Getwd()

	var extendsType *string
	interfaceTypes := []string{}

	if _struct, ok := constructorTypeDecl.(*ast.StructType); ok {
		for _, field := range _struct.Fields.List {
			if len(field.Names) == 0 {
				if t, _, err := context.ConvertToJavaScript(strings.Trim(file[field.Type.Pos()-a.FileStart:field.Type.End()-a.FileStart], "[]*")); err != nil {
					continue
				} else if t != "" {
					if extendsType == nil {
						extendsType = &t
					} else {
						interfaceTypes = append(interfaceTypes, t)
					}
				}
			}
		}
	}

	var extends string
	if extendsType != nil {
		extends = fmt.Sprintf("extends %s ", *extendsType)
	}

	var implements string
	if len(interfaceTypes) > 0 {
		implements = fmt.Sprintf("implements %s ", strings.Join(interfaceTypes, ", "))
	}

	if !isInterfaceImplementation {
		code += fmt.Sprintf("  /** @filename %s @line %d @column %d */\n", strings.TrimPrefix(path.Join(wd, filename), os.Getenv("GOTSROOT")+"/"), line, col)
		code += fmt.Sprintf("  class %s %s%s{\n", *constructorName, extends, implements)
	}

	accessors := map[string]accessor{}
	if keys == nil {
		keys = map[string]bool{}
	}

	for _, node := range nodes {
		if funcDecl, ok := node.Node.(*ast.FuncDecl); ok {
			if funcDecl.Recv.NumFields() == 1 {
				recvType := file[funcDecl.Recv.List[0].Type.Pos()-a.FileStart : funcDecl.Recv.List[0].Type.End()-a.FileStart]
				if recvType == constructorType || recvType == "*"+constructorType {

					funcName := funcDecl.Name.Name

					for _, tag := range node.Tags {
						args := strings.Split(tag.Text, " ")

						if tag.Name == "js" {
							if args[0] == "get" {
								var getter string
								if (len(args)) > 1 {
									getter = args[1]
								} else {
									n := strings.ToLower(funcName[0:1]) + funcName[1:]
									getter = n
								}
								if _, ok := accessors[getter]; !ok {
									accessors[getter] = accessor{}
								}
								entry := accessors[getter]
								entry.Name = getter
								entry.Type = file[funcDecl.Type.Results.List[0].Type.Pos()-a.FileStart : funcDecl.Type.Results.List[0].End()-a.FileStart]
								entry.Getter = true
								accessors[getter] = entry
							}

							if args[0] == "set" {
								var setter string
								if (len(args)) > 1 {
									setter = args[1]
								} else {
									n := strings.ToLower(funcName[0:1]) + funcName[1:]
									setter = n
								}
								if _, ok := accessors[setter]; !ok {
									accessors[setter] = accessor{}
								}
								entry := accessors[setter]
								entry.Name = setter
								entry.Type = file[funcDecl.Type.Params.List[0].Type.Pos()-a.FileStart : funcDecl.Type.Params.List[0].End()-a.FileStart]
								entry.Setter = true
								accessors[setter] = entry
							}

							if args[0] == "method" {
								if c, name, err := parseMethod(context, filename, node, file, a, nodes, kTypeMethod, true); err != nil {
									return "", err
								} else {
									keys[name] = true
									code += c
								}
							}

							if args[0] == "async-method" {
								if c, name, err := parseMethod(context, filename, node, file, a, nodes, kTypeAsyncMethod, true); err != nil {
									return "", err
								} else {
									keys[name] = true
									code += c
								}
							}

							if args[0] == "callback-method" {
								if c, name, err := parseMethod(context, filename, node, file, a, nodes, kTypeCallbackMethod, true); err != nil {
									return "", err
								} else {
									keys[name] = true
									code += c
								}
							}
						}
					}
				}
			}
		}
	}

	for _, accessor := range accessors {
		if c, name, err := parseField(context, filename, node, file, a, nodes, accessor); err != nil {
			return "", err
		} else {
			keys[name] = true
			code += c
		}
	}

	for _, iface := range interfaceTypes {
		if definition, ok := context.definitions[iface]; !ok {
			return "", fmt.Errorf("cannot resolve interface: " + iface + " for: " + *constructorName)
		} else if c, err := parseConstructor(context, definition.filename, definition.node, definition.file, definition.a, definition.nodes, keys, true); err != nil {
			return "", err
		} else {
			code += c
		}
	}

	if !isInterfaceImplementation {
		code += "  }\n"

		for _, export := range exports {
			if export == "default" {
				code += fmt.Sprintf("  export default %s;\n", *constructorName)
			} else if export == *constructorName {
				code += fmt.Sprintf("  export { %s };\n", *constructorName)
			} else {
				code += fmt.Sprintf("  export const %s = %s;\n", export, *constructorName)
			}
		}
	}

	return code, nil
}

func emitTypeScript(context context, filename string, file string, a *ast.File, node *TaggedNode) (string, bool) {
	out := []string{}
	hasTypeScript := false

	line, col := findLoc(file, node.Node, a)
	wd, _ := os.Getwd()

	loc := fmt.Sprintf("  /** @filename %s @line %d @column %d */\n", strings.TrimPrefix(path.Join(wd, filename), os.Getenv("GOTSROOT")+"/"), line, col)

	for _, tag := range node.Tags {
		if tag.Name == "ts" {
			out = append(out, loc+"\n"+tag.Text)
			hasTypeScript = true
		}
	}

	if hasTypeScript {
		return strings.Join(out, "\n"), true
	} else {
		return "", false
	}
}

func parseMethod(context context, filename string, node *TaggedNode, file string, a *ast.File, nodes []*TaggedNode, methodType int, isClassMethod bool) (string, string, error) {

	if code, ok := emitTypeScript(context, filename, file, a, node); ok {
		return code, "", nil
	}

	code := ""

	var export *string
	var methodName *string
	var isAsyncMethod bool
	var isCallbackMethod bool

	var returnType *string
	results := node.Node.(*ast.FuncDecl).Type.Results
	for _, tag := range node.Tags {
		args := strings.Split(tag.Text, " ")

		if tag.Name == "js" {
			if args[0] == "return" {
				if t, _, err := context.ConvertToJavaScript(args[1]); err != nil {
					return "", "", err
				} else {
					returnType = &t
				}
			}
		}
	}

	if returnType == nil && results.NumFields() > 0 {
		t := file[results.List[0].Type.Pos()-a.FileStart : results.List[0].Type.End()-a.FileStart]
		if t != "error" {
			if t, _, err := context.ConvertToJavaScript(t); err != nil {
				return "", "", err
			} else {
				returnType = &t
			}
		}
	}

	for _, tag := range node.Tags {
		args := strings.Split(tag.Text, " ")

		if tag.Name == "js" {
			if methodType == kTypeMethod && args[0] == "method" {
				if len(args) > 1 {
					methodName = &args[1]
				}
			}

			if methodType == kTypeAsyncMethod && args[0] == "async-method" {
				if len(args) > 1 {
					methodName = &args[1]
				}
			}

			if methodType == kTypeCallbackMethod && args[0] == "callback-method" {
				if len(args) > 1 {
					methodName = &args[1]
				}
			}

			if args[0] == "export" {
				export = &args[1]
			}
		}
	}

	if methodName == nil {
		funcDecl := node.Node.(*ast.FuncDecl)
		funcName := funcDecl.Name.Name
		funcName = strings.ToLower(funcName[0:1]) + funcName[1:]
		methodName = &funcName
	}

	line, col := findLoc(file, node.Node, a)
	wd, _ := os.Getwd()

	code += fmt.Sprintf("  /** @filename %s @line %d @column %d */\n", strings.TrimPrefix(path.Join(wd, filename), os.Getenv("GOTSROOT")+"/"), line, col)
	if isClassMethod {
		code += fmt.Sprintf("  %s(", *methodName)
	} else {
		code += fmt.Sprintf("  function %s(", *methodName)
	}
	code += "  )"

	if isAsyncMethod {
		if returnType == nil {
			code += ": Promise<void>"
		} else {
			code += fmt.Sprintf(": Promise<%s>", *returnType)
		}
	} else if isCallbackMethod {
		code += ": void"
	} else {
		if returnType == nil {
			code += ": void"
		} else {
			code += fmt.Sprintf(": %s", *returnType)
		}
	}

	code += ";\n"

	if !isClassMethod && export != nil {
		if *export == "default" {
			code += fmt.Sprintf("  export default %s;\n", *methodName)
		} else if *export == *methodName {
			code += fmt.Sprintf("  export { %s };\n", *methodName)
		} else {
			code += fmt.Sprintf("  export const %s = %s;\n", *export, *methodName)
		}
	}

	return code, *methodName, nil
}

func parseField(context context, filename string, node *TaggedNode, file string, a *ast.File, nodes []*TaggedNode, accessor accessor) (string, string, error) {
	code := ""

	line, col := findLoc(file, node.Node, a)
	wd, _ := os.Getwd()

	code += fmt.Sprintf("  /** @filename %s @line %d @column %d */\n", strings.TrimPrefix(path.Join(wd, filename), os.Getenv("GOTSROOT")+"/"), line, col)
	if t, _, err := context.ConvertToJavaScript(accessor.Type); err != nil {
		return "", "", err
	} else {
		if !accessor.Setter {
			code += fmt.Sprintf("    readonly %s: %s;\n", accessor.Name, t)
		} else if accessor.Getter {
			code += fmt.Sprintf("    %s: %s;\n", accessor.Name, t)
		} else {
			code += fmt.Sprintf("    set %s(value: %s);\n", accessor.Name, t)
		}

		return code, accessor.Name, nil
	}
}
