package resourceset

import "github.com/magodo/armid"

type TFResource struct {
	AzureId    armid.ResourceId
	TFId       string
	TFType     string
	Properties map[string]interface{}
}
