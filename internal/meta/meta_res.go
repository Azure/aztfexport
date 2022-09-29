package meta

import (
	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/resourceset"
	"github.com/Azure/aztfy/internal/tfaddr"
	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
)

type MetaResource struct {
	baseMeta
	AzureId      armid.ResourceId
	ResourceName string
	ResourceType string
}

func newMetaResource(cfg config.Config) (Meta, error) {
	baseMeta, err := NewBaseMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	id, err := armid.ParseResourceId(cfg.ResourceId)
	if err != nil {
		return nil, err
	}
	meta := &MetaResource{
		baseMeta:     *baseMeta,
		AzureId:      id,
		ResourceName: cfg.TFResourceName,
		ResourceType: cfg.TFResourceType,
	}
	return meta, nil
}

func (meta MetaResource) ScopeName() string {
	return meta.AzureId.String()
}

func (meta *MetaResource) ListResource() (ImportList, error) {
	resourceSet := resourceset.AzureResourceSet{
		Resources: []resourceset.AzureResource{
			{
				Id: meta.AzureId,
			},
		},
	}
	rl := resourceSet.ToTFResources()
	var l ImportList
	for _, res := range rl {
		item := ImportItem{
			AzureResourceID: res.AzureId,
			TFResourceId:    res.TFId, // this might be empty if have multiple matches in aztft
			TFAddr: tfaddr.TFAddr{
				Type: res.TFType, //this might be empty if have multiple matches in aztft
				Name: meta.ResourceName,
			},
		}

		// Some special Azure resource is missing the essential property that is used by aztft to detect their TF resource type.
		// In this case, users can use the `--type` option to manually specify the TF resource type.
		if meta.ResourceType != "" {
			if meta.AzureId.Equal(res.AzureId) {
				tfid, err := aztft.QueryId(meta.AzureId.String(), meta.ResourceType, true)
				if err != nil {
					return nil, err
				}
				item.TFResourceId = tfid
				item.TFAddr.Type = meta.ResourceType
			}
		}

		l = append(l, item)
	}

	return l, nil
}
