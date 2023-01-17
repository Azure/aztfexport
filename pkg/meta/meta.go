package meta

import (
	"fmt"
	"github.com/Azure/aztfy/internal/meta"
	"github.com/Azure/aztfy/pkg/config"
)

type ImportItem = meta.ImportItem
type ImportList = meta.ImportList

type Meta interface {
	meta.BaseMeta
	// ScopeName returns a string indicating current scope/mode.
	ScopeName() string
	// ListResource lists the resources belong to current scope.
	ListResource() (meta.ImportList, error)
}

func NewMeta(cfg config.Config) (Meta, error) {
	switch {
	case cfg.ResourceGroupName != "":
		return meta.NewMetaResourceGroup(cfg)
	case cfg.ARGPredicate != "":
		return meta.NewMetaQuery(cfg)
	case cfg.MappingFile != "":
		return meta.NewMetaMap(cfg)
	case cfg.ResourceId != "":
		return meta.NewMetaResource(cfg)
	default:
		return nil, fmt.Errorf("invalid group config")
	}
}
