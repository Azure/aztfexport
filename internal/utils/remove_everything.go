package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

//	RemoveEverythingUnder removes everything under a path.
//
// The top level directory entries whose name matches any "skipps" will be skipped.
func RemoveEverythingUnder(path string, skipps ...string) error {
	// #nosec G304
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %v", path, err)
	}

	skipMap := map[string]bool{}
	for _, v := range skipps {
		skipMap[v] = true
	}

	entries, _ := dir.Readdirnames(0)
	for _, entry := range entries {
		if skipMap[entry] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(path, entry)); err != nil {
			return fmt.Errorf("failed to remove %s: %v", entry, err)
		}
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("closing dir %s: %v", path, err)
	}
	return nil
}
