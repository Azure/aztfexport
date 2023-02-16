package cfgfile

import (
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const CfgDirName = ".aztfy"
const CfgFileName = "config.json"

type Configuration struct {
	InstallationId   string `json:"installation_id"`
	TelemetryEnabled bool   `json:"telemetry_enabled"`
}

func UpdateConfiguration(old Configuration, k, v string) (*Configuration, error) {
	b, err := json.Marshal(old)
	if err != nil {
		return nil, fmt.Errorf("marshalling the old configuration: %v", err)
	}
	var vjson interface{}
	if err := json.Unmarshal([]byte(v), &vjson); err != nil {
		return nil, fmt.Errorf("unmarshalling the value: %v", err)
	}
	if !gjson.Get(string(b), k).Exists() {
		return nil, fmt.Errorf("invalid key %q", k)
	}
	updated, err := sjson.Set(string(b), k, vjson)
	if err != nil {
		return nil, fmt.Errorf("setting the value: %v", err)
	}
	var cfg Configuration
	if err := json.Unmarshal([]byte(updated), &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling the new configuration: %v", err)
	}
	return &cfg, nil
}
