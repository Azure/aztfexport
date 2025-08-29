package resourceset

import (
	"log/slog"
	"sort"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"

	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
	"github.com/magodo/workerpool"
)

type AzureResourceSet struct {
	Resources []AzureResource
}

type AzureResource struct {
	Id armid.ResourceId
}

type PesudoResourceInfo struct {
	TFType string
	TFId   string
}

func (rset AzureResourceSet) ToTFAzureRMResources(logger *slog.Logger, parallelism int, cred azcore.TokenCredential, clientOpt arm.ClientOptions) []TFResource {
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
			logger.Warn("Failed to query resource type", "id", res.resid, "error", res.err)
			// Still put this unresolved resource in the resource set, so that users can later specify the expected TF resource type.
			tfresources = append(tfresources, TFResource{
				AzureId: res.resid,
				// Use the azure ID as the TF ID as a fallback
				TFId: res.resid.String(),
			})
		} else {
			if !res.exact {
				// It is not possible to return multiple result when API is used.
				logger.Warn("No query result for resource type and TF id", "id", res.resid)
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
			tftypes, tfids, exact, err := aztft.QueryTypeAndId(res.Id.String(),
				&aztft.APIOption{
					Cred:         cred,
					ClientOption: clientOpt,
				},
			)
			return result{
				resid:   res.Id,
				tftypes: tftypes,
				tfids:   tfids,
				exact:   exact,
				err:     err,
			}, nil
		})
	}

	// #nosec G104
	wp.Done()

	sort.Slice(tfresources, func(i, j int) bool {
		return tfresources[i].AzureId.String() < tfresources[j].AzureId.String()
	})

	return tfresources
}

func (rset AzureResourceSet) ToTFAzAPIResources() (result []TFResource) {
	for _, res := range rset.Resources {
		result = append(result, TFResource{
			AzureId: res.Id,
			TFId:    res.Id.String(),
			TFType:  "azapi_resource",
		})
	}
	return
}
