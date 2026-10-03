//go:build darwin || linux

package atomicfile

import "os"

func replace(sourcePath, targetPath string) error {
	return os.Rename(sourcePath, targetPath)
}
