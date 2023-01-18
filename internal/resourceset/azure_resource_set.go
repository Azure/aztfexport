package resourceset

import (
	"sort"

	"github.com/Azure/aztfy/pkg/log"

	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
	"github.com/magodo/workerpool"
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

func (rset AzureResourceSet) ToTFResources(parallelism int) []TFResource {
	tfresources := []TFResource{}

	wp := workerpool.NewWorkPool(parallelism)

	type result struct {
		resid   armid.ResourceId
		tftypes []aztft.Type
		tfids   []string
		exact   bool
		err     error
	}

	wp.Run(func(v interface{}) error {
		res := v.(result)
		if res.err != nil {
			log.Printf("[WARN] Failed to query resource type for %s: %v\n", res.resid, res.err)
			// Still put this unresolved resource in the resource set, so that users can later specify the expected TF resource type.
			tfresources = append(tfresources, TFResource{
				AzureId: res.resid,
				// Use the azure ID as the TF ID as a fallback
				TFId: res.resid.String(),
			})
		} else {
			if !res.exact {
				// It is not possible to return multiple result when API is used.
				log.Printf("[WARN] No query result for resource type and TF id for %s\n", res.resid)
				// Still put this unresolved resource in the resource set, so that users can later specify the expected TF resource type.
				tfresources = append(tfresources, TFResource{
					AzureId: res.resid,
					// Use the azure ID as the TF ID as a fallback
					TFId: res.resid.String(),
				})
			} else {
				for i := range res.tfids {
					tfresources = append(tfresources, TFResource{
						AzureId: res.tftypes[i].AzureId,
						TFId:    res.tfids[i],
						TFType:  res.tftypes[i].TFType,
					})
				}
			}
		}
		return nil
	})

	for _, res := range rset.Resources {
		res := res
		wp.AddTask(func() (interface{}, error) {
			tftypes, tfids, exact, err := aztft.QueryTypeAndId(res.Id.String(), true)
			return result{
				resid:   res.Id,
				tftypes: tftypes,
				tfids:   tfids,
				exact:   exact,
				err:     err,
			}, nil
		})
	}

	wp.Done()

	sort.Slice(tfresources, func(i, j int) bool {
		return tfresources[i].AzureId.String() < tfresources[j].AzureId.String()
	})

	return tfresources
}
