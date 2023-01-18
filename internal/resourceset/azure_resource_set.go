package resourceset

import (
	"sort"

	"github.com/Azure/aztfy/pkg/log"

	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
)

type AzureResourceSet struct {
	Resources []AzureResource
}

type AzureResource struct {
	Id         armid.ResourceId
	Properties map[string]interface{}
}

type PesudoResourceInfo struct {
	TFType string
	TFId   string
}

func (rset AzureResourceSet) ToTFResources() []TFResource {
	tfresources := []TFResource{}
	for _, res := range rset.Resources {
		azureId := res.Id.String()
		tftypes, tfids, exact, err := aztft.QueryTypeAndId(azureId, true)
		if err != nil {
			log.Printf("[WARN] Failed to query resource type for %s: %v\n", azureId, err)
			// Still put this unresolved resource in the resource set, so that users can later specify the expected TF resource type.
			tfresources = append(tfresources, TFResource{
				AzureId: res.Id,
				// Use the azure ID as the TF ID as a fallback
				TFId: azureId,
			})
		} else {
			if !exact {
				// It is not possible to return multiple result when API is used.
				log.Printf("[WARN] No query result for resource type and TF id for %s\n", azureId)
				// Still put this unresolved resource in the resource set, so that users can later specify the expected TF resource type.
				tfresources = append(tfresources, TFResource{
					AzureId: res.Id,
					// Use the azure ID as the TF ID as a fallback
					TFId: azureId,
				})
			} else {
				for i := range tfids {
					tfresources = append(tfresources, TFResource{
						AzureId: tftypes[i].AzureId,
						TFId:    tfids[i],
						TFType:  tftypes[i].TFType,
					})
				}
			}
		}

	}

	sort.Slice(tfresources, func(i, j int) bool {
		return tfresources[i].AzureId.String() < tfresources[j].AzureId.String()
	})

	return tfresources
}
