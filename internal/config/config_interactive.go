package config

import "github.com/Azure/aztfexport/pkg/config"

type InteractiveModeConfig struct {
	config.Config

	MockMeta bool
}
