package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/Azure/aztfexport/pkg/config"

	"github.com/Azure/aztfexport/internal/resmap"
	"github.com/Azure/aztfexport/internal/tfaddr"
	"github.com/magodo/armid"
)

type MetaMap struct {
	baseMeta
	mappingFile string
}

func NewMetaMap(cfg config.Config) (*MetaMap, error) {
	cfg.Logger.Info("New map meta")
	baseMeta, err := NewBaseMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	meta := &MetaMap{
		baseMeta:    *baseMeta,
		mappingFile: cfg.MappingFile,
	}

	return meta, nil
}

func (meta MetaMap) ScopeName() string {
	return meta.mappingFile
}

func (meta *MetaMap) ListResource(_ context.Context) (ImportList, error) {
	var m resmap.ResourceMapping

	meta.Logger().Debug("Read resource set from mapping file")
	b, err := os.ReadFile(meta.mappingFile)
	if err != nil {
		return nil, fmt.Errorf("reading mapping file %s: %v", meta.mappingFile, err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshalling the mapping file: %v", err)
	}

	var l ImportList
	for id, res := range m {
		if stringMatchAnyRegexp(id, meta.excludeAzureResources) {
			continue
		}
		if stringEqualFoldAnyStrings(res.ResourceType, meta.excludeTerraformResources) {
			continue
		}
		azureId, err := armid.ParseResourceId(id)
		if err != nil {
			return nil, fmt.Errorf("parsing resource id %q: %v", id, err)
		}
		tfAddr := tfaddr.TFAddr{
			Type: res.ResourceType,
			Name: res.ResourceName,
		}
		item := ImportItem{
			AzureResourceID: azureId,
			TFResourceId:    res.ResourceId,
			TFAddrCache:     tfAddr,
			TFAddr:          tfAddr,
			Recommendations: []string{res.ResourceType},
		}
		l = append(l, item)
	}

	sort.Slice(l, func(i, j int) bool {
		return l[i].AzureResourceID.String() < l[j].AzureResourceID.String()
	})

	return l, nil
}
