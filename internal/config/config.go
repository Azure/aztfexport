package config

import "github.com/Azure/aztfy/internal/resmap"

type Config struct {
	SubscriptionId      string                 `env:"AZTFY_SUBSCRIPTION_ID" default:""`
	ResourceGroupName   string                 // specified via CLI
	Logfile             string                 `env:"AZTFY_LOGFILE" default:""`
	MockClient          bool                   `env:"AZTFY_MOCK_CLIENT" default:"false"`
	OutputDir           string                 // specified via CLI option
	ResourceMapping     resmap.ResourceMapping // specified via CLI option
	ResourceNamePattern string                 // specified via CLI option
	Overwrite           bool                   // specified via CLI option
	BatchMode           bool                   // specified via CLI option
	BackendType         string                 // specified via CLI option
	BackendConfig       []string               // specified via CLI option
}
