package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Azure/aztfy/internal/armtemplate"
	"github.com/Azure/aztfy/internal/resmap"
	"github.com/Azure/aztfy/internal/tfaddr"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

var _ RgMeta = &MetaRgImpl{}

type MetaRgImpl struct {
	Meta
	resourceGroup string
	resources     armtemplate.TFResources

	// Key is azure resource id; Value is terraform resource addr.
	// For azure resources not in this mapping, they are all initialized as to skip.
	resourceMapping resmap.ResourceMapping

	resourceNamePrefix string
	resourceNameSuffix string
}

func newRgMetaRg(cfg config.RgConfig) (RgMeta, error) {
	baseMeta, err := NewMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	meta := &MetaRgImpl{
		Meta:            *baseMeta,
		resourceGroup:   cfg.ResourceGroupName,
		resourceMapping: cfg.ResourceMapping,
	}

	if pos := strings.LastIndex(cfg.ResourceNamePattern, "*"); pos != -1 {
		meta.resourceNamePrefix, meta.resourceNameSuffix = cfg.ResourceNamePattern[:pos], cfg.ResourceNamePattern[pos+1:]
	} else {
		meta.resourceNamePrefix = cfg.ResourceNamePattern
	}

	return meta, nil
}

func (meta MetaRgImpl) ResourceGroupName() string {
	return meta.resourceGroup
}

func (meta *MetaRgImpl) ListResource() (ImportList, error) {
	ctx := context.TODO()

	if err := meta.exportArmTemplate(ctx); err != nil {
		return nil, err
	}

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

func (meta MetaRgImpl) GenerateCfg(l ImportList) error {
	return meta.Meta.generateCfg(l, meta.Meta.lifecycleAddon, meta.resolveDependency)
}

func (meta MetaRgImpl) ExportResourceMapping(l ImportList) error {
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

func (meta *MetaRgImpl) exportArmTemplate(ctx context.Context) error {
	client, err := meta.Meta.clientBuilder.NewResourceGroupClient(meta.subscriptionId)
	if err != nil {
		return fmt.Errorf("building resource group client: %v", err)
	}

	exportOpt := "SkipAllParameterization"
	resourceOpt := "*"
	poller, err := client.BeginExportTemplate(ctx, meta.resourceGroup, armresources.ExportTemplateRequest{
		Resources: []*string{&resourceOpt},
		Options:   &exportOpt,
	}, nil)
	if err != nil {
		return fmt.Errorf("exporting arm template of resource group %s: %w", meta.resourceGroup, err)
	}
	resp, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: 10 * time.Second})
	if err != nil {
		return fmt.Errorf("waiting for exporting arm template of resource group %s: %w", meta.resourceGroup, err)
	}

	// The response has been read into the ".Template" field as an interface, and the reader has been drained.
	// As we have defined some (useful) types for the arm template, so we will do a json marshal then unmarshal here
	// to convert the ".Template" (interface{}) into that artificial type.
	raw, err := json.Marshal(resp.ResourceGroupExportResult.Template)
	if err != nil {
		return fmt.Errorf("marshalling the template: %w", err)
	}
	var tpl armtemplate.Template
	if err := json.Unmarshal(raw, &tpl); err != nil {
		return fmt.Errorf("unmarshalling the template: %w", err)
	}
	if err := tpl.TweakResources(); err != nil {
		return fmt.Errorf("populating managed resources in the ARM template: %v", err)
	}
	meta.resources = tpl.ToTFResources(meta.subscriptionId, meta.resourceGroup)
	return nil
}

func (meta MetaRgImpl) resolveDependency(configs ConfigInfos) (ConfigInfos, error) {
	configSet := map[string]ConfigInfo{}
	for _, cfg := range configs {
		configSet[cfg.ResourceID] = cfg
	}

	// Iterate each config to add dependency by querying the dependency info from arm template.
	var out ConfigInfos
	rgid := armtemplate.ResourceGroupId.ID(meta.subscriptionId, meta.resourceGroup)
	for id, cfg := range configSet {
		if id == rgid {
			out = append(out, cfg)
			continue
		}
		// This should never happen as we always ensure there is at least one implicit dependency on the resource group for each resource.
		tfres, ok := meta.resources[id]
		if !ok {
			return nil, fmt.Errorf("can't find resource %q in the arm template's resources", id)
		}

		if err := hclBlockAppendDependency(cfg.hcl.Body().Blocks()[0].Body(), tfres.DependsOn, configSet); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}

	return out, nil
}
