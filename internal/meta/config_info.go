package meta

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type ConfigInfos []ConfigInfo

type ConfigInfo struct {
	ImportItem

	Dependencies Dependencies

	HCL *hclwrite.File
}

func (cfg ConfigInfo) DumpHCL(w io.Writer) (int, error) {
	out := hclwrite.Format(cfg.HCL.Bytes())
	return w.Write(out)
}

func (cfg *ConfigInfo) applyRefDepsToHCL() {
	var applyF func(*hclwrite.Body, map[string]Dependency)
	applyF = func(body *hclwrite.Body, deps map[string]Dependency) {
		if len(deps) == 0 {
			return
		}
		for name, attr := range body.Attributes() {
			tokens := attr.Expr().BuildTokens(nil)
			newTokens := hclwrite.Tokens{}
			tokensModified := false

			for i := 0; i < len(tokens); i++ {
				refDep, refDepExists := deps[string(tokens[i].Bytes)]
				// Parsing process guaranteed QuotedLit is surrounded by Opening and Closing quote
				if tokens[i].Type == hclsyntax.TokenQuotedLit && refDepExists {
					newTokens[len(newTokens)-1] = &hclwrite.Token{
						Type:         hclsyntax.TokenIdent,
						Bytes:        fmt.Appendf(nil, "%s.id", refDep.TFAddr),
						SpacesBefore: tokens[i-1].SpacesBefore,
					}
					tokensModified = true
					i += 1 // Skip the next token as it was already processed
				} else {
					newTokens = append(newTokens, tokens[i])
				}
			}

			if tokensModified {
				body.SetAttributeRaw(name, newTokens)
			}
			for _, nestedBlock := range body.Blocks() {
				applyF(nestedBlock.Body(), deps)
			}
		}
	}
	applyF(cfg.HCL.Body(), cfg.Dependencies.ByRef)
}

func (cfg *ConfigInfo) applyExplicitDepsToHCL() error {
	body := cfg.HCL.Body()

	relationDep := cfg.Dependencies.ByRelation
	if relationDep != nil {
		// Skip this relation dependency if it's already covered by any of the other applied dependencies.
		var otherDepAzureIds []string
		for _, dep := range cfg.Dependencies.ByRef {
			otherDepAzureIds = append(otherDepAzureIds, dep.AzureResourceId)
		}
		var covered bool
		for _, dep := range otherDepAzureIds {
			if strings.HasPrefix(dep, relationDep.AzureResourceId) {
				covered = true
				break
			}
		}
		if covered {
			relationDep = nil
		}
	}

	ambiguousDeps := cfg.Dependencies.ByRefAmbiguous
	// TODO: Skip the ambiguous depedencies that are covered by any of the other applied Dependencies.

	if len(ambiguousDeps) == 0 && relationDep == nil {
		return nil
	}

	src := "depends_on = [\n"
	if relationDep != nil {
		src += relationDep.TFAddr.String() + ",\n"
	}
	if len(ambiguousDeps) > 0 {
		ambiguousDepsComments := make([]string, 0, len(ambiguousDeps))
		for _, deps := range ambiguousDeps {
			tfAddrs := make([]string, 0, len(deps))
			for _, dep := range deps {
				tfAddrs = append(tfAddrs, dep.TFAddr.String())
			}
			sort.Strings(tfAddrs)
			ambiguousDepsComments = append(ambiguousDepsComments, fmt.Sprintf("# One of %s (can't auto-resolve as their ids are identical)", strings.Join(tfAddrs, ",")))
		}
		sort.Strings(ambiguousDepsComments)
		src += strings.Join(ambiguousDepsComments, "\n") + "\n"
	}
	src += "]\n"
	expr, diags := hclwrite.ParseConfig([]byte(src), "f", hcl.InitialPos)
	if diags.HasErrors() {
		return fmt.Errorf(`building "depends_on" attribute: %s`, diags.Error())
	}
	body.SetAttributeRaw("depends_on", expr.Body().GetAttribute("depends_on").Expr().BuildTokens(nil))

	return nil
}

type Dependencies struct {
	// Dependencies inferred by scanning for resource id values
	// The key is TFResourceId.
	ByRef map[string]Dependency

	// Similar to ByRef, but due to multiple Azure resources can map to a same TF resource id (being referenced),
	// this is regarded as ambiguous references.
	// The key is TFResourceId.
	ByRefAmbiguous map[string][]Dependency

	// Dependencies inferred via Azure resource id parent lookup.
	// At most one such dependency can exist.
	ByRelation *Dependency
}

type Dependency struct {
	TFResourceId    string
	AzureResourceId string
	TFAddr          tfaddr.TFAddr
}

// Look at the Azure resource id and determine if parent dependency exist.
// For example, /subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foos/foo1
// has a parent /subscriptions/123/resourceGroups/rg1, which is the resource group.
func (cfgs ConfigInfos) PopulateRelationDeps() {
	for i, cfg := range cfgs {
		parentId := cfg.AzureResourceID.Parent()

		// This resource is either a root scope or a RP id (which doesn't exist)
		if parentId == nil {
			continue
		}

		// This resource is the first level resource under the (root) scope.
		// E.g.
		// - (root scoped) /subscriptions/sub1/foos/foo
		// - (scoped) 	   /subscriptions/sub1/providers/Microsoft.Foo/foos/foo1
		// Regard the parent scope as its parent.
		if parentId.Parent() == nil {
			parentId = cfg.AzureResourceID.ParentScope()
		}

		// Adding the direct parent resource as its dependency
		for _, ocfg := range cfgs {
			if parentId.Equal(ocfg.AzureResourceID) {
				cfg.Dependencies.ByRelation = &Dependency{
					TFResourceId:    ocfg.TFResourceId,
					AzureResourceId: ocfg.AzureResourceID.String(),
					TFAddr:          ocfg.TFAddr,
				}
				break
			}
		}
		cfgs[i] = cfg
	}
}

// Scan the HCL files for references to other resources.
// For example the HCL attribute `foo_id = "/subscriptions/123/resourceGroups/rg1/providers/Microsoft.Foo/foos/foo1"`
// will yield a dependency to that foo TF resource address.
// Note that a single TF resource id can map to multiple resources, in which case the dependencies is regarded as ambiguous.
func (cfgs ConfigInfos) PopulateReferenceDeps() error {
	// key: TFResourceId
	m := map[string][]*ConfigInfo{}
	for _, cfg := range cfgs {
		m[cfg.TFResourceId] = append(m[cfg.TFResourceId], &cfg)
	}

	for i, cfg := range cfgs {
		file, err := hclsyntax.ParseConfig(cfg.HCL.Bytes(), "main.tf", hcl.InitialPos)
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

			// Try to look up this string attribute from the TF id map. If there is a match, we regard it as a valid TF resource id.
			// This is safe to match case sensitively given the TF id are consistent across the provider. Otherwise, it is a provider bug.
			dependingConfigsRaw, ok := m[maybeTFId]
			if !ok {
				return nil
			}
			depTFResId := maybeTFId

			var dependingConfigs []*ConfigInfo
			for _, depCfg := range dependingConfigsRaw[:] {
				// Ignore the self dependency
				if cfg.AzureResourceID.String() == depCfg.AzureResourceID.String() {
					continue
				}
				// Ignore the dependency on the child resource (which will cause circular dependency)
				if cfg.AzureResourceID.Equal(depCfg.AzureResourceID.Parent()) {
					continue
				}
				dependingConfigs = append(dependingConfigs, depCfg)
			}

			if len(dependingConfigs) == 1 {
				cfg.Dependencies.ByRef[depTFResId] = Dependency{
					TFResourceId:    depTFResId,
					AzureResourceId: dependingConfigs[0].AzureResourceID.String(),
					TFAddr:          dependingConfigs[0].TFAddr,
				}
			} else if len(dependingConfigs) > 1 {
				deps := make([]Dependency, 0, len(dependingConfigs))
				for _, depCfg := range dependingConfigs {
					deps = append(deps, Dependency{
						TFResourceId:    depTFResId,
						AzureResourceId: depCfg.AzureResourceID.String(),
						TFAddr:          depCfg.TFAddr,
					})
				}
				cfg.Dependencies.ByRefAmbiguous[depTFResId] = deps
			}

			return nil
		})
		cfgs[i] = cfg
	}
	return nil
}

func (configs ConfigInfos) ApplyDepsToHCL() error {
	for i, cfg := range configs {
		cfg.applyRefDepsToHCL()
		if err := cfg.applyExplicitDepsToHCL(); err != nil {
			return fmt.Errorf("applying explicit dependencies to %s: %w", cfg.TFResourceId, err)
		}
		configs[i] = cfg
	}
	return nil
}
