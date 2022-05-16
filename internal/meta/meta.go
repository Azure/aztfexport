package meta

import "github.com/Azure/aztfy/internal/config"

const ResourceMappingFileName = ".aztfyResourceMapping.json"

type Meta interface {
	Init() error
	ResourceGroupName() string
	Workspace() string
	ListResource() (ImportList, error)
	CleanTFState(addr string)
	Import(item *ImportItem)
	GenerateCfg(l ImportList) error
	ExportResourceMapping(l ImportList) error
}

func NewMeta(cfg config.Config) (Meta, error) {
	if cfg.MockClient {
		return newMetaDummy(cfg.ResourceGroupName)
	}
	return newMetaImpl(cfg)
}
