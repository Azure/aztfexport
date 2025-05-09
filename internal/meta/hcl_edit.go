package meta

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// hclBlockUpdateDependency substitute ids found in values with TF address to introduce implicit dependency, and
// falls back to the depends_on instructions if none found.
// cfgset is a map keyed by azure resource id.
func hclBlockUpdateDependency(body *hclwrite.Body, deps []Dependency, cfgset map[string]ConfigInfo) error {
	dependsOnMetaArgs := []string{}
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
		cfg := cfgset[dep.Candidates[0]]
		if !replaceIdValuedTokensWithTFAddr(body, cfg) {
			dependsOnMetaArgs = append(dependsOnMetaArgs, cfg.TFAddr.String()+",")
		}
	}
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

// Traverse the attribute tokens and replace all TFResourceId-valued token surrounded with quotes with TFAddr.
// This function recurse through nested blocks. Returns true if any replacement was made
func replaceIdValuedTokensWithTFAddr(body *hclwrite.Body, cfg ConfigInfo) bool {
	resourceId := cfg.TFResourceId
	ret := false

	for name, attr := range body.Attributes() {
		tokens := attr.Expr().BuildTokens(nil)
		filteredTokens := hclwrite.Tokens{}
		resourceIdValuedTokenFound := false

		for i := 0; i < len(tokens); {
			if i+2 < len(tokens) &&
				tokens[i].Type == hclsyntax.TokenOQuote &&
				tokens[i+1].Type == hclsyntax.TokenQuotedLit &&
				tokens[i+2].Type == hclsyntax.TokenCQuote &&
				string(tokens[i+1].Bytes) == resourceId {
				filteredTokens = append(filteredTokens, &hclwrite.Token{
					Type:         hclsyntax.TokenIdent,
					Bytes:        fmt.Appendf(nil, "%s.id", cfg.TFAddr.String()),
					SpacesBefore: tokens[i].SpacesBefore,
				})
				resourceIdValuedTokenFound = true
				i += 3
			} else {
				filteredTokens = append(filteredTokens, tokens[i])
				i++
			}
		}

		if resourceIdValuedTokenFound {
			body.SetAttributeRaw(name, filteredTokens)
			ret = true
		}
	}

	for _, block := range body.Blocks() {
		if replaceIdValuedTokensWithTFAddr(block.Body(), cfg) {
			ret = true
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
