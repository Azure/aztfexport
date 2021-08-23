package schema

import "github.com/zclconf/go-cty/cty"

// This is a simplified and modified version of the hashicorp/terraform-json.
// The motivation for this is to add more information that is lost during the conversion from plugin sdk (v2) to the terraform core schema.
// (github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema/core_schema.go)
// Specifically, we are:
// 1. adding Required, Optional, Computed for the SchemaBlockType
// 2. adding Default for the SchemaAttribute
// 3. adding ExactlyOneOf, AtLeastOneOf, ConflictsWith and RequiredWith for both SchemaBlockType and the SchemaAttribute
// 4. removing any other attributes

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

	ConflictsWith []string `json:"conflicts_with,omitempty"`
	ExactlyOneOf  []string `json:"exactly_one_of,omitempty"`
	AtLeastOneOf  []string `json:"at_least_one_of,omitempty"`
	RequiredWith  []string `json:"required_with,omitempty"`
}

type SchemaAttribute struct {
	AttributeType cty.Type `json:"type,omitempty"`

	Required bool        `json:"required,omitempty"`
	Optional bool        `json:"optional,omitempty"`
	Computed bool        `json:"computed,omitempty"`
	Default  interface{} `json:"default,omitempty"`

	ConflictsWith []string `json:"conflicts_with,omitempty"`
	ExactlyOneOf  []string `json:"exactly_one_of,omitempty"`
	AtLeastOneOf  []string `json:"at_least_one_of,omitempty"`
	RequiredWith  []string `json:"required_with,omitempty"`
}
