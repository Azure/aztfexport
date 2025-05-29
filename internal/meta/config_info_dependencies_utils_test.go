package meta

import (
	"fmt"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/internal/tfresourceid"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/magodo/armid"
)

type AzureResourceId string

func tfAddrSet(tfAddrStrs ...string) *TFAddrSet {
	tfAddrSet := TFAddrSet{
		internalMap: make(map[tfaddr.TFAddr]bool),
	}
	for _, tfAddrStr := range tfAddrStrs {
		tfAddrSet.internalMap[tfAddr(tfAddrStr)] = true
	}
	return &tfAddrSet
}

func tfAddr(s string) tfaddr.TFAddr {
	tfAddr, err := tfaddr.ParseTFResourceAddr(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse TF resource address %s: %v", s, err))
	}
	return *tfAddr
}

func configInfo(
	azureResourceIdStr AzureResourceId,
	tFResourceId tfresourceid.TFResourceId,
	tfAddr tfaddr.TFAddr,
	hclStr string,
	ReferenceDependencies ReferenceDependencies,
	AmbiguousDependencies AmbiguousDependencies,
) ConfigInfo {
	azureResourceId, err := armid.ParseResourceId(string(azureResourceIdStr))
	if err != nil {
		panic(fmt.Sprintf("failed to parse Azure resource ID %s: %v", azureResourceIdStr, err))
	}
	hcl, diag := hclwrite.ParseConfig([]byte(hclStr), "main.tf", hcl.InitialPos)
	if diag.HasErrors() {
		panic(fmt.Sprintf("failed to parse HCL for Azure resource ID %s: %v", azureResourceIdStr, diag))
	}
	return ConfigInfo{
		ImportItem: ImportItem{
			AzureResourceID: azureResourceId,
			TFResourceId:    tFResourceId,
			TFAddr:          tfAddr,
		},
		referenceDependencies: ReferenceDependencies,
		ambiguousDependencies: AmbiguousDependencies,
		hcl:                   hcl,
	}
}
