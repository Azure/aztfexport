package config

import "github.com/Azure/aztfy/pkg/config"

type InteractiveModeConfig struct {
	config.Config

	MockMeta bool
}
