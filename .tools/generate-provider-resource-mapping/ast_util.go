package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

func functionDeclOfMethod(pkg *packages.Package, nt *types.Named, methodName string) (*ast.FuncDecl, error) {
	fileMap := map[*token.File]*ast.File{}
	for _, f := range pkg.Syntax {
		fileMap[pkg.Fset.File(f.Pos())] = f
	}

	for i := 0; i < nt.NumMethods(); i++ {
		method := nt.Method(i)
		if method.Name() != methodName {
			continue
		}

		f := fileMap[pkg.Fset.File(method.Pos())]
		// Lookup the function declaration from the method identifier position.
		// The returned enclosing interval starts from the identifier node, then the function declaration node.
		nodes, _ := astutil.PathEnclosingInterval(f, method.Pos(), method.Pos())
		fdecl := nodes[1].(*ast.FuncDecl)
		return fdecl, nil
	}

	return nil, fmt.Errorf("failed to find the method %q in type %q", methodName, nt.Obj().Name())
}
