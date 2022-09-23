package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Azure/aztfy/internal/armschema"
	"github.com/Azure/aztfy/internal/config"
	"github.com/magodo/armid"
	"github.com/magodo/aztft/aztft"
)

type ResMeta struct {
	Meta
	AzureId      armid.ResourceId
	ResourceName string
	ResourceType string
}

func NewResMeta(cfg config.ResConfig) (*ResMeta, error) {
	baseMeta, err := NewMeta(cfg.CommonConfig)
	if err != nil {
		return nil, err
	}

	id, err := armid.ParseResourceId(cfg.ResourceId)
	if err != nil {
		return nil, err
	}
	meta := &ResMeta{
		Meta:         *baseMeta,
		AzureId:      id,
		ResourceName: cfg.ResourceName,
		ResourceType: cfg.ResourceType,
	}
	return meta, nil
}

func (meta ResMeta) GetAzureResource(ctx context.Context) (map[string]interface{}, error) {
	armschemas, err := armschema.GetARMSchemas()
	if err != nil {
		return nil, err
	}

	rt := meta.AzureId.TypeString()
	versions, ok := armschemas[strings.ToUpper(rt)]
	if !ok {
		return nil, fmt.Errorf("no resource type %q found in the ARM schema", rt)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no available api version defined for resource type %q in the ARM schema", rt)
	}
	version := versions[len(versions)-1]

	resp, err := meta.resourceClient.GetByID(ctx, meta.AzureId.String(), version, nil)
	if err != nil {
		return nil, fmt.Errorf("getting Azure resource %s: %v", meta.AzureId, err)
	}
	b, err := json.Marshal(resp.GenericResource)
	if err != nil {
		return nil, fmt.Errorf("marshal resource model for %s: %v", meta.AzureId, err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(b, &body); err != nil {
		return nil, fmt.Errorf("unmarshal resource model for %s: %v", meta.AzureId, err)
	}
	return body, nil
}

func (meta ResMeta) QueryResourceTypeAndId() (string, string, error) {
	lrt, lid, err := aztft.QueryTypeAndId(meta.AzureId.String(), true)
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
	return aztft.QueryId(meta.AzureId.String(), rt, true)
}
