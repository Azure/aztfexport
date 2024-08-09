package meta

import (
	"context"
	"fmt"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
)

type MetaResource struct {
	baseMeta
	AzureIds           []armid.ResourceId
	ResourceName       string
	ResourceType       string
	resourceNamePrefix string
	resourceNameSuffix string
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
		baseMeta:     *baseMeta,
		AzureIds:     ids,
		ResourceName: cfg.TFResourceName,
		ResourceType: cfg.TFResourceType,
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

	meta.Logger().Debug("Azure Resource set map to TF resource set")

	var rl []resourceset.TFResource
	if meta.useAzAPI() {
		rl = rset.ToTFAzAPIResources()
	} else {
		rl = rset.ToTFAzureRMResources(meta.Logger(), meta.parallelism, meta.azureSDKCred, meta.azureSDKClientOpt)
	}

	var l ImportList

	// The ResourceName and ResourceType are only honored for single resource
	if len(rl) == 1 {
		res := rl[0]

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
		return l, nil
	}

	// Multi-resource mode only honors the resourceName[Pre|Suf]fix
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
			item.TFAddr.Type = res.TFType
			item.TFAddrCache.Type = res.TFType
			item.Recommendations = []string{res.TFType}
			item.IsRecommended = true
		}

		l = append(l, item)
	}

	return l, nil
}
