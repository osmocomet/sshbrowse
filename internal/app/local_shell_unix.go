//go:build darwin || linux

package app

import (
	"os"
	"runtime"
)

func localShell() ([]string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		// GUI launches may not provide SHELL. Keep the macOS default while
		// using Fedora's standard interactive shell on Linux.
		shell = "/bin/bash"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	return []string{shell, "-il"}, nil
}
