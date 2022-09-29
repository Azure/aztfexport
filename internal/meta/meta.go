package meta

import (
	"fmt"

	"github.com/Azure/aztfy/internal/config"
)

type TFConfigTransformer func(configs ConfigInfos) (ConfigInfos, error)

type Meta interface {
	BaseMeta
	ScopeName() string
	ListResource() (ImportList, error)
}

func NewMeta(cfg config.Config) (Meta, error) {
	if cfg.MockClient {
		return newGroupMetaDummy(cfg.ResourceGroupName)
	}

	switch {
	case cfg.ResourceGroupName != "":
		return newMetaResourceGroup(cfg)
	case cfg.ARGPredicate != "":
		return newMetaQuery(cfg)
	case cfg.MappingFile != "":
		return newMetaMap(cfg)
	case cfg.ResourceId != "":
		return newMetaResource(cfg)
	default:
		return nil, fmt.Errorf("invalid group config")
	}
}
