package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"strconv"
	"strings"
)

func normalizeAPIPath(p string) string {
	segs := strings.Split(p, "/")
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out = append(out, "{}")
			continue
		}
		out = append(out, strings.ToUpper(seg))
	}
	return strings.Join(out, "/")
}

func ApiPathTrack1(pkg *packages.Package, f *ast.FuncDecl) (string, error) {
	var apiPath string

	ast.Inspect(f.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		// Looking for api path and create operation kind
		case *ast.AssignStmt:
			lhs := node.Lhs
			if len(lhs) != 1 {
				return false
			}
			lIdent, ok := lhs[0].(*ast.Ident)
			if !ok {
				return false
			}
			if lIdent.Name != "preparer" {
				return false
			}
			for _, arg := range node.Rhs[0].(*ast.CallExpr).Args {
				callexpr, ok := arg.(*ast.CallExpr)
				if !ok {
					continue
				}
				fun, ok := callexpr.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				switch fun.Sel.Name {
				case "WithPath",
					"WithPathParameters":
					pathLit, ok := callexpr.Args[0].(*ast.BasicLit)
					if !ok {
						continue
					}
					apiPath, _ = strconv.Unquote(pathLit.Value)
					apiPath = normalizeAPIPath(apiPath)
				default:
					continue
				}
			}
			return false
		default:
			return true
		}
	})

	if apiPath == "" {
		return "", fmt.Errorf("failed to find the api path")
	}

	return apiPath, nil
}

func ApiPathPandora(pkg *packages.Package, f *ast.FuncDecl) (string, error) {
	var (
		err     error
		apiPath string
	)
	ast.Inspect(f.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		// Looking for api path, version and operation kind
		case *ast.AssignStmt:
			lhs := node.Lhs
			if len(lhs) != 1 {
				return false
			}
			lIdent, ok := lhs[0].(*ast.Ident)
			if !ok {
				return false
			}

			switch lIdent.Name {
			// API Path and Operation kind
			case "preparer":
				for _, arg := range node.Rhs[0].(*ast.CallExpr).Args {
					callexpr, ok := arg.(*ast.CallExpr)
					if !ok {
						continue
					}
					fun, ok := callexpr.Fun.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					switch fun.Sel.Name {
					case "WithPathParameters",
						"WithPath":
						firstArgCallExpr, ok := callexpr.Args[0].(*ast.CallExpr)
						if !ok {
							continue
						}
						sel, ok := firstArgCallExpr.Fun.(*ast.SelectorExpr)
						if !ok {
							continue
						}
						switch sel.X.(*ast.Ident).Name {
						case "id":
							// Call the id.ID() to construct the api path
							apiPath, err = apiPathFromID(pkg, sel)
							if err != nil {
								return false
							}
						case "fmt":
							// Call the fmt.Sprintf() to construct the api path
							// e.g. '"%s/eventhubs/"'
							formatString, _ := strconv.Unquote(firstArgCallExpr.Args[0].(*ast.BasicLit).Value)
							sel := firstArgCallExpr.Args[1].(*ast.CallExpr).Fun.(*ast.SelectorExpr)
							apiPath, err = apiPathFromID(pkg, sel)
							if err != nil {
								return false
							}
							apiPath = normalizeAPIPath(fmt.Sprintf(formatString, apiPath))
						default:
							panic(fmt.Sprintf("unexpected WithPath/WithPathParameters call happened at %s", pkg.Fset.Position(callexpr.Pos())))
						}
					default:
						continue
					}
				}
				return false

			default:
				return false
			}
		default:
			return true
		}
	})
	return apiPath, err
}

func apiPathFromID(pkg *packages.Package, idSelExpr *ast.SelectorExpr) (string, error) {
	idObj, ok := pkg.TypesInfo.Uses[idSelExpr.X.(*ast.Ident)]
	if !ok {
		return "", fmt.Errorf("failed to find the usage of %s in pacakge %s", pkg.Fset.Position(idSelExpr.X.Pos()), pkg.Name)
	}

	f, err := functionDeclOfMethod(pkg, idObj.Type().(*types.Named), idSelExpr.Sel.Name)
	if err != nil {
		return "", fmt.Errorf("getting the '%s' function syntax for %s", idSelExpr.Sel.Name, idObj.Type().String())
	}
	apiPath, _ := strconv.Unquote(f.Body.List[0].(*ast.AssignStmt).Rhs[0].(*ast.BasicLit).Value)
	apiPath = strings.ReplaceAll(apiPath, "%s", "{}")
	apiPath = normalizeAPIPath(apiPath)
	return apiPath, nil
}

// findApiPathFromDeleteFunc searches for the API path from the delete function of a TF resource.
// In case there is no API path found, it returns empty string.
func findApiPathFromDeleteFunc(pkg *Package, deleteFunc *ssa.Function) (string, error) {
	deleteFuncNode, ok := pkg.CallGraph.Nodes[deleteFunc]
	if !ok {
		return "", fmt.Errorf(`failed to find the function %s in the callgraph`, pkg.Position(deleteFunc))
	}
	callees := DeduplicateInterfaceMethod(AllCalleesOf(deleteFuncNode))

	var apiPaths []string
	for callee := range callees {
		if !strings.Contains(callee.Name(), "Delete") {
			continue
		}
		if !pTrack1.MatchString(callee.Pkg().Path()) && !pEmbeddedSDK.MatchString(callee.Pkg().Path()) {
			continue
		}

		calleeType := callee.Type().(*types.Signature)
		calleePkg := pkg.GoPackages[callee.Pkg().Path()]
		calleeRecv := calleeType.Recv()
		if calleeRecv == nil {
			// In both SDKs, the CRUD are defined as method on some named receiver.
			continue
		}
		calleeRecvType := calleeRecv.Type().(*types.Named)

		// Identify the callee is from Azure GO SDK Track1 or from Pandora SDK.
		// This is done based on the naming convention of the preparer method.
		// In Track1: callee.Name() + "Preparer"
		// In Pandora: "preparerFor" + strings.TrimSuffix(callee.Name(), "ThenPoll")
		track1Name := callee.Name() + "Preparer"
		pandoraName := "preparerFor" + strings.TrimSuffix(callee.Name(), "ThenPoll")
		switch {
		case hasMethod(calleeRecvType, track1Name):
			calleeDecl, err := functionDeclOfMethod(calleePkg, calleeRecvType, track1Name)
			if err != nil {
				return "", fmt.Errorf(`getting the %q function syntax of %s: %w`, track1Name, calleeRecvType.String(), err)
			}

			apiPath, err := ApiPathTrack1(calleePkg, calleeDecl)
			if err != nil {
				return "", fmt.Errorf("finding api path from function %q: %w", pkg.Position(calleeDecl), err)
			}
			apiPaths = append(apiPaths, apiPath)
		case hasMethod(calleeRecvType, pandoraName):
			calleeDecl, err := functionDeclOfMethod(calleePkg, calleeRecvType, pandoraName)
			if err != nil {
				return "", fmt.Errorf(`getting the %q function syntax of %s: %w`, pandoraName, calleeRecvType.String(), err)
			}

			apiPath, err := ApiPathPandora(calleePkg, calleeDecl)
			if err != nil {
				return "", fmt.Errorf("finding api path from function %q: %w", pkg.Position(callee), err)
			}
			apiPaths = append(apiPaths, apiPath)
		}
	}
	if len(apiPaths) == 0 {
		return "", fmt.Errorf("no API path found from the delete method of %s", pkg.Position(deleteFunc))
	} else if len(apiPaths) > 1 {
		return "", fmt.Errorf("more than one API paths found from the delete method of %s", pkg.Position(deleteFunc))
	}
	return apiPaths[0], nil
}
