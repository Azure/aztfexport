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
	DependsOn []Dependency
	hcl       *hclwrite.File
}

type Dependency struct {
	Candidates []string
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

		// Disduplicate same resource ids that has exact one candidate
		set := map[string]bool{}
		duplicates := []Dependency{}
		for _, dep := range cfg.DependsOn {
			if len(dep.Candidates) == 1 {
				set[dep.Candidates[0]] = true
			} else {
				duplicates = append(duplicates, dep)
			}
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

		// If all duplicate candidates are child of this dependency, then also remove it.
		covlist = []string{}
		for dep := range set {
			for _, odep := range duplicates {
				allIsChild := true
				for _, candidate := range odep.Candidates {
					if !strings.HasPrefix(strings.ToUpper(candidate), strings.ToUpper(dep)) {
						allIsChild = false
						break
					}
				}
				if allIsChild {
					covlist = append(covlist, dep)
					break
				}
			}
		}
		for _, dep := range covlist {
			delete(set, dep)
		}

		// Sort the dependencies
		cfg.DependsOn = duplicates
		for id := range set {
			id := id
			cfg.DependsOn = append(cfg.DependsOn, Dependency{Candidates: []string{id}})
		}
		sort.Slice(cfg.DependsOn, func(i, j int) bool {
			d1, d2 := cfg.DependsOn[i], cfg.DependsOn[j]
			if len(d1.Candidates) != len(d2.Candidates) {
				return len(d1.Candidates) < len(d2.Candidates)
			}
			return strings.Join(d1.Candidates, "") < strings.Join(d2.Candidates, "")
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
				id := ocfg.AzureResourceID.String()
				cfg.DependsOn = []Dependency{{Candidates: []string{id}}}
				cfgs[i] = cfg
				break
			}
		}
	}
}

func (cfgs ConfigInfos) addReferenceDependency() error {
	// TF resource id to Azure resource ids.
	// Typically, one TF resource id maps to one Azure resource id. However, there are cases that one one TF resource id maps to multiple Azure resource ids.
	// E.g. A parent and child resources have the same TF id. Or the association resource's TF id is the same as the master resource's.
	m := map[string][]string{}
	for _, cfg := range cfgs {
		m[cfg.TFResourceId] = append(m[cfg.TFResourceId], cfg.AzureResourceID.String())
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
			dependingResourceIds, ok := m[maybeTFId]
			if !ok {
				return nil
			}

			var dependingResourceIdsWithoutSelf []string
			for _, id := range dependingResourceIds[:] {
				if id == cfg.AzureResourceID.String() {
					continue
				}
				dependingResourceIdsWithoutSelf = append(dependingResourceIdsWithoutSelf, id)
			}
			cfg.DependsOn = append(cfg.DependsOn, Dependency{Candidates: dependingResourceIdsWithoutSelf})
			return nil
		})
		cfgs[i] = cfg
	}
	return nil
}
