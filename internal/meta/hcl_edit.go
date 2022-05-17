package meta

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

func hclBlockAppendDependency(body *hclwrite.Body, ids []string, cfgset map[string]ConfigInfo) error {
	dependencies := []string{}
	for _, id := range ids {
		cfg, ok := cfgset[id]
		if !ok {
			dependencies = append(dependencies, fmt.Sprintf("# Depending on %q, which is not imported by Terraform.", id))
			continue
		}
		dependencies = append(dependencies, cfg.TFAddr.String()+",")
	}
	if len(dependencies) > 0 {
		src := []byte("depends_on = [\n" + strings.Join(dependencies, "\n") + "\n]")
		expr, diags := hclwrite.ParseConfig(src, "f", hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf(`building "depends_on" attribute: %s`, diags.Error())
		}

		body.SetAttributeRaw("depends_on", expr.Body().GetAttribute("depends_on").Expr().BuildTokens(nil))
	}

	return nil
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
