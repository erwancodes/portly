//go:build windows

package main

import "os"

func replaceConfigFile(tmp, path string) error {
	// os.Rename cannot replace an existing file on Windows. Remove only the
	// known config target after the atomic temp write has completed.
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp, path)
}
