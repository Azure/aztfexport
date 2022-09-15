package resourceset

import (
	"log"

	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
)

type AzureResourceSet struct {
	Resources []AzureResource
}

type AzureResource struct {
	Id         armid.ResourceId
	Properties interface{}
}

func (rset AzureResourceSet) ToTFResources() TFResourceSet {
	tfresources := TFResourceSet{}
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

		tfresources[tfId] = TFResource{
			AzureId:    res.Id,
			TFId:       tfId,
			TFType:     tfType,
			Properties: res.Properties,
		}
	}

Loop:
	for tfId, tfRes := range tfresources {
		parentId := tfRes.AzureId.Parent()

		// This resource is either a root scope or a root scoped resource
		if parentId == nil {
			// Root scope: ignore as it has no parent
			if tfRes.AzureId.ParentScope() == nil {
				continue
			}
			// Root scoped resource: use its parent scope as its parent
			parentId = tfRes.AzureId.ParentScope()
		}

		// Adding the direct parent resource as its dependency
		for oTfId, oTfRes := range tfresources {
			if tfRes.TFId == oTfRes.TFId {
				continue
			}
			if parentId.Equal(oTfRes.AzureId) {
				tfRes.DependsOn = []string{oTfId}
				tfresources[tfId] = tfRes
				continue Loop
			}
		}
	}

	return tfresources
}
