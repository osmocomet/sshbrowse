//go:build darwin || linux

package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSFTPLocalDirectoryFallsBackWhenDownloadsCannotBeEntered(t *testing.T) {
	home := t.TempDir()
	downloads := filepath.Join(home, "Downloads")
	if err := os.Mkdir(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(downloads, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(downloads, 0o755); err != nil {
			t.Errorf("restore Downloads permissions: %v", err)
		}
	})

	if got := sftpLocalDirectoryForHome(home, downloads); got != home {
		t.Fatalf("sftp local directory: got %q, want home %q", got, home)
	}
}
