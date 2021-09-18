package meta

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/magodo/aztfy/schema"
	"github.com/zclconf/go-cty/cty"
)

// Currently, some special attributes are output by `terraform add`, while should be excluded.
// This is tracked in: https://github.com/hashicorp/terraform/issues/29219
// We are manually excluding these special properties here by modifying the hcl.
func tuneHCLSchemaForResource(rb *hclwrite.Body, sch *schema.Schema) error {
	rb.RemoveAttribute("id")
	rb.RemoveBlock(rb.FirstMatchingBlock("timeouts", nil))

	return tuneForBlock(rb, sch.Block, nil)
}

func tuneForBlock(rb *hclwrite.Body, sch *schema.SchemaBlock, parentAttrNames []string) error {
	for attrName, attrVal := range rb.Attributes() {
		schAttr := sch.Attributes[attrName]
		if schAttr.Required {
			continue
		}

		if schAttr.Computed {
			// Especially, we will keep O+C attribute who has "ExactlyOneOf" constraint, but only keep one.
			// The one got picked is the first one in alphabetic order.
			// TODO: We should tackle more cases for different kinds of constraints.
			if schAttr.Optional && len(schAttr.ExactlyOneOf) != 0 {
				l := make([]string, len(schAttr.ExactlyOneOf))
				copy(l, schAttr.ExactlyOneOf)
				sort.Strings(l)

				addrs := append(parentAttrNames, attrName)
				if l[0] != strings.Join(addrs, ".0.") {
					rb.RemoveAttribute(attrName)
					continue
				}
			} else {
				rb.RemoveAttribute(attrName)
				continue
			}
		}

		// For optional only attributes, remove it from the output config if it holds the default value
		var dstr string
		switch schAttr.AttributeType {
		case cty.Number:
			dstr = "0"
		case cty.Bool:
			dstr = "false"
		case cty.String:
			dstr = `""`
		default:
			if schAttr.AttributeType.IsListType() || schAttr.AttributeType.IsSetType() {
				dstr = "[]"
				break
			}
			if schAttr.AttributeType.IsMapType() {
				dstr = "{}"
				break
			}
		}
		if schAttr.Default != nil {
			dstr = fmt.Sprintf("%#v", schAttr.Default)
		}
		attrExpr, diags := hclwrite.ParseConfig(attrVal.BuildTokens(nil).Bytes(), "generate_attr", hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf(`building attribute %q attribute: %s`, attrName, diags.Error())
		}
		attrValLit := strings.TrimSpace(string(attrExpr.Body().GetAttribute(attrName).Expr().BuildTokens(nil).Bytes()))
		if attrValLit == dstr {
			rb.RemoveAttribute(attrName)
			continue
		}
	}

	for _, blkVal := range rb.Blocks() {
		if sch.NestedBlocks[blkVal.Type()].Computed {
			rb.RemoveBlock(blkVal)
			continue
		}
		if err := tuneForBlock(blkVal.Body(), sch.NestedBlocks[blkVal.Type()].Block, append(parentAttrNames, blkVal.Type())); err != nil {
			return err
		}
	}
	return nil
}
