package meta

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

func (configs ConfigInfos) applyDependenciesToHclBlock() error {
	for i, cfg := range configs {
		applyReferenceDependenciesToHcl(cfg.hcl.Body().Blocks()[0].Body(), cfg.dependencies.referenceDeps)
		if err := applyParentChildAndAmbiguousDepsToHclBlock(
			cfg.hcl.Body().Blocks()[0].Body(),
			cfg.dependencies.parentChildDeps,
			cfg.dependencies.ambiguousDeps); err != nil {
			return fmt.Errorf("applying explicit and ambiguous dependencies to %s: %w", cfg.TFResourceId, err)
		}
		configs[i] = cfg
	}
	return nil
}

func applyReferenceDependenciesToHcl(body *hclwrite.Body, refDeps map[string]Dependency) {
	if len(refDeps) == 0 {
		return
	}

	for name, attr := range body.Attributes() {
		tokens := attr.Expr().BuildTokens(nil)
		filteredTokens := hclwrite.Tokens{}
		tokensModified := false

		for i := 0; i < len(tokens); i++ {
			refDep, refDepExists := refDeps[string(tokens[i].Bytes)]
			// Parsing process guaranteed QuotedLit is surrounded by Opening and Closing quote
			if tokens[i].Type == hclsyntax.TokenQuotedLit && refDepExists {
				filteredTokens[len(filteredTokens)-1] = &hclwrite.Token{
					Type:         hclsyntax.TokenIdent,
					Bytes:        fmt.Appendf(nil, "%s.id", refDep.TFAddr),
					SpacesBefore: tokens[i-1].SpacesBefore,
				}
				tokensModified = true
				i += 1 // Skip the next token as it was already processed
			} else {
				filteredTokens = append(filteredTokens, tokens[i])
			}
		}

		if tokensModified {
			body.SetAttributeRaw(name, filteredTokens)
		}

		for _, nestedBlock := range body.Blocks() {
			applyReferenceDependenciesToHcl(nestedBlock.Body(), refDeps)
		}
	}
}

func applyParentChildAndAmbiguousDepsToHclBlock(
	body *hclwrite.Body,
	parentChildDeps map[Dependency]bool,
	ambiguousDeps map[string][]Dependency) error {

	if len(parentChildDeps) > 0 || len(ambiguousDeps) > 0 {
		src := "depends_on = [\n"
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
		if len(parentChildDeps) > 0 {
			tfAddrs := make([]string, 0, len(parentChildDeps))
			for dep := range parentChildDeps {
				tfAddrs = append(tfAddrs, dep.TFAddr.String())
			}
			sort.Strings(tfAddrs)
			src += strings.Join(tfAddrs, ",\n") + "\n"
		}
		src += "]\n"
		expr, diags := hclwrite.ParseConfig([]byte(src), "f", hcl.InitialPos)
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
