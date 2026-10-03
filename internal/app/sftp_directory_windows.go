//go:build windows

package app

import "os"

func canEnterDirectory(path string) bool {
	dir, err := os.Open(path)
	if err != nil {
		return false
	}
	return dir.Close() == nil
}
