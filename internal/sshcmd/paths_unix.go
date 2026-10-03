//go:build darwin || linux

package sshcmd

import (
	"fmt"
	"os"
)

var (
	SSHPath  = "/usr/bin/ssh"
	SFTPPath = "/usr/bin/sftp"
)

func RequireSSHClient() error {
	return requireClient(SSHPath, "ssh")
}

func RequireSFTPClient() error {
	return requireClient(SFTPPath, "sftp")
}

func requireClient(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("OpenSSH %s client %q is unavailable: %w", name, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("OpenSSH %s client %q is not a regular file", name, path)
	}
	return nil
}
