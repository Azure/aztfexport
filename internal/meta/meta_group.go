package meta

import (
	"github.com/Azure/aztfy/internal/config"
)

const ResourceMappingFileName = ".aztfyResourceMapping.json"

type GroupMeta interface {
	meta
	ScopeName() string
	ListResource() (ImportList, error)
	ExportResourceMapping(l ImportList) error
}

func NewGroupMeta(cfg config.GroupConfig) (GroupMeta, error) {
	if cfg.MockClient {
		return newGroupMetaDummy(cfg.ResourceGroupName)
	}
	return newGroupMetaImpl(cfg)
}
