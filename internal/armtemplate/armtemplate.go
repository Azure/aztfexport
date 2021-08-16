package armtemplate

type Template struct {
	Schema         string                 `json:"$schema"`
	ContentVersion string                 `json:"contentVersion"`
	ApiProfile     string                 `json:"apiProfile,omitempty"`
	Parameters     map[string]Parameter   `json:"parameters,omitempty"`
	Variables      map[string]interface{} `json:"variables,omitempty"`
	Functions      []Function             `json:"functions,omitempty"`
	Resources      []Resource             `json:"resources"`
	Outputs        []Output               `json:"outputs,omitempty"`
}

type ParameterType string

const (
	ParameterTypeString       ParameterType = "string"
	ParameterTypeSecureString               = "securestring"
	ParameterTypeInt                        = "int"
	ParameterTypeBool                       = "bool"
	ParameterTypeObject                     = "object"
	ParameterTypeSecureObject               = "secureObject"
	ParameterTypeArray                      = "array"
)

type Parameter struct {
	Type          ParameterType      `json:"type"`
	DefaultValue  interface{}        `json:"defaultValue,omitempty"`
	AllowedValues []interface{}      `json:"allowedValues,omitempty"`
	MinValue      int                `json:"minValue,omitempty"`
	MaxValue      int                `json:"maxValue,omitempty"`
	MinLength     int                `json:"minLength,omitempty"`
	MaxLength     int                `json:"maxLength,omitempty"`
	Metadata      *ParameterMetadata `json:"metadata,omitempty"`
}

type ParameterMetadata struct {
	Description string `json:"description,omitempty"`
}

type Function struct {
	Namespace string                    `json:"namespace"`
	Members   map[string]FunctionMember `json:"members"`
}

type FunctionMember struct {
	Parameters []FunctionParameter `json:"parameters,omitempty"`
	Output     FunctionOutput      `json:"output"`
}

type FunctionParameter struct {
	Name string        `json:"name"`
	Type ParameterType `json:"type"`
}

type FunctionOutput struct {
	Type  ParameterType `json:"type"`
	Value interface{}   `json:"value"`
}

type Resource struct {
	Condition  *bool             `json:"condition,omitempty"`
	Type       string            `json:"type"`
	ApiVersion string            `json:"apiVersion"`
	Name       string            `json:"name"`
	Comments   string            `json:"comments,omitempty"`
	Location   string            `json:"location,omitempty"`
	DependsOn  []Dependency      `json:"dependsOn,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	Sku        *ResourceSku      `json:"sku,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Scope      string            `json:"scope,omitempty"`
	//TODO Copy
}

type Dependency string

type ResourceSku struct {
	Name     string `json:"name"`
	Tier     string `json:"tier,omitempty"`
	Size     string `json:"size,omitempty"`
	Family   string `json:family,omitempty"`
	Capacity string `json:"capacity,omitempty"`
}
