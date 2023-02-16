package cfgfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func GetInstallationIdFromCLI() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("retrieving user's HOME dir")
	}
	path := filepath.Join(home, ".azure", "azureProfile.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %v", path, err)
	}
	// Removing the preceding BOM (Byte Order Mark)
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var f struct {
		InstallationId string `json:"installationId"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return "", fmt.Errorf("unmarshalling the file: %v", err)
	}
	if f.InstallationId == "" {
		return "", fmt.Errorf("no installation id found")
	}
	return f.InstallationId, nil
}

func GetInstallationIdFromPWSH() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("retrieving user's HOME dir")
	}
	path := filepath.Join(home, ".azure", "AzureRmContextSettings.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %v", path, err)
	}
	var f struct {
		Settings struct {
			InstallationId string `json:"InstallationId"`
		} `json:"Settings"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return "", fmt.Errorf("unmarshalling the file: %v", err)
	}
	if f.Settings.InstallationId == "" {
		return "", fmt.Errorf("no installation id found")
	}
	return f.Settings.InstallationId, nil
}
