package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/ssa"
	"strconv"
)

// handleUntypedRegistration analyze the code of the untyped registration to get a mapping from TF resource type to Azure API path.
func handleUntypedRegistration(pkg *Package, regObj types.Object) (map[string]string, error) {
	// TF resource type -> Azure api path
	resourceMapping := map[string]string{}

	// Iterate each sdk.Resource as defined in the SupportedResources() method.
	nt, ok := regObj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%s is not a named type", regObj.Type())
	}
	f, err := functionDeclOfMethod(pkg.GoPackage, nt, "SupportedResources")
	if err != nil {
		return nil, err
	}

	if len(f.Body.List) != 1 {
		return nil, fmt.Errorf(`the function body of %s does not only contain one return`, pkg.Position(f))
	}

	retValue, ok := f.Body.List[0].(*ast.ReturnStmt).Results[0].(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf(`return value of function %s is not a composite literal`, pkg.Position(f))
	}

	for i, elt := range retValue.Elts {
		kvexpr, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return nil, fmt.Errorf(`the %dth element of the composite literal of %s is not KeyValueExpr`, i, pkg.Position(f))
		}
		key, ok := kvexpr.Key.(*ast.BasicLit)
		if !ok {
			return nil, fmt.Errorf(`the key of the element of the composite literal kvexpr %s is not BasicLit`, pkg.Position(kvexpr))
		}
		value, ok := kvexpr.Value.(*ast.CallExpr)
		if !ok {
			return nil, fmt.Errorf(`the value of the element of the composite literal kvexpr %s is not CallExpr`, pkg.Position(kvexpr))
		}

		resourceType, _ := strconv.Unquote(key.Value)

		resourceFuncIdent, ok := value.Fun.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf(`the Fun of the CallExpr %s is not Ident`, pkg.Position(value))
		}

		resourceFuncTypeObj, ok := pkg.GoPackage.TypesInfo.Uses[resourceFuncIdent]
		if !ok {
			return nil, fmt.Errorf(`failed to find the resource function object by %s`, pkg.Position(resourceFuncIdent))
		}

		resourceFuncTypeObjFunc, ok := resourceFuncTypeObj.(*types.Func)
		if !ok {
			return nil, fmt.Errorf(`the resource function object %s is not a Func`, pkg.Position(resourceFuncTypeObj))
		}

		// Since we only build the SSA pacakge for the current package, we don't support that the resource function is defined in
		// external packages.
		if resourceFuncTypeObjFunc.Pkg().Path() != pkg.GoPackage.PkgPath {
			return nil, fmt.Errorf(`the resource function is defined in external package %s`, resourceFuncTypeObjFunc.Pkg().Path())
		}

		apiPath, err := handleUntypedResource(pkg, resourceFuncTypeObjFunc)
		if err != nil {
			return nil, fmt.Errorf(`handling the untyped resource function %s: %w`, pkg.Position(resourceFuncIdent), err)
		}

		resourceMapping[resourceType] = apiPath
	}

	return resourceMapping, nil
}

func handleUntypedResource(pkg *Package, resFunc *types.Func) (string, error) {
	f := functionDeclOfFunc(pkg.GoPackage, resFunc)

	if len(f.Body.List) != 1 {
		return "", fmt.Errorf("the function body of %s doesn't only contain one return", pkg.Position(f))
	}

	result, ok := f.Body.List[0].(*ast.ReturnStmt).Results[0].(*ast.UnaryExpr)
	if !ok {
		return "", fmt.Errorf("the return value of function %s is not a UnaryExpr", pkg.Position(f))
	}

	resultX, ok := result.X.(*ast.CompositeLit)
	if !ok {
		return "", fmt.Errorf("the return value %s doesn't point to a CompositeLit", pkg.Position(result))
	}

	for _, elet := range resultX.Elts {
		kvexpr, ok := elet.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kvexpr.Key.(*ast.Ident)
		if !ok {
			return "", fmt.Errorf(`the key of the element %s is not Ident`, pkg.Position(kvexpr))
		}
		if key.Name != "Delete" {
			continue
		}

		value, ok := kvexpr.Value.(*ast.Ident)
		if !ok {
			return "", fmt.Errorf(`the value of the element %s is not Ident`, pkg.Position(kvexpr))
		}

		deleteFuncMember, ok := pkg.SSAPackage.Members[value.Name]
		if !ok {
			return "", fmt.Errorf(`failed to find the %s in the SSA pacakge`, pkg.Position(value))
		}

		deleteFunc, ok := deleteFuncMember.(*ssa.Function)
		if !ok {
			return "", fmt.Errorf(`the %s is not a function`, pkg.Position(value))
		}

		apiPath, err := findApiPathFromDeleteFunc(pkg, deleteFunc)
		if err != nil {
			return "", err
		}
		return apiPath, nil
	}
	return "", fmt.Errorf("failed to find the delete callback in %s", pkg.Position(resultX))
}
