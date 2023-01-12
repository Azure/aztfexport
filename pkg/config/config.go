package config

import "github.com/hashicorp/go-hclog"

type CommonConfig struct {
	LogLevel            hclog.Level
	SubscriptionId      string
	OutputDir           string
	Append              bool
	DevProvider         bool
	Batch               bool
	ContinueOnError     bool
	BackendType         string
	BackendConfig       []string
	FullConfig          bool
	Parallelism         int
	PlainUI             bool
	GenerateMappingFile bool
	HCLOnly             bool
	ModulePath          string
}

type Config struct {
	CommonConfig
	MockClient bool

	// Exactly one of below is non empty
	ResourceGroupName string
	ARGPredicate      string
	MappingFile       string
	ResourceId        string

	// Rg and query mode
	ResourceNamePattern string

	// Query mode only
	RecursiveQuery bool

	// Resource mode only
	TFResourceName string
	TFResourceType string // TF resource type. If this is empty, then uses aztft to deduce the correct resource type.
}
