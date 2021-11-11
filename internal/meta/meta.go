package meta

import "github.com/Azure/aztfy/internal/config"

type Meta interface {
	Init() error
	ResourceGroupName() string
	Workspace() string
	ListResource() ImportList
	CleanTFState()
	Import(item ImportItem) error
	GenerateCfg(l ImportList) error
}

func NewMeta(cfg config.Config) (Meta, error) {
	if cfg.MockClient {
		return newMetaDummy(cfg.ResourceGroupName)
	}
	return newMetaImpl(cfg.ResourceGroupName, cfg.OutputDir)
}
