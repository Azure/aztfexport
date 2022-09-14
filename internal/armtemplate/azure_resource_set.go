package armtemplate

import (
	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
	"log"
)

type AzureResourceSet struct {
	Resources []AzureResource
}

type AzureResource struct {
	Id         armid.ResourceId
	Properties interface{}
}

func (rset AzureResourceSet) ToTFResources() TFResources {
	// A temporary mapping to map from the azure ID to TF ID. This mapping assumes that azure and TF resource has 1:1 mapping.
	azToTf := map[string]string{}
	tfresources := TFResources{}
	for _, res := range rset.Resources {
		azureId := res.Id.String()

		var (
			// Use the azure ID as the TF ID as a fallback
			tfId   = azureId
			tfType string
		)
		tftypes, tfids, err := aztft.QueryTypeAndId(azureId, true)
		if err == nil {
			if len(tfids) == 1 && len(tftypes) == 1 {
				tfId = tfids[0]
				tfType = tftypes[0]
			} else {
				log.Printf("WARNING: Expect one query result for resource type and TF id for %s, got %d type and %d id.\n", azureId, len(tftypes), len(tfids))
			}
		} else {
			log.Printf("WARNING: Failed to query resource type for %s: %v\n", azureId, err)
		}

		azToTf[azureId] = tfId
		tfresources[tfId] = TFResource{
			AzureId:    azureId,
			TFId:       tfId,
			TFType:     tfType,
			Properties: res.Properties,
		}
	}

	// TODO: Resolve dependencies for TF resources, based on their Azure Id

	return tfresources
}
