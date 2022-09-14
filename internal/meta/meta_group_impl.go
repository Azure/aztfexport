package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/magodo/armid"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Azure/aztfy/internal/armtemplate"
	"github.com/Azure/aztfy/internal/resmap"
	"github.com/Azure/aztfy/internal/tfaddr"

	"github.com/Azure/aztfy/internal/config"
)

var _ GroupMeta = &MetaGroupImpl{}

type MetaGroupImpl struct {
	Meta

	// Only non empty when in resource group mode
	resourceGroup string

	argQuery  string
	resources armtemplate.TFResources

	// Key is azure resource id; Value is terraform resource addr.
	// For azure resources not in this mapping, they are all initialized as to skip.
	resourceMapping resmap.ResourceMapping

	resourceNamePrefix string
	resourceNameSuffix string
}

func newGroupMetaImpl(cfg config.GroupConfig) (GroupMeta, error) {
	baseMeta, err := NewMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	argQuery := cfg.ARGQuery
	if argQuery == "" {
		argQuery = fmt.Sprintf("Resources | where resourceGroup =~ %q", cfg.ResourceGroupName)
	}

	meta := &MetaGroupImpl{
		Meta:            *baseMeta,
		resourceGroup:   cfg.ResourceGroupName,
		argQuery:        argQuery,
		resourceMapping: cfg.ResourceMapping,
	}

	if pos := strings.LastIndex(cfg.ResourceNamePattern, "*"); pos != -1 {
		meta.resourceNamePrefix, meta.resourceNameSuffix = cfg.ResourceNamePattern[:pos], cfg.ResourceNamePattern[pos+1:]
	} else {
		meta.resourceNamePrefix = cfg.ResourceNamePattern
	}

	return meta, nil
}

func (meta MetaGroupImpl) ScopeName() string {
	if meta.resourceGroup != "" {
		return meta.resourceGroup
	}
	return meta.argQuery
}

func (meta *MetaGroupImpl) ListResource() (ImportList, error) {
	ctx := context.TODO()

	rset, err := meta.queryResourceSet(ctx)
	if err != nil {
		return nil, err
	}
	if err := rset.TweakResources(); err != nil {
		return nil, fmt.Errorf("populating managed resources in the azure resource set: %v", err)
	}
	meta.resources = rset.ToTFResources()

	var l ImportList

	rl := []armtemplate.TFResource{}
	for _, res := range meta.resources {
		rl = append(rl, res)
	}
	sort.Slice(rl, func(i, j int) bool {
		return rl[i].AzureId < rl[j].AzureId
	})

	for i, res := range rl {
		item := ImportItem{
			ResourceID: res.TFId,
			TFAddr: tfaddr.TFAddr{
				Type: "",
				Name: fmt.Sprintf("%s%d%s", meta.resourceNamePrefix, i, meta.resourceNameSuffix),
			},
		}
		if res.TFType != "" {
			item.Recommendations = []string{res.TFType}
		}

		if len(meta.resourceMapping) != 0 {
			if addr, ok := meta.resourceMapping[res.TFId]; ok {
				item.TFAddr = addr
			}
		} else {
			// Only auto deduce the TF resource type from recommendations when there is no resource mapping file specified.
			if res.TFType != "" {
				item.TFAddr.Type = res.TFType
				item.IsRecommended = true
			}
		}
		l = append(l, item)
	}
	return l, nil
}

func (meta MetaGroupImpl) GenerateCfg(l ImportList) error {
	return meta.Meta.generateCfg(l, meta.Meta.lifecycleAddon, meta.addDependency)
}

func (meta MetaGroupImpl) ExportResourceMapping(l ImportList) error {
	m := resmap.ResourceMapping{}
	for _, item := range l {
		m[item.ResourceID] = item.TFAddr
	}
	output := filepath.Join(meta.Workspace(), ResourceMappingFileName)
	b, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return fmt.Errorf("JSON marshalling the resource mapping: %v", err)
	}
	if err := os.WriteFile(output, b, 0644); err != nil {
		return fmt.Errorf("writing the resource mapping to %s: %v", output, err)
	}
	return nil
}

func (meta MetaGroupImpl) queryResourceSet(ctx context.Context) (*armtemplate.AzureResourceSet, error) {
	client, err := meta.Meta.clientBuilder.NewResourceGraphClient()
	if err != nil {
		return nil, fmt.Errorf("building resource graph client: %v", err)
	}

	scopeFilter := armresourcegraph.AuthorizationScopeFilterAtScopeAndBelow
	resultFormat := armresourcegraph.ResultFormatObjectArray
	query := armresourcegraph.QueryRequest{
		Query: &meta.argQuery,
		Options: &armresourcegraph.QueryRequestOptions{
			AuthorizationScopeFilter: &scopeFilter,
			ResultFormat:             &resultFormat,
		},
		Subscriptions: []*string{&meta.subscriptionId},
	}
	resp, err := client.Resources(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("running ARG query %q: %v", meta.argQuery, err)
	}

	var rl []armtemplate.AzureResource
	for _, resource := range resp.QueryResponse.Data.([]interface{}) {
		id := resource.(map[string]interface{})["id"].(string)
		azureId, err := armid.ParseResourceId(id)
		if err != nil {
			return nil, fmt.Errorf("parsing resource id %s: %v", id, err)
		}
		rl = append(rl, armtemplate.AzureResource{
			Id:         azureId,
			Properties: resource,
		})
	}

	// Especially, if this is for resource group, adding the resoruce group itself to the resource set
	if meta.resourceGroup != "" {
		rl = append(rl, armtemplate.AzureResource{Id: &armid.ResourceGroup{
			SubscriptionId: meta.subscriptionId,
			Name:           meta.resourceGroup,
		}})
	}

	return &armtemplate.AzureResourceSet{Resources: rl}, nil
}

func (meta MetaGroupImpl) addDependency(configs ConfigInfos) (ConfigInfos, error) {
	configSet := map[string]ConfigInfo{}
	for _, cfg := range configs {
		configSet[cfg.ResourceID] = cfg
	}

	// Iterate each config to add dependency by querying the dependency info from azure resource set.
	var out ConfigInfos
	for tfid, cfg := range configSet {
		tfres, ok := meta.resources[tfid]
		if !ok {
			return nil, fmt.Errorf("can't find resource %q in the arm template's resources", tfid)
		}

		if len(tfres.DependsOn) != 0 {
			if err := hclBlockAppendDependency(cfg.hcl.Body().Blocks()[0].Body(), tfres.DependsOn, configSet); err != nil {
				return nil, err
			}
		}
		out = append(out, cfg)
	}

	return out, nil
}
