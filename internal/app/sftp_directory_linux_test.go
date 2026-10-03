//go:build linux

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSFTPLocalDirectoryUsesXDGDownload(t *testing.T) {
	if expected := os.Getenv("SSHBROWSE_TEST_XDG_DOWNLOAD"); expected != "" {
		got, err := sftpLocalDirectory()
		if err != nil || got != expected {
			t.Fatalf("sftp local directory = %q, %v; want %q", got, err, expected)
		}
		return
	}

	home := t.TempDir()
	configured := filepath.Join(home, "Transfers")
	for _, dir := range []string{filepath.Join(home, "Downloads"), configured, filepath.Join(home, ".config")} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(home, ".config", "user-dirs.dirs")
	if err := os.WriteFile(config, []byte("XDG_DOWNLOAD_DIR=\"$HOME/Transfers\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestSFTPLocalDirectoryUsesXDGDownload$")
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DOWNLOAD_DIR=", "SSHBROWSE_TEST_XDG_DOWNLOAD="+configured)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configured Downloads test: %v\n%s", err, output)
	}
}
