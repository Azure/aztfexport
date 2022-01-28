package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/meowgorithm/babyenv"
)

type Config struct {
	ResourceGroupName string            // specified via CLI
	Logfile           string            `env:"AZTFY_LOGFILE" default:""`
	Debug             bool              `env:"AZTFY_DEBUG" default:"false"`
	MockClient        bool              `env:"AZTFY_MOCK_CLIENT" default:"false"`
	OutputDir         string            // specified via CLI option
	ResourceMapping   map[string]string // specified via CLI option
}

func NewConfig(rg, outputDir, mappingFile string) (*Config, error) {
	var cfg Config
	if err := babyenv.Parse(&cfg); err != nil {
		return nil, err
	}

	if mappingFile != "" {
		b, err := os.ReadFile(mappingFile)
		if err != nil {
			return nil, fmt.Errorf("reading mapping file %s: %v", mappingFile, err)
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("unmarshalling the mapping file: %v", err)
		}
		cfg.ResourceMapping = m
	}

	cfg.ResourceGroupName = rg
	cfg.OutputDir = outputDir
	return &cfg, nil
}
