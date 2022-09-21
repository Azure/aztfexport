package meta

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem
	DependsOn []string // Azure resource ids
	hcl       *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	out := hclwrite.Format(cfg.hcl.Bytes())
	return w.Write(out)
}

func (cfgs ConfigInfos) AddDependency() error {
	cfgs.addParentChildDependency()
	if err := cfgs.addReferenceDependency(); err != nil {
		return err
	}

	// Disduplicate then sort the dependencies
	for i, cfg := range cfgs {
		if len(cfg.DependsOn) == 0 {
			continue
		}

		// Disduplicate same resource ids
		set := map[string]bool{}
		for _, dep := range cfg.DependsOn {
			set[dep] = true
		}

		// Disduplicate dependency that is parent of another dependency
		var covlist []string
		for dep := range set {
			for odep := range set {
				if dep == odep {
					continue
				}
				if strings.HasPrefix(strings.ToUpper(odep), strings.ToUpper(dep)) {
					covlist = append(covlist, dep)
					break
				}
			}
		}
		for _, dep := range covlist {
			delete(set, dep)
		}

		// Sort the dependencies
		cfg.DependsOn = []string{}
		for dep := range set {
			cfg.DependsOn = append(cfg.DependsOn, dep)
		}

		sort.Slice(cfg.DependsOn, func(i, j int) bool {
			return cfg.DependsOn[i] < cfg.DependsOn[j]
		})
		cfgs[i] = cfg
	}

	return nil
}

func (cfgs ConfigInfos) addParentChildDependency() {
	for i, cfg := range cfgs {
		parentId := cfg.AzureResourceID.Parent()
		// This resource is either a root scope or a root scoped resource
		if parentId == nil {
			// Root scope: ignore as it has no parent
			if cfg.AzureResourceID.ParentScope() == nil {
				continue
			}
			// Root scoped resource: use its parent scope as its parent
			parentId = cfg.AzureResourceID.ParentScope()
		}

		// Adding the direct parent resource as its dependency
		for _, ocfg := range cfgs {
			if cfg.AzureResourceID.Equal(ocfg.AzureResourceID) {
				continue
			}
			if parentId.Equal(ocfg.AzureResourceID) {
				cfg.DependsOn = []string{ocfg.AzureResourceID.String()}
				cfgs[i] = cfg
				break
			}
		}
	}
}

func (cfgs ConfigInfos) addReferenceDependency() error {
	// TODO: we shall consider a TF resource and its child resource have the same TF id?
	m := map[string]string{} // TF resource id to Azure resource id
	for _, cfg := range cfgs {
		m[cfg.TFResourceId] = cfg.AzureResourceID.String()
	}

	for i, cfg := range cfgs {
		file, err := hclsyntax.ParseConfig(cfg.hcl.Bytes(), "main.tf", hcl.InitialPos)
		if err != nil {
			return fmt.Errorf("parsing hcl for %s: %v", cfg.AzureResourceID, err)
		}
		hclsyntax.VisitAll(file.Body.(*hclsyntax.Body), func(node hclsyntax.Node) hcl.Diagnostics {
			expr, ok := node.(*hclsyntax.LiteralValueExpr)
			if !ok {
				return nil
			}
			val := expr.Val
			if !expr.Val.IsKnown() || !val.Type().Equals(cty.String) {
				return nil
			}
			maybeTFId := val.AsString()

			// This is safe to match case sensitively given the TF id are consistent across the provider. Otherwise, it is a provider bug.
			dependingResourceId, ok := m[maybeTFId]
			if !ok {
				return nil
			}
			cfg.DependsOn = append(cfg.DependsOn, dependingResourceId)
			return nil
		})
		cfgs[i] = cfg
	}
	return nil
}
