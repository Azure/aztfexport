package schema

// A modified version based on: github.com/hashicorp/terraform-plugin-sdk/v2/internal/configs/configschema/implied_type.go

import (
	"github.com/zclconf/go-cty/cty"
)

func (b *SchemaBlock) ImpliedType() cty.Type {
	if b == nil {
		return cty.EmptyObject
	}

	atys := make(map[string]cty.Type)

	for name, attrS := range b.Attributes {
		atys[name] = attrS.AttributeType
	}

	for name, blockS := range b.NestedBlocks {
		if _, exists := atys[name]; exists {
			panic("invalid schema, blocks and attributes cannot have the same name")
		}

		childType := blockS.Block.ImpliedType()

		switch blockS.NestingMode {
		case NestingSingle, NestingGroup:
			atys[name] = childType
		case NestingList:
			if childType.HasDynamicTypes() {
				atys[name] = cty.DynamicPseudoType
			} else {
				atys[name] = cty.List(childType)
			}
		case NestingSet:
			if childType.HasDynamicTypes() {
				panic("can't use cty.DynamicPseudoType inside a block type with NestingSet")
			}
			atys[name] = cty.Set(childType)
		case NestingMap:
			if childType.HasDynamicTypes() {
				atys[name] = cty.DynamicPseudoType
			} else {
				atys[name] = cty.Map(childType)
			}
		default:
			panic("invalid nesting type")
		}
	}

	return cty.Object(atys)
}
