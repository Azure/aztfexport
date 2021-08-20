package internal

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/magodo/aztfy/schema"
)

// Currently, some special attributes are output by `terraform add`, while should be excluded.
// This is tracked in: https://github.com/hashicorp/terraform/issues/29219
// We are manually excluding these special properties here by modifying the hcl.
func tuneHCLSchemaForResource(rb *hclwrite.Body, sch *schema.Schema) {
	rb.RemoveAttribute("id")
	rb.RemoveBlock(rb.FirstMatchingBlock("timeouts", nil))
	removeComputedForBody(rb, sch.Block)
}
func removeComputedForBody(rb *hclwrite.Body, sch *schema.SchemaBlock) {
	for attrName := range rb.Attributes() {
		if sch.Attributes[attrName].Computed {
			rb.RemoveAttribute(attrName)
			continue
		}
	}

	for _, blkVal := range rb.Blocks() {
		if sch.NestedBlocks[blkVal.Type()].Computed {
			rb.RemoveBlock(blkVal)
			continue
		}
		removeComputedForBody(blkVal.Body(), sch.NestedBlocks[blkVal.Type()].Block)
	}
}
