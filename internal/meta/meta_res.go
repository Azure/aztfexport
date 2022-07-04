package meta

import (
	"fmt"
	"github.com/Azure/aztfy/internal/config"
	"github.com/magodo/aztft/aztft"
)

type ResMeta struct {
	Meta
	Id           string
	ResourceName string
}

func NewResMeta(cfg config.ResConfig) (*ResMeta, error) {
	baseMeta, err := NewMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}
	meta := &ResMeta{
		Meta:         *baseMeta,
		Id:           cfg.ResourceId.String(),
		ResourceName: cfg.ResourceName,
	}
	return meta, nil
}

func (meta ResMeta) QueryResourceType() (string, error) {
	l, err := aztft.Query(meta.Id, true)
	if err != nil {
		return "", err
	}
	if len(l) != 1 {
		return "", fmt.Errorf("expect exactly one resource type, got=%d", len(l))
	}
	return l[0], nil
}
