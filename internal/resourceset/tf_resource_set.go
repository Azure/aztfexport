package resourceset

import "github.com/magodo/armid"

// Key is the TF Resource Id
type TFResourceSet map[string]TFResource

type TFResource struct {
	AzureId    armid.ResourceId
	TFId       string
	TFType     string
	Properties map[string]interface{}
	DependsOn  []string // TF resource IDs
}
