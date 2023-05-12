package utils

import (
	"fmt"
	"os"
)

func CopyFile(src, dst string) error {
	stat, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stating source file %s: %w", src, err)
	}
	// #nosec G304
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading from %s: %v", src, err)
	}
	if err := os.WriteFile(dst, b, stat.Mode()); err != nil {
		return fmt.Errorf("writing to %s: %v", dst, err)
	}
	return nil
}
