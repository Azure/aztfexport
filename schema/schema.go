package schema

import "github.com/zclconf/go-cty/cty"

// This is a simplified and modified version of the hashicorp/terraform-json.
// The motivation for this is to add more information that is lost during the conversion from plugin sdk (v2) to the terraform core schema.
// (github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema/core_schema.go)
// Specifically, we are adding the Required, Optional, Computed for the SchemaBlockType, adding Defualt for the SchemaAttribute, and removing any other attributes.

type ProviderSchema struct {
	ResourceSchemas map[string]*Schema `json:"resource_schemas,omitempty"`
}

type Schema struct {
	Block *SchemaBlock `json:"block,omitempty"`
}

type SchemaBlock struct {
	Attributes   map[string]*SchemaAttribute `json:"attributes,omitempty"`
	NestedBlocks map[string]*SchemaBlockType `json:"block_types,omitempty"`
}

type SchemaBlockType struct {
	NestingMode NestingMode  `json:"nesting_mode,omitempty"`
	Block       *SchemaBlock `json:"block,omitempty"`

	Required bool `json:"required,omitempty"`
	Optional bool `json:"optional,omitempty"`
	Computed bool `json:"computed,omitempty"`
}

type SchemaAttribute struct {
	AttributeType cty.Type `json:"type,omitempty"`

	Required bool        `json:"required,omitempty"`
	Optional bool        `json:"optional,omitempty"`
	Computed bool        `json:"computed,omitempty"`
	Default  interface{} `json:"default,omitempty"`
}
