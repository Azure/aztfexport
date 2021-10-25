package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/ssa"
	"log"
	"strconv"
)

func main() {
	pkgs, err := loadPackage("/home/magodo/github/terraform-provider-azurerm", []string{"./internal/sdk", "./internal/services/web"})
	if err != nil {
		log.Fatal(err)
	}

	sdkPkg := pkgs[0].GoPackage
	var typedRegistration, untypedRegistration *types.Interface
	for _, obj := range sdkPkg.TypesInfo.Defs {
		if obj == nil {
			continue
		}
		t := obj.Type()
		nt, ok := t.(*types.Named)
		if !ok {
			continue
		}
		ut, ok := nt.Underlying().(*types.Interface)
		if !ok {
			continue
		}
		switch obj.Name() {
		case "TypedServiceRegistration":
			typedRegistration = ut
		case "UntypedServiceRegistration":
			untypedRegistration = ut
		}
	}
	if typedRegistration == nil {
		log.Fatal("can't find TypedServiceRegistration")
	}
	if untypedRegistration == nil {
		log.Fatal("can't find UntypedServiceRegistration")
	}

	webPkg := pkgs[1]

	for _, obj := range webPkg.GoPackage.TypesInfo.Defs {
		if obj == nil || obj.Name() != "Registration" {
			continue
		}
		if types.Implements(obj.Type(), typedRegistration) {
			if _, err := handleTypedRegistration(webPkg, obj); err != nil {
				log.Fatal(err)
			}
		}
		if types.Implements(obj.Type(), untypedRegistration) {
			if _, err := handleUntypedRegistration(webPkg, obj); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func handleTypedRegistration(pkg *Package, obj types.Object) (map[string]string, error) {
	// TF resource type -> Azure api path
	resourceMapping := map[string]string{}

	// Iterate each sdk.Resource as defined in the Resources() method.
	nt, ok := obj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%s is not a named type", obj.Type())
	}
	f, err := functionDeclOfMethod(pkg.GoPackage, nt, "Resources")
	if err != nil {
		return nil, err
	}
	retStmt, ok := f.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return nil, fmt.Errorf("the first statement of the function %s is not a return statement", pkg.GoPackage.Fset.Position(f.Pos()))
	}

	result, ok := retStmt.Results[0].(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("the returned expression of the function %s is not a composite literal", pkg.GoPackage.Fset.Position(f.Pos()))
	}

	for _, resExpr := range result.Elts {
		resComplit, ok := resExpr.(*ast.CompositeLit)
		if !ok {
			return nil, fmt.Errorf("the returned resource %s is not a composite literal", pkg.GoPackage.Fset.Position(resExpr.Pos()))
		}

		resTypeObj, ok := pkg.GoPackage.TypesInfo.Uses[resComplit.Type.(*ast.Ident)]
		if !ok {
			return nil, fmt.Errorf("failed to find the type info for %s", pkg.GoPackage.Fset.Position(resExpr.Pos()))
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
		return "", "", fmt.Errorf("the first statement of the function %s is not a return statement", pkg.GoPackage.Fset.Position(f.Pos()))
	}
	result, ok := retStmt.Results[0].(*ast.BasicLit)
	if !ok {
		return "", "", fmt.Errorf("the returned expression of the function %s is not a basic literal", pkg.GoPackage.Fset.Position(f.Pos()))
	}
	tfResourceType, err := strconv.Unquote(result.Value)
	if err != nil {
		return "", "", err
	}

	// Identify the Azure API path.
	// The API path comes from its Delete() method
	deleteMethod := pkg.SSAPackage.Prog.LookupMethod(obj.Type(), obj.Pkg(), "Delete")
	if deleteMethod == nil {
		return "", "", fmt.Errorf(`can't find the "Delete" method for object %s in the SSA program`, pkg.GoPackage.Fset.Position(obj.Pos()))
	}

	// In current implementation, there are two patterns used to define the "Delete()" method:
	// 1. Directly return a composite literal of the sdk.ResourceFunc: e.g. https://github.com/hashicorp/terraform-provider-azurerm/blob/cf19ce361eabe87192e3d5265d67d55ceb10e327/internal/services/web/app_service_environment_v3_resource.go#L393
	// 2. Return a function invocation, which returns a sdk.ResourceFunc: e.g. https://github.com/hashicorp/terraform-provider-azurerm/blob/9c8a6259bee42db8c4611549b30155bc03586ba2/internal/services/policy/assignment_resource.go#L43
	// Therefore, we need to do data flow analysis backwards from the return instruction.
	deleteFunc, err := getRealDeleteFunc(pkg, deleteMethod)
	if err != nil {
		return "", "", err
	}

	deleteFuncNode, ok := pkg.CallGraph.Nodes[deleteFunc]
	if !ok {
		return "", "", fmt.Errorf(`can't find the real "Delete" function for object %s in the callgraph`, pkg.GoPackage.Fset.Position(obj.Pos()))
	}
	callees := AllCalleesOf(deleteFuncNode)
	_ = callees

	return tfResourceType, "", errors.New("TODO")
}

func getRealDeleteFunc(pkg *Package, deleteMethod *ssa.Function) (*ssa.Function, error) {
	bbs := deleteMethod.DomPreorder()
	lastBB := bbs[len(bbs)-1]
	returnInstr := lastBB.Instrs[len(lastBB.Instrs)-1].(*ssa.Return)

	switch ret := returnInstr.Results[0].(type) {
	case *ssa.Call:
		callcom := ret.Common()
		if callcom.Method != nil {
			return nil, fmt.Errorf("expected CallCommon to be call-mode, but got invoke-mode for the return value in %s", pkg.GoPackage.Fset.Position(deleteMethod.Pos()))
		}
		f, ok := callcom.Value.(*ssa.Function)
		if !ok {
			return nil, fmt.Errorf("expected value of CallCommon to be Function for the return value in %s", pkg.GoPackage.Fset.Position(deleteMethod.Pos()))
		}
		return getRealDeleteFunc(pkg, f)
	case *ssa.UnOp:
		referrers := ret.X.Referrers()
		if referrers == nil {
			return nil, fmt.Errorf(`unexpected nil referrers of the returned value of the "Delete" method %s in the SSA program`, pkg.GoPackage.Fset.Position(deleteMethod.Pos()))
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
			return nil, fmt.Errorf(`can't find FieldAddr for the "Func" field in the "Delete" method %s in the SSA program`, pkg.GoPackage.Fset.Position(deleteMethod.Pos()))
		}

		referrers = funcFieldAddr.Referrers()
		if referrers == nil {
			return nil, fmt.Errorf(`unexpected nil referrers of the "Func" FieldAddr value of the "Delete" method %s in the SSA program`, pkg.GoPackage.Fset.Position(deleteMethod.Pos()))
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
			return nil, fmt.Errorf(`can't find Store instruction for the "Func" field in the "Delete" method %s in the SSA program`, pkg.GoPackage.Fset.Position(deleteMethod.Pos()))
		}
		return targetFunc, nil
	default:
		return nil, fmt.Errorf("unexpected type of the return value in %s", pkg.GoPackage.Fset.Position(deleteMethod.Pos()))
	}
}

func handleUntypedRegistration(pkg *Package, obj types.Object) (map[string]string, error) {
	// TF resource type -> Azure api path
	resourceMapping := map[string]string{}
	return resourceMapping, nil
}
