package meta

import (
	"context"
	"fmt"

	"github.com/Azure/aztfexport/internal/resourceset"
	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/magodo/azlist/azlist"
)

type MetaQuery struct {
	baseMeta
	argPredicate                 string
	recursiveQuery               bool
	resourceNamePrefix           string
	resourceNameSuffix           string
	includeRoleAssignment        bool
	includeManagedResource       bool
	includeResourceGroup         bool
	argTable                     string
	argAuthenticationScopeFilter armresourcegraph.AuthorizationScopeFilter
}

func NewMetaQuery(cfg config.Config) (*MetaQuery, error) {
	cfg.Logger.Info("New query meta")
	baseMeta, err := NewBaseMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	meta := &MetaQuery{
		baseMeta:                     *baseMeta,
		argPredicate:                 cfg.ARGPredicate,
		recursiveQuery:               cfg.RecursiveQuery,
		includeRoleAssignment:        cfg.IncludeRoleAssignment,
		includeManagedResource:       cfg.IncludeManagedResource,
		includeResourceGroup:         cfg.IncludeResourceGroup,
		argTable:                     cfg.ARGTable,
		argAuthenticationScopeFilter: armresourcegraph.AuthorizationScopeFilter(cfg.ARGAuthorizationScopeFilter),
	}
	meta.resourceNamePrefix, meta.resourceNameSuffix = resourceNamePattern(cfg.ResourceNamePattern)

	return meta, nil
}

func (meta MetaQuery) ScopeName() string {
	msg := meta.argPredicate
	if meta.recursiveQuery {
		msg += " (recursive)"
	}
	return msg
}

func (meta *MetaQuery) ListResource(ctx context.Context) (ImportList, error) {
	meta.Logger().Debug("Query resource set")
	rset, err := meta.queryResourceSet(ctx, meta.argPredicate, meta.recursiveQuery)
	if err != nil {
		return nil, err
	}
	var rl []resourceset.TFResource
	if meta.useAzAPI() {
		meta.Logger().Debug("Azure Resource set map to TF resource set")
		rl = rset.ToTFAzAPIResources()
	} else {
		meta.Logger().Debug("Reduce resource set")
		if err := rset.ReduceResource(); err != nil {
			return nil, fmt.Errorf("tweaking across resources in the azure resource set: %v", err)
		}

		meta.Logger().Debug("Azure Resource set map to TF resource set")
		rl = rset.ToTFAzureRMResources(meta.Logger(), meta.parallelism, meta.azureSDKCred, meta.azureSDKClientOpt)
	}

	var l ImportList
	for i, res := range rl {
		item := ImportItem{
			AzureResourceID: res.AzureId,
			TFResourceId:    res.TFId,
			TFAddr: tfaddr.TFAddr{
				Type: "",
				Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, i, meta.resourceNameSuffix),
			},
			TFAddrCache: tfaddr.TFAddr{
				Type: "",
				Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, i, meta.resourceNameSuffix),
			},
		}
		if res.TFType != "" {
			item.Recommendations = []string{res.TFType}
			item.TFAddr.Type = res.TFType
			item.TFAddrCache.Type = res.TFType
			item.IsRecommended = true
		}

		l = append(l, item)
	}

	l = meta.excludeImportList(l)

	return l, nil
}

func (meta MetaQuery) queryResourceSet(ctx context.Context, predicate string, recursive bool) (*resourceset.AzureResourceSet, error) {
	opt := azlist.Option{
		Logger:                      meta.logger.WithGroup("azlist"),
		SubscriptionId:              meta.subscriptionId,
		Cred:                        meta.azureSDKCred,
		ClientOpt:                   meta.azureSDKClientOpt,
		Parallelism:                 meta.parallelism,
		Recursive:                   recursive,
		IncludeResourceGroup:        meta.includeResourceGroup,
		ExtensionResourceTypes:      extBuilder{includeRoleAssignment: meta.includeRoleAssignment}.Build(),
		IncludeManaged:              meta.includeManagedResource,
		ARGTable:                    meta.argTable,
		ARGAuthorizationScopeFilter: meta.argAuthenticationScopeFilter,
	}
	lister, err := azlist.NewLister(opt)
	if err != nil {
		return nil, fmt.Errorf("building azlister: %v", err)
	}
	result, err := lister.ListByQuery(ctx, predicate)
	if err != nil {
		return nil, fmt.Errorf("listing resource set: %w", err)
	}

	var rl []resourceset.AzureResource
	for _, res := range result.Resources {
		res := resourceset.AzureResource{
			Id: res.Id,
		}
		rl = append(rl, res)
	}

	return &resourceset.AzureResourceSet{Resources: rl}, nil
}
