//go:build darwin || linux

package app

import "golang.org/x/sys/unix"

func canEnterDirectory(path string) bool {
	return unix.Access(path, unix.X_OK) == nil
}
