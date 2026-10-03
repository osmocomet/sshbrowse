//go:build windows

package sshcmd

import "testing"

func TestQuoteWindowsProxyCommand(t *testing.T) {
	argv := []string{`C:\Program Files\OpenSSH\ssh.exe`, "-i", `C:\Users\Core User\.ssh\key`, "-W", "[%h]:%p", "--", "user@bastion"}
	want := `"C:\Program Files\OpenSSH\ssh.exe" -i "C:\Users\Core User\.ssh\key" -W [%h]:%p -- user@bastion`
	if got := quoteProxyCommand(argv); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
