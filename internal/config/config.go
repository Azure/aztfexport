package config

type Config struct {
	ResourceGroupName   string // specified via CLI
	Logfile             string `env:"AZTFY_LOGFILE" default:""`
	Debug               bool   `env:"AZTFY_DEBUG" default:"false"`
	MockClient          bool   `env:"AZTFY_MOCK_CLIENT" default:"false"`
	OutputDir           string // specified via CLI option
	ResourceMappingFile string // specified via CLI option
	ResourceNamePattern string // specified via CLI option
	Overwrite           bool   // specified via CLI option
	BatchMode           bool   // specified via CLI option
}
