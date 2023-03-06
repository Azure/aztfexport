package config

import "github.com/Azure/aztfexport/pkg/config"

type NonInteractiveModeConfig struct {
	config.Config

	MockMeta           bool
	PlainUI            bool
	GenMappingFileOnly bool
}
