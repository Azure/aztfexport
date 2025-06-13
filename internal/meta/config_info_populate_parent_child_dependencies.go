package meta

import (
	"strings"

	"github.com/magodo/armid"
)

// Look at the resource id and determine if parent dependency exist.
// For example, /subscriptions/123/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1
// has a parent /subscriptions/123/resourceGroups/rg1, which is the resource group.
// Unless present as reference dependency (maybe transitively), this parent will be added as an explicit dependency
// (via depends_on meta arg).
func (cfgs ConfigInfos) populateParentChildDependency() {
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
		} else if parentId.Parent() == nil {
			// The cfg resource is the RP 1st level resource, we regard its parent scope as its parent
			parentId = cfg.AzureResourceID.ParentScope()
		}

		// Adding the direct parent resource as its dependency
		for _, ocfg := range cfgs {
			if cfg.AzureResourceID.Equal(ocfg.AzureResourceID) {
				continue
			}
			if parentId.Equal(ocfg.AzureResourceID) &&
				// Only add parent dependency if it is not already (maybe transitively) a reference dependency.
				!hasReferenceDepWithPrefix(cfg.dependencies.refDeps, ocfg.AzureResourceID) {
				cfg.dependencies.parentChildDeps[Dependency{
					TFResourceId:    ocfg.TFResourceId,
					AzureResourceId: ocfg.AzureResourceID.String(),
					TFAddr:          ocfg.TFAddr,
				}] = true
				break
			}
		}
		cfgs[i] = cfg
	}
}

func hasReferenceDepWithPrefix(refDeps map[string]Dependency, prefix armid.ResourceId) bool {
	for _, dep := range refDeps {
		if strings.HasPrefix(dep.AzureResourceId, prefix.String()) {
			return true
		}
	}
	return false
}
