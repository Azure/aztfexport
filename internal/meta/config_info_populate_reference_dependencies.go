package meta

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Scan the HCL files for references to other resources.
// For example the HCL attribute `key_vault_id = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.KeyVault/vaults/vault1"`
// will yield a dependency to the TF resource with address `azurerm_key_vault.vault1`.
// Note that the a single TF resource id can map to multiple resources -- in which case the dependencies will be categorised
// as ambiguous.
func (cfgs ConfigInfos) PopulateReferenceDependencies() error {
	// key: TFResourceId
	m := map[string][]*ConfigInfo{}
	for _, cfg := range cfgs {
		m[cfg.TFResourceId] = append(m[cfg.TFResourceId], &cfg)
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
			dependingConfigs, ok := m[maybeTFId]
			if !ok {
				return nil
			}

			depTFResId := maybeTFId

			var dependingConfigsWithoutSelf []*ConfigInfo
			for _, depCfg := range dependingConfigs[:] {
				if depCfg.AzureResourceID.String() == cfg.AzureResourceID.String() {
					continue
				}
				// if cfg is parent of `id` resource, we should skip, or it will cause circular dependency, so skip parent depends on sub resources
				if cfg.AzureResourceID.Equal(depCfg.AzureResourceID.Parent()) {
					continue
				}
				dependingConfigsWithoutSelf = append(dependingConfigsWithoutSelf, depCfg)
			}

			if len(dependingConfigsWithoutSelf) == 1 {
				cfg.dependencies.referenceDeps[depTFResId] = Dependency{
					TFResourceId:    depTFResId,
					AzureResourceId: dependingConfigsWithoutSelf[0].AzureResourceID.String(),
					TFAddr:          dependingConfigsWithoutSelf[0].TFAddr,
				}
			} else if len(dependingConfigsWithoutSelf) > 1 {
				deps := make([]Dependency, 0, len(dependingConfigsWithoutSelf))
				for _, depCfg := range dependingConfigsWithoutSelf {
					deps = append(deps, Dependency{
						TFResourceId:    depTFResId,
						AzureResourceId: depCfg.AzureResourceID.String(),
						TFAddr:          depCfg.TFAddr,
					})
				}
				cfg.dependencies.ambiguousDeps[depTFResId] = deps
			}

			return nil
		})
		cfgs[i] = cfg
	}
	return nil
}
