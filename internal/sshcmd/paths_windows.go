//go:build windows

package sshcmd

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	// Windows OpenSSH is deliberately resolved to the Microsoft optional
	// feature location. PATH, Git for Windows and WSL are not substitutes.
	SSHPath  = windowsOpenSSHPath("ssh.exe")
	SFTPPath = windowsOpenSSHPath("sftp.exe")
)

func windowsOpenSSHPath(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = os.Getenv("WINDIR")
	}
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "OpenSSH", name)
}

func RequireSSHClient() error {
	return requireWindowsClient(SSHPath, "ssh")
}

func RequireSFTPClient() error {
	return requireWindowsClient(SFTPPath, "sftp")
}

func requireWindowsClient(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("Windows OpenSSH %s client is unavailable at %q; install the Windows OpenSSH Client optional feature: %w", name, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Windows OpenSSH %s client at %q is not a regular file", name, path)
	}
	return nil
}
