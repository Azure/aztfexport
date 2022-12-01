package meta

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/Azure/aztfy/internal/config"
	"github.com/Azure/aztfy/internal/log"
	"github.com/Azure/aztfy/internal/resmap"
	"github.com/Azure/aztfy/internal/tfaddr"
	"github.com/magodo/armid"
)

var _ Meta = &MetaMap{}

type MetaMap struct {
	baseMeta
	mappingFile string
}

func newMetaMap(cfg config.Config) (Meta, error) {
	log.Printf("[INFO] New map meta")
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

func (meta *MetaMap) ListResource() (ImportList, error) {
	var m resmap.ResourceMapping

	b, err := os.ReadFile(meta.mappingFile)
	if err != nil {
		return nil, fmt.Errorf("reading mapping file %s: %v", meta.mappingFile, err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshalling the mapping file: %v", err)
	}

	var l ImportList
	for id, res := range m {
		azureId, err := armid.ParseResourceId(id)
		if err != nil {
			return nil, fmt.Errorf("parsing resource id %q: %v", id, err)
		}
		item := ImportItem{
			AzureResourceID: azureId,
			TFResourceId:    res.ResourceId,
			TFAddr: tfaddr.TFAddr{
				Type: res.ResourceType,
				Name: res.ResourceName,
			},
			Recommendations: []string{res.ResourceType},
		}
		l = append(l, item)
	}

	sort.Slice(l, func(i, j int) bool {
		return l[i].AzureResourceID.String() < l[j].AzureResourceID.String()
	})

	return l, nil
}
