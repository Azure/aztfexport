package schema

// A modified version based on: github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema/core_schema.go

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/zclconf/go-cty/cty"
)

type NestingMode int

const (
	nestingModeInvalid NestingMode = iota
	NestingSingle
	NestingGroup
	NestingList
	NestingSet
	NestingMap
)

func FromProviderSchemaMap(providerschemas map[string]*schema.Schema) *SchemaBlock {
	if len(providerschemas) == 0 {
		return &SchemaBlock{}
	}

	ret := &SchemaBlock{
		Attributes:   map[string]*SchemaAttribute{},
		NestedBlocks: map[string]*SchemaBlockType{},
	}

	for name, ps := range providerschemas {
		if ps.Elem == nil {
			ret.Attributes[name] = fromProviderSchemaAttribute(ps)
			continue
		}
		if ps.Type == schema.TypeMap {
			if _, isResource := ps.Elem.(*schema.Resource); isResource {
				sch := *ps
				sch.Elem = &schema.Schema{
					Type: schema.TypeString,
				}
				ret.Attributes[name] = fromProviderSchemaAttribute(&sch)
				continue
			}
		}
		switch ps.ConfigMode {
		case schema.SchemaConfigModeAttr:
			ret.Attributes[name] = fromProviderSchemaAttribute(ps)
		case schema.SchemaConfigModeBlock:
			ret.NestedBlocks[name] = fromProviderSchemaBlock(ps)
		default: // SchemaConfigModeAuto, or any other invalid value
			if ps.Computed && !ps.Optional {
				// Computed-only schemas are always handled as attributes,
				// because they never appear in configuration.
				ret.Attributes[name] = fromProviderSchemaAttribute(ps)
				continue
			}
			switch ps.Elem.(type) {
			case *schema.Schema, schema.ValueType:
				ret.Attributes[name] = fromProviderSchemaAttribute(ps)
			case *schema.Resource:
				ret.NestedBlocks[name] = fromProviderSchemaBlock(ps)
			default:
				// Should never happen for a valid schema
				panic(fmt.Errorf("invalid Schema.Elem %#v; need *schema.Schema or *schema.Resource", ps.Elem))
			}
		}
	}

	return ret
}

func fromProviderSchemaAttribute(ps *schema.Schema) *SchemaAttribute {
	reqd := ps.Required
	opt := ps.Optional
	if reqd && ps.DefaultFunc != nil {
		v, err := ps.DefaultFunc()
		if err != nil || (err == nil && v != nil) {
			reqd = false
			opt = true
		}
	}

	return &SchemaAttribute{
		AttributeType: fromProviderSchemaType(ps),
		Optional:      opt,
		Required:      reqd,
		Computed:      ps.Computed,
	}
}

func fromProviderSchemaBlock(ps *schema.Schema) *SchemaBlockType {
	ret := &SchemaBlockType{
		Required: ps.Required,
		Optional: ps.Optional,
		Computed: ps.Computed,
	}

	if nested := fromProviderResource(ps.Elem.(*schema.Resource)); nested != nil {
		ret.Block = nested
	}

	switch ps.Type {
	case schema.TypeList:
		ret.NestingMode = NestingList
	case schema.TypeSet:
		ret.NestingMode = NestingSet
	case schema.TypeMap:
		ret.NestingMode = NestingMap
	default:
		// Should never happen for a valid schema
		panic(fmt.Errorf("invalid s.Type %s for s.Elem being resource", ps.Type))
	}

	return ret
}

func fromProviderSchemaType(ps *schema.Schema) cty.Type {
	switch ps.Type {
	case schema.TypeString:
		return cty.String
	case schema.TypeBool:
		return cty.Bool
	case schema.TypeInt, schema.TypeFloat:
		return cty.Number
	case schema.TypeList, schema.TypeSet, schema.TypeMap:
		var elemType cty.Type
		switch set := ps.Elem.(type) {
		case *schema.Schema:
			elemType = fromProviderSchemaType(set)
		case schema.ValueType:
			elemType = fromProviderSchemaType(&schema.Schema{Type: set})
		case *schema.Resource:
			elemType = fromProviderResource(set).ImpliedType()
		default:
			if set != nil {
				panic(fmt.Errorf("invalid Schema.Elem %#v; need *schema.Schema or *schema.Resource", ps.Elem))
			}
			elemType = cty.String
		}
		switch ps.Type {
		case schema.TypeList:
			return cty.List(elemType)
		case schema.TypeSet:
			return cty.Set(elemType)
		case schema.TypeMap:
			return cty.Map(elemType)
		default:
			panic("invalid collection type")
		}
	default:
		panic(fmt.Errorf("invalid Schema.Type %s", ps.Type))
	}
}

func fromProviderResource(pr *schema.Resource) *SchemaBlock {
	return FromProviderSchemaMap(pr.Schema)
}
