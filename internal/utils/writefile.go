package utils

import "os"

func WriteFileSync(name string, data []byte, perm os.FileMode) error {
	// #nosec G304
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	// #nosec G307
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}
