//go:build windows

package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsLocalShellUsesPowerShellWithoutUnixFlags(t *testing.T) {
	argv, err := localShell()
	if err != nil {
		t.Skip(err)
	}
	if !strings.EqualFold(filepath.Base(argv[0]), "powershell.exe") {
		t.Fatalf("local shell = %q", argv[0])
	}
	if len(argv) != 2 || argv[1] != "-NoLogo" {
		t.Fatalf("PowerShell arguments = %#v", argv[1:])
	}
}
