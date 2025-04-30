package meta

import (
	"context"
	"fmt"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/magodo/azlist/azlist"
)

type MetaResourceGroup struct {
	baseMeta
	resourceGroup         string
	resourceNamePrefix    string
	resourceNameSuffix    string
	includeRoleAssignment bool
}

func NewMetaResourceGroup(cfg config.Config) (*MetaResourceGroup, error) {
	cfg.Logger.Info("New resource group meta")
	baseMeta, err := NewBaseMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	meta := &MetaResourceGroup{
		baseMeta:              *baseMeta,
		resourceGroup:         cfg.ResourceGroupName,
		includeRoleAssignment: cfg.IncludeRoleAssignment,
	}
	meta.resourceNamePrefix, meta.resourceNameSuffix = resourceNamePattern(cfg.ResourceNamePattern)

	return meta, nil
}

func (meta MetaResourceGroup) ScopeName() string {
	return meta.resourceGroup
}

func (meta *MetaResourceGroup) ListResource(ctx context.Context) (ImportList, error) {
	meta.Logger().Debug("Query resource set")
	rset, err := meta.queryResourceSet(ctx, meta.resourceGroup)
	if err != nil {
		return nil, err
	}

	var rl []resourceset.TFResource
	if meta.useAzAPI() {
		rl = rset.ToTFAzAPIResources()
	} else {
		meta.Logger().Debug("Populate resource set")
		if err := rset.PopulateResource(); err != nil {
			return nil, fmt.Errorf("tweaking single resources in the azure resource set: %v", err)
		}
		meta.Logger().Debug("Reduce resource set")
		if err := rset.ReduceResource(); err != nil {
			return nil, fmt.Errorf("tweaking across resources in the azure resource set: %v", err)
		}

		meta.Logger().Debug("Azure Resource set map to TF resource set")
		rl = rset.ToTFAzureRMResources(meta.Logger(), meta.parallelism, meta.azureSDKCred, meta.azureSDKClientOpt)
	}

	var l ImportList
	for i, res := range rl {
		if stringMatchAnyRegexp(res.AzureId.String(), meta.excludeAzureResources) {
			continue
		}
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
			if stringEqualFoldAnyStrings(res.TFType, meta.excludeTerraformResources) {
				continue
			}
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
	var rl []resourceset.AzureResource

	// Try to get the resource group (with any extension resources) first, in case it doesn't exist.
	opt := azlist.Option{
		Logger:                 meta.logger.WithGroup("azlist"),
		SubscriptionId:         meta.subscriptionId,
		Cred:                   meta.azureSDKCred,
		ClientOpt:              meta.azureSDKClientOpt,
		Parallelism:            meta.parallelism,
		ExtensionResourceTypes: extBuilder{includeRoleAssignment: meta.includeRoleAssignment}.Build(),
		ARGTable:               "ResourceContainers",
	}
	lister, err := azlist.NewLister(opt)
	if err != nil {
		return nil, fmt.Errorf("building azlister for listing resource group only: %v", err)
	}
	result, err := lister.List(ctx, fmt.Sprintf("name == %q", rg))
	if err != nil {
		return nil, fmt.Errorf("listing resource group only: %w", err)
	}
	for _, res := range result.Resources {
		res := resourceset.AzureResource{
			Id:         res.Id,
			Properties: res.Properties,
		}
		rl = append(rl, res)
	}

	// Skip the resource listing if the resource group itself doesn't exist.
	if len(result.Resources) == 0 {
		return &resourceset.AzureResourceSet{}, nil
	}

	// List the resources within the resource group.
	opt = azlist.Option{
		Logger:                 meta.logger.WithGroup("azlist"),
		SubscriptionId:         meta.subscriptionId,
		Cred:                   meta.azureSDKCred,
		ClientOpt:              meta.azureSDKClientOpt,
		Parallelism:            meta.parallelism,
		ExtensionResourceTypes: extBuilder{includeRoleAssignment: meta.includeRoleAssignment}.Build(),
		Recursive:              true,
	}
	lister, err = azlist.NewLister(opt)
	if err != nil {
		return nil, fmt.Errorf("building azlister for listing resource group: %v", err)
	}
	result, err = lister.List(ctx, fmt.Sprintf("resourceGroup =~ %q", rg))
	if err != nil {
		return nil, fmt.Errorf("listing resource group: %w", err)
	}
	for _, res := range result.Resources {
		res := resourceset.AzureResource{
			Id:         res.Id,
			Properties: res.Properties,
		}
		rl = append(rl, res)
	}

	return &resourceset.AzureResourceSet{Resources: rl}, nil
}
