package cfgfile

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const CfgDirName = ".aztfexport"
const CfgFileName = "config.json"

type Configuration struct {
	InstallationId   string `json:"installation_id"`
	TelemetryEnabled bool   `json:"telemetry_enabled"`
}

func GetKey(key string) (interface{}, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("retrieving the user's HOME directory: %v", err)
	}
	path := filepath.Join(homeDir, CfgDirName, CfgFileName)
	// #nosec G304
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config: %v", err)
	}
	// #nosec G307
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading config: %v", err)
	}

	result := gjson.Get(string(b), key)
	if !result.Exists() {
		return "", fmt.Errorf("invalid key")
	}
	return result.Value(), nil
}

func SetKey(key, value string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("retrieving the user's HOME directory: %v", err)
	}
	path := filepath.Join(homeDir, CfgDirName, CfgFileName)
	// #nosec G304
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config: %v", err)
	}

	var cfg Configuration
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("unmarshalling the config: %v", err)
	}
	newCfg, err := updateConfiguration(cfg, key, value)
	if err != nil {
		return err
	}
	b, err = json.Marshal(*newCfg)
	if err != nil {
		return fmt.Errorf("marshalling the updated config: %v", err)
	}
	// #nosec G304
	f, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open config for writing: %v", err)
	}

	// #nosec G307
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("writing config: %v", err)
	}
	return nil
}

func GetConfig() (*Configuration, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("retrieving the user's HOME directory: %v", err)
	}
	path := filepath.Join(homeDir, CfgDirName, CfgFileName)
	// #nosec G304
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config: %v", err)
	}
	// #nosec G307
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading config: %v", err)
	}

	var v Configuration
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func updateConfiguration(old Configuration, k, v string) (*Configuration, error) {
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
