package meta

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/magodo/armid"
	"github.com/magodo/azlist/azlist"
	"github.com/magodo/aztft/aztft"
)

type MetaResource struct {
	baseMeta
	AzureIds              []armid.ResourceId
	ResourceName          string
	ResourceType          string
	resourceNamePrefix    string
	resourceNameSuffix    string
	includeRoleAssignment bool
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
		baseMeta:              *baseMeta,
		AzureIds:              ids,
		ResourceName:          cfg.TFResourceName,
		ResourceType:          cfg.TFResourceType,
		includeRoleAssignment: cfg.IncludeRoleAssignment,
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
	var resources []resourceset.AzureResource
	for _, id := range meta.AzureIds {
		resources = append(resources, resourceset.AzureResource{Id: id})
	}

	rset := &resourceset.AzureResourceSet{
		Resources: resources,
	}
	if meta.includeRoleAssignment {
		var err error
		rset, err = meta.queryResourceSet(ctx, resources)
		if err != nil {
			return nil, fmt.Errorf("querying resource set: %v", err)
		}
	}

	meta.Logger().Debug("Azure Resource set map to TF resource set")

	var rl []resourceset.TFResource
	if meta.useAzAPI() {
		rl = rset.ToTFAzAPIResources()
	} else {
		rl = rset.ToTFAzureRMResources(meta.Logger(), meta.parallelism, meta.azureSDKCred, meta.azureSDKClientOpt)
	}

	var l ImportList

	originalRl, extensionRl := splitOriginalAndExtension(rl, resources)

	// The ResourceName and ResourceType are only honored for single non-role-assignment-resource
	if len(originalRl) == 1 {
		res := originalRl[0]

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
		meta.appendRlToImportList(originalRl, &l)
	}

	meta.appendRlToImportList(extensionRl, &l)

	l = meta.excludeImportList(l)

	return l, nil
}

func (meta MetaResource) appendRlToImportList(rl []resourceset.TFResource, l *ImportList) {
	for _, res := range rl {
		tfAddr := tfaddr.TFAddr{
			Type: "",
			Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, len(*l), meta.resourceNameSuffix),
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

		*l = append(*l, item)
	}
}

func (meta MetaResource) queryResourceSet(ctx context.Context, resources []resourceset.AzureResource) (*resourceset.AzureResourceSet, error) {
	var rl []resourceset.AzureResource
	var quotedIds []string

	opt := azlist.Option{
		Logger:                 meta.logger.WithGroup("azlist"),
		SubscriptionId:         meta.subscriptionId,
		Cred:                   meta.azureSDKCred,
		ClientOpt:              meta.azureSDKClientOpt,
		Parallelism:            meta.parallelism,
		ExtensionResourceTypes: extBuilder{includeRoleAssignment: meta.includeRoleAssignment}.Build(),
	}

	lister, err := azlist.NewLister(opt)
	if err != nil {
		return nil, fmt.Errorf("building azlister for listing resources: %v", err)
	}

	for _, res := range resources {
		quotedIds = append(quotedIds, fmt.Sprintf("%q", res.Id.String()))
	}

	result, err := lister.List(ctx, fmt.Sprintf("id in (%s)", strings.Join(quotedIds, ", ")))
	if err != nil {
		return nil, fmt.Errorf("listing resources: %w", err)
	}

	for _, res := range result.Resources {
		res := resourceset.AzureResource{
			Id:         res.Id,
			Properties: res.Properties,
		}
		rl = append(rl, res)
	}

	return &resourceset.AzureResourceSet{
		Resources: rl,
	}, nil
}

func splitOriginalAndExtension(combined []resourceset.TFResource, requested []resourceset.AzureResource) (original []resourceset.TFResource, extension []resourceset.TFResource) {
	idRequested := make(map[string]bool, len(requested))
	for _, res := range requested {
		idRequested[res.Id.String()] = true
	}

	for _, res := range combined {
		if idRequested[res.AzureId.String()] {
			original = append(original, res)
		} else {
			extension = append(extension, res)
		}
	}

	return
}
