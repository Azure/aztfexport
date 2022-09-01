package config

import (
	"github.com/Azure/aztfy/internal/resmap"
)

type Config interface {
	isConfig()
}

type CommonConfig struct {
	SubscriptionId string
	OutputDir      string
	Overwrite      bool
	Append         bool
	DevProvider    bool
	BatchMode      bool
	BackendType    string
	BackendConfig  []string
	FullConfig     bool
}

type RgConfig struct {
	CommonConfig

	ResourceGroupName   string
	ResourceMapping     resmap.ResourceMapping
	ResourceNamePattern string
	MockClient          bool
}

func (RgConfig) isConfig() {}

type ResConfig struct {
	CommonConfig

	// Azure resource id
	ResourceId string

	// TF resource name
	ResourceName string

	// TF resource type. If this is empty, then uses aztft to deduce the correct resource type.
	ResourceType string
}

func (ResConfig) isConfig() {}
