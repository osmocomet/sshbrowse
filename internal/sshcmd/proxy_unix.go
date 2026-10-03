//go:build darwin || linux

package sshcmd

import "strings"

// OpenSSH runs ProxyCommand through the user's shell. Quote every literal
// argument so profile values cannot become shell syntax.
func quoteProxyCommand(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}
