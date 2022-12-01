package meta

import (
	"fmt"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/log"
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
	log.Printf("[INFO] New resource meta")
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

	// This is to record known resource types. In case there is a known resource type and there comes another same typed resource,
	// then we need to modify the resource name. Otherwise, there will be a resource address conflict.
	// See https://github.com/Azure/aztfy/issues/275 for an example.
	rtCnt := map[string]int{}

	var l ImportList
	for _, res := range rl {
		name := meta.ResourceName
		rtCnt[res.TFType]++
		if rtCnt[res.TFType] > 1 {
			name += fmt.Sprintf("-%d", rtCnt[res.TFType]-1)
		}
		item := ImportItem{
			AzureResourceID: res.AzureId,
			TFResourceId:    res.TFId, // this might be empty if have multiple matches in aztft
			TFAddr: tfaddr.TFAddr{
				Type: res.TFType, //this might be empty if have multiple matches in aztft
				Name: name,
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
