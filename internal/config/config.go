package config

import "github.com/Azure/aztfy/internal/resmap"

type Config struct {
	LogPath             string
	MockClient          bool
	SubscriptionId      string
	ResourceMapping     resmap.ResourceMapping
	ResourceGroupName   string
	OutputDir           string
	ResourceNamePattern string
	Overwrite           bool
	BatchMode           bool
	BackendType         string
	BackendConfig       []string
}
