package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"log"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

type TypedRegistration struct {
	pkg *Package
	obj types.Object
}

func NewTypedRegistration(pkg *Package, obj types.Object) TypedRegistration {
	return TypedRegistration{
		pkg: pkg,
		obj: obj,
	}
}

func (reg TypedRegistration) run() (map[string]string, error) {
	// TF resource type -> Azure api path
	resourceMapping := map[string]string{}

	// Iterate each sdk.Resource as defined in the Resources() method.
	nt, ok := reg.obj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%s is not a named type", reg.obj.Type())
	}
	f, err := functionDeclOfMethod(reg.pkg.GoPackage, nt, "Resources")
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
	for _, resExpr := range resourceList.Elts {
		resComplit, ok := resExpr.(*ast.CompositeLit)
		if !ok {
			log.Printf("the returned resource %s is not a composite literal", reg.pkg.Position(resExpr))
			continue
		}
		resTypeObj, ok := reg.pkg.GoPackage.TypesInfo.Uses[resComplit.Type.(*ast.Ident)]
		if !ok {
			log.Printf("failed to find type info for %s", reg.pkg.Position(resExpr))
			continue
		}

		tfName, apiPath, err := reg.processResource(resTypeObj)
		if err != nil {
			log.Println(err)
			continue
		}
		resourceMapping[tfName] = apiPath
	}

	return resourceMapping, nil
}

func (reg TypedRegistration) processResource(resTypeObj types.Object) (string, string, error) {
	// Identify the TF resource type.
	// The TF resource type is defined in its ResourceType() method
	f, err := functionDeclOfMethod(reg.pkg.GoPackage, resTypeObj.Type().(*types.Named), "ResourceType")
	if err != nil {
		return "", "", err
	}
	retStmt, ok := f.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return "", "", fmt.Errorf("the function %s doesn't contain only a return", reg.pkg.Position(f))
	}
	result, ok := retStmt.Results[0].(*ast.BasicLit)
	if !ok {
		return "", "", fmt.Errorf("return value of the function %s is not a basic literal", reg.pkg.Position(f))
	}
	tfResourceType, err := strconv.Unquote(result.Value)
	if err != nil {
		return "", "", err
	}

	// Identify the Azure API path.
	// The API path comes from its Delete() method
	deleteMethod := reg.pkg.SSAPackage.Prog.LookupMethod(resTypeObj.Type(), resTypeObj.Pkg(), "Delete")
	if deleteMethod == nil {
		return "", "", fmt.Errorf(`failed to find the "Delete" method for object %s in the SSA program`, reg.pkg.Position(resTypeObj))
	}

	deleteFunc, err := getRealFunc(reg.pkg, deleteMethod)
	if err != nil {
		return "", "", fmt.Errorf(`getting the real "Delete" function: %w`, err)
	}

	apiPath, err := findApiPathFromDeleteFunc(reg.pkg, deleteFunc)
	if err != nil {
		return "", "", fmt.Errorf(`finding api path from delete function: %w`, err)
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
