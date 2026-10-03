//go:build windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
)

func localShell() ([]string, error) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = os.Getenv("WINDIR")
	}
	if root == "" {
		root = `C:\Windows`
	}
	shell := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if info, err := os.Stat(shell); err != nil {
		return nil, fmt.Errorf("local terminal: Windows PowerShell is unavailable at %q: %w", shell, err)
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("local terminal: Windows PowerShell at %q is not a regular file", shell)
	}
	// Do not use Unix login flags or change PowerShell's execution policy. The
	// standard Windows PowerShell host is interactive under ConPTY by default.
	return []string{shell, "-NoLogo"}, nil
}
