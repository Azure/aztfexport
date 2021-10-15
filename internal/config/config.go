package config

import "github.com/meowgorithm/babyenv"

type Config struct {
	ResourceGroupName string // specified via CLI
	Logfile           string `env:"AZTFY_LOGFILE" default:""`
	Debug             bool   `env:"AZTFY_DEBUG" default:"false"`
	MockClient        bool   `env:"AZTFY_MOCK_CLIENT" default:"false"`
	OutputDir         string // specified via CLI option
}

func NewConfig(rg string, outputDir string) (*Config, error) {
	var cfg Config
	if err := babyenv.Parse(&cfg); err != nil {
		return nil, err
	}
	cfg.ResourceGroupName = rg
	cfg.OutputDir = outputDir
	return &cfg, nil
}
