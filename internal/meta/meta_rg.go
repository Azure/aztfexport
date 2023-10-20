package meta

import (
	"context"
	"fmt"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/aztfexport/pkg/log"
	"github.com/magodo/armid"
	"github.com/magodo/azlist/azlist"
)

type MetaResourceGroup struct {
	baseMeta
	resourceGroup      string
	resourceNamePrefix string
	resourceNameSuffix string
}

func NewMetaResourceGroup(cfg config.Config) (*MetaResourceGroup, error) {
	log.Printf("[INFO] New resource group meta")
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

func (meta *MetaResourceGroup) ListResource(ctx context.Context) (ImportList, error) {
	log.Printf("[DEBUG] Query resource set")
	rset, err := meta.queryResourceSet(ctx, meta.resourceGroup)
	if err != nil {
		return nil, err
	}

	var rl []resourceset.TFResource
	if meta.useAzAPI() {
		rl = rset.ToTFAzAPIResources()
	} else {

		log.Printf("[DEBUG] Populate resource set")
		if err := rset.PopulateResource(); err != nil {
			return nil, fmt.Errorf("tweaking single resources in the azure resource set: %v", err)
		}
		log.Printf("[DEBUG] Reduce resource set")
		if err := rset.ReduceResource(); err != nil {
			return nil, fmt.Errorf("tweaking across resources in the azure resource set: %v", err)
		}

		log.Printf("[DEBUG] Azure Resource set map to TF resource set")
		rl = rset.ToTFAzureRMResources(meta.parallelism, meta.azureSDKCred, meta.azureSDKClientOpt)
	}

	var l ImportList
	for i, res := range rl {
		tfAddr := tfaddr.TFAddr{
			Type: "",
			Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, i, meta.resourceNameSuffix),
		}
		item := ImportItem{
			AzureResourceID: res.AzureId,
			TFResourceId:    res.TFId,
			TFAddr:          tfAddr,
			TFAddrCache:     tfAddr,
		}
		if res.TFType != "" {
			item.Recommendations = []string{res.TFType}
			item.TFAddr.Type = res.TFType
			item.TFAddrCache.Type = res.TFType
			item.IsRecommended = true
		}

		l = append(l, item)
	}
	return l, nil
}

func (meta MetaResourceGroup) queryResourceSet(ctx context.Context, rg string) (*resourceset.AzureResourceSet, error) {
	result, err := azlist.List(ctx, fmt.Sprintf("resourceGroup =~ %q", rg),
		azlist.Option{
			SubscriptionId: meta.subscriptionId,
			Cred:           meta.azureSDKCred,
			ClientOpt:      meta.azureSDKClientOpt,
			Parallelism:    meta.parallelism,
			Recursive:      true,
		})
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
