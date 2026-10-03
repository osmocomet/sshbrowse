//go:build windows

package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
)

var systemConfigPath = windowsSystemConfigPath()

func windowsSystemConfigPath() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "ssh", "ssh_config")
}

func systemConfigPaths() []string {
	return []string{systemConfigPath}
}

func sameConfigPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
