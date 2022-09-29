package meta

import (
	"context"
	"fmt"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/resourceset"
	"github.com/Azure/aztfy/internal/tfaddr"
	"github.com/magodo/armid"
	"github.com/magodo/azlist/azlist"
)

var _ Meta = &MetaResourceGroup{}

type MetaResourceGroup struct {
	baseMeta
	resourceGroup      string
	resourceNamePrefix string
	resourceNameSuffix string
}

func newMetaResourceGroup(cfg config.Config) (Meta, error) {
	baseMeta, err := NewBaseMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	meta := &MetaResourceGroup{
		baseMeta:      *baseMeta,
		resourceGroup: cfg.ResourceGroupName,
	}
	meta.resourceNamePrefix, meta.resourceNameSuffix = resourceNamePattern(cfg.ResourceNamePattern)

	return meta, nil
}

func (meta MetaResourceGroup) ScopeName() string {
	return meta.resourceGroup
}

func (meta *MetaResourceGroup) ListResource() (ImportList, error) {
	ctx := context.TODO()

	rset, err := meta.queryResourceSet(ctx, meta.resourceGroup)
	if err != nil {
		return nil, err
	}
	if err := rset.PopulateResource(); err != nil {
		return nil, fmt.Errorf("tweaking single resources in the azure resource set: %v", err)
	}
	if err := rset.ReduceResource(); err != nil {
		return nil, fmt.Errorf("tweaking across resources in the azure resource set: %v", err)
	}

	rl := rset.ToTFResources()

	var l ImportList
	for i, res := range rl {
		item := ImportItem{
			AzureResourceID: res.AzureId,
			TFResourceId:    res.TFId,
			TFAddr: tfaddr.TFAddr{
				Type: "",
				Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, i, meta.resourceNameSuffix),
			},
		}
		if res.TFType != "" {
			item.Recommendations = []string{res.TFType}
			item.TFAddr.Type = res.TFType
			item.IsRecommended = true
		}

		l = append(l, item)
	}
	return l, nil
}

func (meta MetaResourceGroup) queryResourceSet(ctx context.Context, rg string) (*resourceset.AzureResourceSet, error) {
	result, err := azlist.List(ctx, meta.subscriptionId, fmt.Sprintf("resourceGroup =~ %q", rg), &azlist.Option{Parallelism: meta.parallelism, Recursive: true})
	if err != nil {
		return nil, fmt.Errorf("listing resource set: %v", err)
	}

	var rl []resourceset.AzureResource
	for _, res := range result.Resources {
		res := resourceset.AzureResource{
			Id:         res.Id,
			Properties: res.Properties,
		}
		rl = append(rl, res)
	}

	// Especially, adding the resoruce group itself to the resource set
	rl = append(rl, resourceset.AzureResource{Id: &armid.ResourceGroup{
		SubscriptionId: meta.subscriptionId,
		Name:           meta.resourceGroup,
	}})

	return &resourceset.AzureResourceSet{Resources: rl}, nil
}
