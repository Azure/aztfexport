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
		if !replaceIdAttrValuesWithTFAddr(body, cfg) {
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

func replaceIdAttrValuesWithTFAddr(body *hclwrite.Body, cfg ConfigInfo) bool {
	resourceId := cfg.AzureResourceID.String()
	ret := false

	for attrName, attrVal := range body.Attributes() {
		attrVal.Expr().BuildTokens(nil)
		if attrValue(attrVal) == resourceId {
			tfAddr := fmt.Sprintf("%s.id", cfg.TFAddr.String())
			body.SetAttributeRaw(attrName, hclwrite.Tokens{
				{
					Type:         hclsyntax.TokenIdent,
					Bytes:        []byte(tfAddr),
					SpacesBefore: 1,
				},
			})
			ret = true
		}
	}
	return ret
}

func attrValue(attrVal *hclwrite.Attribute) string {
	ret := string(attrVal.Expr().BuildTokens(nil).Bytes())
	ret = strings.Trim(ret, `'"\ `)
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
