//go:build windows

package sshcmd

import (
	"strings"
	"syscall"
)

// Windows OpenSSH launches ProxyCommand with CreateProcess, not a POSIX shell.
func quoteProxyCommand(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = syscall.EscapeArg(arg)
	}
	return strings.Join(quoted, " ")
}
