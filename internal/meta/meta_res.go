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
	ResourceType string
}

func NewResMeta(cfg config.ResConfig) (*ResMeta, error) {
	baseMeta, err := NewMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}
	meta := &ResMeta{
		Meta:         *baseMeta,
		Id:           cfg.ResourceId,
		ResourceName: cfg.ResourceName,
		ResourceType: cfg.ResourceType,
	}
	return meta, nil
}

func (meta ResMeta) QueryResourceTypeAndId() (string, string, error) {
	lrt, lid, err := aztft.QueryTypeAndId(meta.Id, true)
	if err != nil {
		return "", "", err
	}
	if len(lrt) != 1 {
		return "", "", fmt.Errorf("expect exactly one resource type, got=%d", len(lrt))
	}
	if len(lid) != 1 {
		return "", "", fmt.Errorf("expect exactly one resource id, got=%d", len(lid))
	}
	return lrt[0], lid[0], nil
}

func (meta ResMeta) QueryResourceId(rt string) (string, error) {
	return aztft.QueryId(meta.Id, rt, true)
}
