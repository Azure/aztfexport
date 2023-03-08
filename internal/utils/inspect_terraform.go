package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type TerraformBlockDetail struct {
	BackendType string
}

// InspecTerraformBlock inspects the terraform block by interating the top level .tf files.
// This function assumes the dir is a valid terraform workspace, which means there is at most one terraform block defined.
func InspecTerraformBlock(dir string) (*TerraformBlockDetail, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %v", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".tf" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %v", entry.Name(), err)
		}
		f, diags := hclsyntax.ParseConfig(b, entry.Name(), hcl.InitialPos)
		if diags.HasErrors() {
			return nil, fmt.Errorf("parsing file %s: %v", entry.Name(), diags.Error())
		}
		for _, block := range f.Body.(*hclsyntax.Body).Blocks {
			if block.Type != "terraform" {
				continue
			}
			var detail TerraformBlockDetail
			for _, block := range block.Body.Blocks {
				switch block.Type {
				case "backend":
					detail.BackendType = block.Labels[0]
				}
			}
			return &detail, nil
		}
	}
	return nil, nil
}
