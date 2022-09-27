package resmap

type ResourceMapEntity struct {
	// TF resource ID
	ResourceId string `json:"resource_id"`
	// TF resource type
	ResourceType string `json:"resource_type"`
	// TF resource name
	ResourceName string `json:"resource_name"`
}

// ResourceMapping is the resource mapping file, the key is the Azure resource Id in uppercase.
type ResourceMapping map[string]ResourceMapEntity
