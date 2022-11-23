package utils

import (
	"fmt"
	"io"
	"os"
)

func DirIsEmpty(path string) (bool, error) {
	stat, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, fmt.Errorf("the path %q doesn't exist", path)
	}
	if !stat.IsDir() {
		return false, fmt.Errorf("the path %q is not a directory", path)
	}
	// #nosec G304
	dir, err := os.Open(path)
	if err != nil {
		return false, err
	}
	_, err = dir.Readdirnames(1)
	if err != nil {
		if err == io.EOF {
			if err := dir.Close(); err != nil {
				return false, fmt.Errorf("closing dir %s: %v", path, err)
			}
			return true, nil
		}
		return false, err
	}
	if err := dir.Close(); err != nil {
		return false, fmt.Errorf("closing dir %s: %v", path, err)
	}
	return false, nil
}
