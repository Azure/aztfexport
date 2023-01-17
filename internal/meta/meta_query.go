package meta

import (
	"context"
	"fmt"
	"github.com/Azure/aztfy/internal/resourceset"
	"github.com/Azure/aztfy/internal/tfaddr"
	"github.com/Azure/aztfy/pkg/config"
	"github.com/Azure/aztfy/pkg/log"
	"github.com/magodo/azlist/azlist"
)

type MetaQuery struct {
	baseMeta
	argPredicate       string
	recursiveQuery     bool
	resourceNamePrefix string
	resourceNameSuffix string
}

func NewMetaQuery(cfg config.Config) (*MetaQuery, error) {
	log.Printf("[INFO] New query meta")
	baseMeta, err := NewBaseMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	meta := &MetaQuery{
		baseMeta:       *baseMeta,
		argPredicate:   cfg.ARGPredicate,
		recursiveQuery: cfg.RecursiveQuery,
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
	rset, err := meta.queryResourceSet(ctx, meta.argPredicate, meta.recursiveQuery)
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
	return l, nil
}

func (meta MetaQuery) queryResourceSet(ctx context.Context, predicate string, recursive bool) (*resourceset.AzureResourceSet, error) {
	result, err := azlist.List(ctx, meta.subscriptionId, predicate, &azlist.Option{Parallelism: meta.parallelism, Recursive: recursive})
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

	return &resourceset.AzureResourceSet{Resources: rl}, nil
}
