package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"log"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

type UntypedRegistration struct {
	pkg *Package
	obj types.Object
}

func NewUntypedRegistration(pkg *Package, obj types.Object) UntypedRegistration {
	return UntypedRegistration{
		pkg: pkg,
		obj: obj,
	}
}

func (reg UntypedRegistration) run() (map[string]string, error) {
	// TF resource type -> Azure api path
	resourceMapping := map[string]string{}

	// Iterate each sdk.Resource as defined in the SupportedResources() method.
	nt, ok := reg.obj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%s is not a named type", reg.obj.Type())
	}
	f, err := functionDeclOfMethod(reg.pkg.GoPackage, nt, "SupportedResources")
	if err != nil {
		return nil, err
	}

	var resourceList *ast.CompositeLit
	switch len(f.Body.List) {
	case 1:
		var ok bool
		resourceList, ok = f.Body.List[0].(*ast.ReturnStmt).Results[0].(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf(`return value of function %s is not a composite literal`, reg.pkg.Position(f))
		}
	default:
		asmt, ok := f.Body.List[0].(*ast.AssignStmt)
		if !ok {
			return nil, fmt.Errorf(`the 1st statement of function %s is not an assignment`, reg.pkg.Position(f))
		}
		resourceList, ok = asmt.Rhs[0].(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf(`the 1st assignment statement's RHS of function %s is not a composite literal`, reg.pkg.Position(f))
		}
	}

	// Iterating each resource in the resource list to analyze and retrieve the TF resource and api path.
	// Note that if any step ends for processing the current resource up with an error,
	// it will be skipped and continue processing the next resource.
	for _, elt := range resourceList.Elts {
		resourceType, apiPath, err := reg.processResource(elt)
		if err != nil {
			log.Println(err)
			continue
		}
		resourceMapping[resourceType] = apiPath
	}

	return resourceMapping, nil
}

func (reg UntypedRegistration) processResource(resExpr ast.Expr) (string, string, error) {
	kvexpr, ok := resExpr.(*ast.KeyValueExpr)
	if !ok {
		return "", "", fmt.Errorf(`the resource expression %s is not KeyValueExpr`, reg.pkg.Position(resExpr))
	}

	// Identify the TF resource type.
	// The TF resource type is defined as the key of the KeyValueExpr
	key, ok := kvexpr.Key.(*ast.BasicLit)
	if !ok {
		return "", "", fmt.Errorf(`the key of the resource expression %s is not BasicLit`, reg.pkg.Position(kvexpr))
	}
	resourceType, _ := strconv.Unquote(key.Value)

	// Identify the Azure API path.
	// The API path comes from its Delete() method
	value, ok := kvexpr.Value.(*ast.CallExpr)
	if !ok {
		return "", "", fmt.Errorf(`the value of the resource expression %s is not CallExpr`, reg.pkg.Position(kvexpr))
	}

	apiPath, err := reg.identifyApiPath(value)
	if err != nil {
		return "", "", fmt.Errorf(`identifying the API path from %s: %w`, reg.pkg.Position(value), err)
	}

	return resourceType, apiPath, nil
}

func (reg UntypedRegistration) identifyApiPath(call *ast.CallExpr) (string, error) {
	resFuncIdent, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", fmt.Errorf(`the Fun of the CallExpr %s is not an Ident`, reg.pkg.Position(call))
	}

	resFuncObj, ok := reg.pkg.GoPackage.TypesInfo.Uses[resFuncIdent]
	if !ok {
		return "", fmt.Errorf(`failed to find the resource function object by %s`, reg.pkg.Position(resFuncIdent))
	}

	resFunc, ok := resFuncObj.(*types.Func)
	if !ok {
		return "", fmt.Errorf(`the resource function object %s is not a Func`, reg.pkg.Position(resFuncObj))
	}

	// Since we only build the SSA package for the current package, we don't support that the resource function is defined in
	// external packages.
	if resFunc.Pkg().Path() != reg.pkg.GoPackage.PkgPath {
		return "", fmt.Errorf(`the resource function is defined in external package %s`, resFunc.Pkg().Path())
	}

	f := functionDeclOfFunc(reg.pkg.GoPackage, resFunc)

	if len(f.Body.List) != 1 {
		return "", fmt.Errorf("the function body of %s doesn't only contain one return", reg.pkg.Position(f))
	}

	result, ok := f.Body.List[0].(*ast.ReturnStmt).Results[0].(*ast.UnaryExpr)
	if !ok {
		return "", fmt.Errorf("the return value of function %s is not a UnaryExpr", reg.pkg.Position(f))
	}

	resultX, ok := result.X.(*ast.CompositeLit)
	if !ok {
		return "", fmt.Errorf("the return value %s doesn't point to a CompositeLit", reg.pkg.Position(result))
	}

	for _, elet := range resultX.Elts {
		kvexpr, ok := elet.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kvexpr.Key.(*ast.Ident)
		if !ok {
			return "", fmt.Errorf(`the key of the element %s is not Ident`, reg.pkg.Position(kvexpr))
		}
		if key.Name != "Delete" {
			continue
		}

		value, ok := kvexpr.Value.(*ast.Ident)
		if !ok {
			return "", fmt.Errorf(`the value of the element %s is not Ident`, reg.pkg.Position(kvexpr))
		}

		deleteFuncMember, ok := reg.pkg.SSAPackage.Members[value.Name]
		if !ok {
			return "", fmt.Errorf(`failed to find the %s in the SSA pacakge`, reg.pkg.Position(value))
		}

		deleteFunc, ok := deleteFuncMember.(*ssa.Function)
		if !ok {
			return "", fmt.Errorf(`the %s is not a function`, reg.pkg.Position(value))
		}

		apiPath, err := findApiPathFromDeleteFunc(reg.pkg, deleteFunc)
		if err != nil {
			return "", err
		}
		return apiPath, nil
	}
	return "", fmt.Errorf("failed to find the delete callback in %s", reg.pkg.Position(resultX))
}
