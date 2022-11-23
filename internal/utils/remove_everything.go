package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

func RemoveEverythingUnder(path string) error {
	// #nosec G304
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", path, err)
	}
	entries, _ := dir.Readdirnames(0)
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry)); err != nil {
			return fmt.Errorf("failed to remove %s: %v", entry, err)
		}
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("closing dir %s: %v", path, err)
	}
	return nil
}
