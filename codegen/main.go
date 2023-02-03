package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"os"
	"strings"
)

func main() {
	// generate rtti
	// generate typescript types
	// generate adapters for different interfaces
	// magic comments in the parser
	var fileSet token.FileSet
	if wd, err := os.Getwd(); err != nil {
		panic(err)
	} else if packages, err := parser.ParseDir(&fileSet, wd, nil, parser.ParseComments|parser.AllErrors); err != nil {
		panic(err)
	} else {
		tags := []string{"js"}
		for _, pkg := range packages {
			for f, a := range pkg.Files {
				if text, err := os.ReadFile(f); err != nil {
					panic(err)
				} else {
					nodes := FindDeclarationCommentTags(string(text), tags, a)

					for _, node := range nodes {
						fmt.Println(node)
					}
				}

			}
		}
	}
}

func FindDeclarationCommentTags(file string, tags []string, a *ast.File) []*TaggedNode {
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
			endPos := comments.End()
			ast.Inspect(a, func(node ast.Node) bool {
				if node == nil {
					return false
				}

				if node.Pos() > endPos && strings.TrimSpace(file[endPos:node.Pos()]) == "" {
					record := false
					if _, ok := node.(*ast.StructType); ok {

						record = true
					} else if _, ok := node.(*ast.File); ok {
						record = true
					} else if _, ok := node.(*ast.FuncDecl); ok {
						record = true
					} else if _, ok := node.(*ast.Field); ok {
						record = true
					}

					if record {
						d := &TaggedNode{
							Tags: declTags,
							Node: node,
						}
						decls = append(decls, d)
						return false
					} else {
						endPos = node.End()
					}
				}
				return true
			})
		}

	}

	return decls
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
