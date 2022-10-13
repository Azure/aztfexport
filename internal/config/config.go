package config

type CommonConfig struct {
	LogPath             string
	SubscriptionId      string
	OutputDir           string
	Overwrite           bool
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
	ParallelImport      bool
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
