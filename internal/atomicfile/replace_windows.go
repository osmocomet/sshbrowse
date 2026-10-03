//go:build windows

package atomicfile

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func replace(sourcePath, targetPath string) error {
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return fmt.Errorf("encode temporary path: %w", err)
	}
	target, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return fmt.Errorf("encode target path: %w", err)
	}
	if err := windows.MoveFileEx(source, target, windows.MOVEFILE_REPLACE_EXISTING); err != nil {
		return fmt.Errorf("replace %s: %w", targetPath, err)
	}
	return nil
}
