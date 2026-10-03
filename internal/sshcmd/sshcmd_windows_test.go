//go:build windows

package sshcmd

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"sshbrowse/internal/profile"
)

func TestWindowsMissingOpenSSHClientErrorNamesTheRequiredFeature(t *testing.T) {
	path := filepath.Join(t.TempDir(), "OpenSSH Client", "ssh.exe")
	err := requireWindowsClient(path, "ssh")
	if err == nil || !strings.Contains(err.Error(), "Windows OpenSSH ssh client") || !strings.Contains(err.Error(), "optional feature") {
		t.Fatalf("missing client error = %v", err)
	}
}

func TestWindowsCommandLinePreservesProfileArguments(t *testing.T) {
	connection := profile.Connection{
		Host:         "worker",
		User:         "core",
		IdentityFile: `C:\Users\Core User\.ssh\lab key`,
		JumpHost:     "bastion",
		LocalForwards: []string{
			"8080:localhost:80",
		},
	}
	want := Argv(connection)
	line := windows.ComposeCommandLine(want)
	got, err := windows.DecomposeCommandLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command line round trip = %#v, want %#v", got, want)
	}
}

func TestWindowsSFTPCommandLinePreservesPathArguments(t *testing.T) {
	want := SFTPArgv(profile.Connection{
		Host:         "worker",
		User:         "core",
		IdentityFile: `\\server\Share\Core User\lab key`,
		Port:         2222,
	})
	got, err := windows.DecomposeCommandLine(windows.ComposeCommandLine(want))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sftp command line round trip = %#v, want %#v", got, want)
	}
}
