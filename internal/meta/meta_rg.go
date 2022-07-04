package meta

import (
	"github.com/Azure/aztfy/internal/config"
)

const ResourceMappingFileName = ".aztfyResourceMapping.json"

type RgMeta interface {
	meta
	ResourceGroupName() string
	ListResource() (ImportList, error)
	ExportResourceMapping(l ImportList) error
}

func NewRgMeta(cfg config.RgConfig) (RgMeta, error) {
	if cfg.MockClient {
		return newRgMetaDummy(cfg.ResourceGroupName)
	}
	return newRgMetaRg(cfg)
}
