package meta

import (
	"context"
	"fmt"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/magodo/armid"
	"github.com/magodo/azlist/azlist"
	"github.com/magodo/aztft/aztft"
)

type MetaResource struct {
	baseMeta
	AzureIds               []armid.ResourceId
	ResourceName           string
	ResourceType           string
	resourceNamePrefix     string
	resourceNameSuffix     string
	includeRoleAssignment  bool
	includeManagedResource bool
	includeResourceGroup   bool
	recursive              bool
}

func NewMetaResource(cfg config.Config) (*MetaResource, error) {
	cfg.Logger.Info("New resource meta")
	baseMeta, err := NewBaseMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	var ids []armid.ResourceId

	for _, id := range cfg.ResourceIds {
		id, err := armid.ParseResourceId(id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	meta := &MetaResource{
		baseMeta:               *baseMeta,
		AzureIds:               ids,
		ResourceName:           cfg.TFResourceName,
		ResourceType:           cfg.TFResourceType,
		recursive:              cfg.RecursiveQuery,
		includeRoleAssignment:  cfg.IncludeRoleAssignment,
		includeManagedResource: cfg.IncludeManagedResource,
		includeResourceGroup:   cfg.IncludeResourceGroup,
	}

	meta.resourceNamePrefix, meta.resourceNameSuffix = resourceNamePattern(cfg.ResourceNamePattern)

	return meta, nil
}

func (meta MetaResource) ScopeName() string {
	if len(meta.AzureIds) == 1 {
		return meta.AzureIds[0].String()
	} else {
		return meta.AzureIds[0].String() + " and more..."
	}
}

func (meta *MetaResource) ListResource(ctx context.Context) (ImportList, error) {
	var rl []resourceset.AzureResource
	for _, id := range meta.AzureIds {
		rl = append(rl, resourceset.AzureResource{Id: id})
	}

	rl, err := meta.listByIds(ctx, rl)
	if err != nil {
		return nil, fmt.Errorf("listing resources: %v", err)
	}

	meta.Logger().Debug("Azure Resource set map to TF resource set")
	rset := &resourceset.AzureResourceSet{Resources: rl}
	var tfl []resourceset.TFResource
	if meta.useAzAPI() {
		tfl = rset.ToTFAzAPIResources()
	} else {
		tfl = rset.ToTFAzureRMResources(meta.Logger(), meta.parallelism, meta.azureSDKCred, meta.azureSDKClientOpt)
	}

	// Split the specified resources and the extension resources
	var tfrl, tfel []resourceset.TFResource
	for _, tfres := range tfl {
		rmap := map[string]bool{}
		for _, r := range rl {
			rmap[r.Id.String()] = true
		}
		if rmap[tfres.AzureId.String()] {
			tfrl = append(tfrl, tfres)
		} else {
			tfel = append(tfel, tfres)
		}
	}

	var l ImportList

	// The ResourceName and ResourceType are only honored for single non-role-assignment-resource
	if len(tfrl) == 1 {
		res := tfrl[0]

		// Honor the ResourceName
		name := meta.ResourceName
		if name == "" {
			name = fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, 0, meta.resourceNameSuffix)
		}

		// Honor the ResourceType
		tftype := res.TFType
		tfid := res.TFId
		if meta.ResourceType != "" && meta.ResourceType != res.TFType {
			// res.TFType can be either empty (if aztft failed to query), or not.
			// If the user has specified a different type, then use it.
			tftype = meta.ResourceType

			// Also use this resource type to requery its resource id.
			var err error
			tfid, err = aztft.QueryId(res.AzureId.String(), meta.ResourceType,
				&aztft.APIOption{
					Cred:         meta.azureSDKCred,
					ClientOption: meta.azureSDKClientOpt,
				})
			if err != nil {
				return nil, err
			}
		}

		tfAddr := tfaddr.TFAddr{
			Type: tftype,
			Name: name,
		}

		item := ImportItem{
			AzureResourceID: res.AzureId,
			TFResourceId:    tfid,
			TFAddr:          tfAddr,
			TFAddrCache:     tfAddr,
		}
		l = append(l, item)
	} else {
		l = append(l, meta.toImportList(tfrl, 0)...)
	}
	l = append(l, meta.toImportList(tfel, len(tfrl))...)

	l = meta.excludeImportList(l)
	return l, nil
}

func (meta MetaResource) toImportList(rl []resourceset.TFResource, fromIdx int) ImportList {
	var l ImportList
	for idx, res := range rl {
		tfAddr := tfaddr.TFAddr{
			Type: "",
			Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, idx+fromIdx, meta.resourceNameSuffix),
		}
		item := ImportItem{
			AzureResourceID: res.AzureId,
			TFResourceId:    res.TFId,
			TFAddr:          tfAddr,
			TFAddrCache:     tfAddr,
		}
		if res.TFType != "" {
			item.TFAddr.Type = res.TFType
			item.TFAddrCache.Type = res.TFType
			item.Recommendations = []string{res.TFType}
			item.IsRecommended = true
		}

		l = append(l, item)
	}
	return l
}

func (meta MetaResource) listByIds(ctx context.Context, resources []resourceset.AzureResource) ([]resourceset.AzureResource, error) {
	opt := azlist.Option{
		Logger:                 meta.logger.WithGroup("azlist"),
		SubscriptionId:         meta.subscriptionId,
		Cred:                   meta.azureSDKCred,
		ClientOpt:              meta.azureSDKClientOpt,
		Parallelism:            meta.parallelism,
		ExtensionResourceTypes: extBuilder{includeRoleAssignment: meta.includeRoleAssignment}.Build(),
		IncludeManaged:         meta.includeManagedResource,
		IncludeResourceGroup:   meta.includeResourceGroup,
		Recursive:              meta.recursive,
	}

	lister, err := azlist.NewLister(opt)
	if err != nil {
		return nil, fmt.Errorf("building azlister for listing resources: %v", err)
	}

	var ids []string
	for _, r := range resources {
		ids = append(ids, r.Id.String())
	}

	result, err := lister.ListByIds(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("azlist listing resources by ids: %w", err)
	}

	var rl []resourceset.AzureResource
	for _, res := range result.Resources {
		res := resourceset.AzureResource{
			Id:         res.Id,
			Properties: res.Properties,
		}
		rl = append(rl, res)
	}

	return rl, nil
}
