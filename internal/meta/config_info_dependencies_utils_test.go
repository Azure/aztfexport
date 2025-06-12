package meta

import (
	"fmt"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/magodo/armid"
)

func tfAddr(s string) tfaddr.TFAddr {
	tfAddr, err := tfaddr.ParseTFResourceAddr(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse TF resource address %s: %v", s, err))
	}
	return *tfAddr
}

func configInfoWithDeps(
	azureResourceIdStr string,
	tFResourceId string,
	tfAddr tfaddr.TFAddr,
	hclStr string,
	refDeps map[string]Dependency,
	ambiguousDeps map[string][]Dependency,
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
		dependencies: Dependencies{
			referenceDeps:   refDeps,
			parentChildDeps: make(map[Dependency]bool),
			ambiguousDeps:   ambiguousDeps,
		},
		hcl: hcl,
	}
}

func configInfo(
	azureResourceIdStr string,
	tFResourceId string,
	tfAddr tfaddr.TFAddr,
	hclStr string,
) ConfigInfo {
	return configInfoWithDeps(
		azureResourceIdStr,
		tFResourceId,
		tfAddr,
		hclStr,
		make(map[string]Dependency),
		make(map[string][]Dependency),
	)
}
