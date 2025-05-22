package meta

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// Performs id --> TF address substitution for all dependency id value found in attributes.
// Any remaining unsubstituted descendant dependencies will be applied using "depends_on" meta argument.
// A dependency is descendant of another if the path is its prefix
// For example: /subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/networkInterfaces/my-nic
// is descendant of /subscriptions/123/resourceGroups/my-rg
func hclBlockUpdateDependency(body *hclwrite.Body, deps []Dependency, cfgset map[string]ConfigInfo) error {
	dependsOnMetaArgs := []string{}
	resourceIdsReplacedWithTFAddr := []string{}
	leftoverDepsYetToBeApplied := []Dependency{}

	for _, dep := range deps {
		if len(dep.Candidates) > 1 {
			var candidateIds []string
			for _, id := range dep.Candidates {
				cfg := cfgset[id]
				candidateIds = append(candidateIds, cfg.TFAddr.String())
			}
			dependsOnMetaArgs = append(dependsOnMetaArgs, fmt.Sprintf("# One of %s (can't auto-resolve as their ids are identical)", strings.Join(candidateIds, ",")))
			continue
		}
		resourceId := dep.Candidates[0]
		cfg := cfgset[resourceId]
		if replaceIdValuedTokensWithTFAddr(body, cfg) {
			resourceIdsReplacedWithTFAddr = append(resourceIdsReplacedWithTFAddr, resourceId)
		} else {
			leftoverDepsYetToBeApplied = append(leftoverDepsYetToBeApplied, dep)
		}
	}

	for _, dep := range leftoverDepsYetToBeApplied {
		resourceId := dep.Candidates[0]
		if !descendantResourceIdInCollection(resourceId, &resourceIdsReplacedWithTFAddr) {
			dependsOnMetaArgs = append(dependsOnMetaArgs, cfgset[resourceId].TFAddr.String())
		}
	}

	// Add depends_on for remaining deps that couldn't be applied via id
	if len(dependsOnMetaArgs) > 0 {
		src := []byte("depends_on = [\n" + strings.Join(dependsOnMetaArgs, "\n") + "\n]")
		expr, diags := hclwrite.ParseConfig(src, "f", hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf(`building "depends_on" attribute: %s`, diags.Error())
		}

		body.SetAttributeRaw("depends_on", expr.Body().GetAttribute("depends_on").Expr().BuildTokens(nil))
	}

	return nil
}

func descendantResourceIdInCollection(resourceIdToCheck string, resourceIds *[]string) bool {
	for _, rid := range *resourceIds {
		if strings.HasPrefix(rid, resourceIdToCheck) {
			return true
		}
	}
	return false
}

// Traverse the attribute tokens and replace all TFResourceId-valued token surrounded with quotes with TFAddr.
// This function recurse through nested blocks. Returns true if any replacement was made
func replaceIdValuedTokensWithTFAddr(body *hclwrite.Body, cfg ConfigInfo) bool {
	resourceId := cfg.TFResourceId
	ret := false

	for name, attr := range body.Attributes() {
		tokens := attr.Expr().BuildTokens(nil)
		filteredTokens := hclwrite.Tokens{}
		resourceIdValuedTokenFound := false

		for i := 0; i < len(tokens); i++ {
			// Parsing process guaranteed QuotedLit is surrounded by Opening and Closing quote
			if tokens[i].Type == hclsyntax.TokenQuotedLit && string(tokens[i].Bytes) == resourceId {
				filteredTokens[len(filteredTokens)-1] = &hclwrite.Token{
					Type:         hclsyntax.TokenIdent,
					Bytes:        fmt.Appendf(nil, "%s.id", cfg.TFAddr.String()),
					SpacesBefore: tokens[i-1].SpacesBefore,
				}
				resourceIdValuedTokenFound = true
				i += 1
			} else {
				filteredTokens = append(filteredTokens, tokens[i])
			}
		}

		if resourceIdValuedTokenFound {
			body.SetAttributeRaw(name, filteredTokens)
			ret = true
		}

		for _, block := range body.Blocks() {
			if replaceIdValuedTokensWithTFAddr(block.Body(), cfg) {
				ret = true
			}
		}
	}

	return ret
}

func hclBlockAppendLifecycle(body *hclwrite.Body, ignoreChanges []string) error {
	srcs := map[string][]byte{}
	if len(ignoreChanges) > 0 {
		for i := range ignoreChanges {
			ignoreChanges[i] = ignoreChanges[i] + ","
		}
		srcs["ignore_changes"] = []byte("ignore_changes = [\n" + strings.Join(ignoreChanges, "\n") + "\n]\n")
	}

	if len(srcs) == 0 {
		return nil
	}

	b := hclwrite.NewBlock("lifecycle", nil)
	for name, src := range srcs {
		expr, diags := hclwrite.ParseConfig(src, "f", hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf(`building "lifecycle.%s" attribute: %s`, name, diags.Error())
		}
		b.Body().SetAttributeRaw(name, expr.Body().GetAttribute(name).Expr().BuildTokens(nil))
	}
	body.AppendBlock(b)
	return nil
}
