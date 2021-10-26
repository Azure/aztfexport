package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/ssa"
	"strconv"
)

func handleTypedRegistration(pkg *Package, regObj types.Object) (map[string]string, error) {
	// TF resource type -> Azure api path
	resourceMapping := map[string]string{}

	// Iterate each sdk.Resource as defined in the Resources() method.
	nt, ok := regObj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%s is not a named type", regObj.Type())
	}
	f, err := functionDeclOfMethod(pkg.GoPackage, nt, "Resources")
	if err != nil {
		return nil, err
	}
	retStmt, ok := f.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return nil, fmt.Errorf("the function %s doesn't contain only a return", pkg.Position(f))
	}

	result, ok := retStmt.Results[0].(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("return value of function %s is not a composite literal", pkg.Position(f))
	}

	for _, resExpr := range result.Elts {
		resComplit, ok := resExpr.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("the returned resource %s is not a composite literal", pkg.Position(resExpr))
		}

		resTypeObj, ok := pkg.GoPackage.TypesInfo.Uses[resComplit.Type.(*ast.Ident)]
		if !ok {
			return nil, fmt.Errorf("failed to find type info for %s", pkg.Position(resExpr))
		}

		tfName, apiPath, err := handleTypedResource(pkg, resTypeObj)
		if err != nil {
			return nil, err
		}
		resourceMapping[tfName] = apiPath
	}

	return resourceMapping, nil
}

func handleTypedResource(pkg *Package, obj types.Object) (string, string, error) {
	// Identify the TF resource type.
	// The TF resource type is defined in its ResourceType() method
	f, err := functionDeclOfMethod(pkg.GoPackage, obj.Type().(*types.Named), "ResourceType")
	if err != nil {
		return "", "", err
	}
	retStmt, ok := f.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return "", "", fmt.Errorf("the function %s doesn't contain only a return", pkg.Position(f))
	}
	result, ok := retStmt.Results[0].(*ast.BasicLit)
	if !ok {
		return "", "", fmt.Errorf("return value of the function %s is not a basic literal", pkg.Position(f))
	}
	tfResourceType, err := strconv.Unquote(result.Value)
	if err != nil {
		return "", "", err
	}

	// Identify the Azure API path.
	// The API path comes from its Delete() method
	deleteMethod := pkg.SSAPackage.Prog.LookupMethod(obj.Type(), obj.Pkg(), "Delete")
	if deleteMethod == nil {
		return "", "", fmt.Errorf(`failed to find the "Delete" method for object %s in the SSA program`, pkg.Position(obj))
	}

	deleteFunc, err := getRealFunc(pkg, deleteMethod)
	if err != nil {
		return "", "", err
	}

	apiPath, err := findApiPathFromDeleteFunc(pkg, deleteFunc)
	if err != nil {
		return "", "", err
	}
	return tfResourceType, apiPath, nil
}

func getRealFunc(pkg *Package, ofunc *ssa.Function) (*ssa.Function, error) {
	bbs := ofunc.DomPreorder()
	lastBB := bbs[len(bbs)-1]
	returnInstr := lastBB.Instrs[len(lastBB.Instrs)-1].(*ssa.Return)

	// In current implementation, there are two patterns used to define the "CRUD" method:
	// 1. Directly return a composite literal of the sdk.ResourceFunc: e.g. https://github.com/hashicorp/terraform-provider-azurerm/blob/cf19ce361eabe87192e3d5265d67d55ceb10e327/internal/services/web/app_service_environment_v3_resource.go#L393
	// 2. Return a function invocation, which returns a sdk.ResourceFunc: e.g. https://github.com/hashicorp/terraform-provider-azurerm/blob/9c8a6259bee42db8c4611549b30155bc03586ba2/internal/services/policy/assignment_resource.go#L43
	// Therefore, we need to do data flow analysis backwards from the return instruction.
	switch ret := returnInstr.Results[0].(type) {
	case *ssa.Call:
		callcom := ret.Common()
		if callcom.Method != nil {
			return nil, fmt.Errorf("expected CallCommon to be call-mode, but got invoke-mode for the return value in %s", pkg.GoPackage.Fset.Position(ofunc.Pos()))
		}
		f, ok := callcom.Value.(*ssa.Function)
		if !ok {
			return nil, fmt.Errorf("expected value of CallCommon to be Function for the return value in %s", pkg.GoPackage.Fset.Position(ofunc.Pos()))
		}
		return getRealFunc(pkg, f)
	case *ssa.UnOp:
		referrers := ret.X.Referrers()
		if referrers == nil {
			return nil, fmt.Errorf(`unexpected nil referrers of the returned value of the method %s in the SSA program`, pkg.GoPackage.Fset.Position(ofunc.Pos()))
		}
		var funcFieldAddr *ssa.FieldAddr
		for _, referer := range *referrers {
			fieldAddr, ok := referer.(*ssa.FieldAddr)
			if !ok {
				continue
			}
			if fieldAddr.Field != 0 {
				continue
			}
			funcFieldAddr = fieldAddr
		}
		if funcFieldAddr == nil {
			return nil, fmt.Errorf(`can't find FieldAddr for the "Func" field in the method %s in the SSA program`, pkg.GoPackage.Fset.Position(ofunc.Pos()))
		}

		referrers = funcFieldAddr.Referrers()
		if referrers == nil {
			return nil, fmt.Errorf(`unexpected nil referrers of the "Func" FieldAddr value of the method %s in the SSA program`, pkg.GoPackage.Fset.Position(ofunc.Pos()))
		}
		var targetFunc *ssa.Function
		for _, referrer := range *referrers {
			store, ok := referrer.(*ssa.Store)
			if !ok {
				continue
			}
			changeType, ok := store.Val.(*ssa.ChangeType)
			if !ok {
				continue
			}
			f, ok := changeType.X.(*ssa.Function)
			if !ok {
				continue
			}
			targetFunc = f
		}
		if targetFunc == nil {
			return nil, fmt.Errorf(`can't find Store instruction for the "Func" field in the method %s in the SSA program`, pkg.GoPackage.Fset.Position(ofunc.Pos()))
		}
		return targetFunc, nil
	default:
		return nil, fmt.Errorf("unexpected type of the return value in %s", pkg.GoPackage.Fset.Position(ofunc.Pos()))
	}
}
