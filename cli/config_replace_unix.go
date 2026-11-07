//go:build !windows

package main

import "os"

func replaceConfigFile(tmp, path string) error {
	return os.Rename(tmp, path)
}
