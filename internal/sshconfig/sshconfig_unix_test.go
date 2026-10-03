//go:build darwin || linux

package sshconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnreadableIncludedFileIsAWarning(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files without permission bits")
	}
	home, directory := sshHome(t)
	root := filepath.Join(directory, "config")
	unreadable := filepath.Join(directory, "unreadable")
	writeFile(t, root, "Include unreadable\nHost root\n")
	writeFile(t, unreadable, "Host hidden\n")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(unreadable, 0o600); err != nil {
			t.Errorf("restore unreadable fixture permissions: %v", err)
		}
	})

	result := Scan(root, Options{HomeDir: home})
	if aliases := candidateAliases(result.Candidates); !reflect.DeepEqual(aliases, []string{"root"}) {
		t.Fatalf("candidates = %v", aliases)
	}
	if !hasWarning(result.Warnings, "permission denied") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}
