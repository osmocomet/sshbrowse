//go:build windows

package sshconfig

import (
	"path/filepath"
	"testing"
)

func TestWindowsSSHConfigIncludesDrivePath(t *testing.T) {
	home, sshDir := sshHome(t)
	root := filepath.Join(sshDir, "config")
	child := filepath.Join(t.TempDir(), "included config")
	writeFile(t, child, "Host windows-include\n")
	writeFile(t, root, "Include \""+filepath.ToSlash(child)+"\"\nHost root\n")

	result := Scan(root, Options{HomeDir: home})
	if aliases := candidateAliases(result.Candidates); len(aliases) != 2 || aliases[0] != "windows-include" || aliases[1] != "root" {
		t.Fatalf("aliases = %v, warnings = %v", aliases, result.Warnings)
	}
}
