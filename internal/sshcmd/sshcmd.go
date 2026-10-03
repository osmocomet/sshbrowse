// Package sshcmd turns a saved connection into an OpenSSH command line.
package sshcmd

import (
	"strconv"
	"strings"

	"sshbrowse/internal/profile"
)

// Argv builds the ssh command for a connection. Only fields the user set are
// passed, so ssh_config still applies to everything else. A connection
// imported from ssh_config has the alias as its host and nothing else set.
func Argv(connection profile.Connection) []string {
	argv := connectionOptions(connection, SSHPath, "-p")
	if connection.AgentForwarding {
		argv = append(argv, "-A")
	}
	if connection.X11Forwarding {
		argv = append(argv, "-X")
	}
	for _, spec := range connection.LocalForwards {
		argv = append(argv, "-L", spec)
	}
	for _, spec := range connection.RemoteForwards {
		argv = append(argv, "-R", spec)
	}
	for _, spec := range connection.DynamicForwards {
		argv = append(argv, "-D", spec)
	}
	// "--" ends option parsing, so a host that starts with "-" cannot become an option.
	return append(argv, "--", destination(connection.User, connection.Host))
}

// SFTPArgv builds the sftp command for the same connection. sftp reads
// ssh_config like ssh but takes only some of the flags: the port flag is -P,
// and port forwards and X11 do not apply to a file transfer.
func SFTPArgv(connection profile.Connection) []string {
	argv := connectionOptions(connection, SFTPPath, "-P")
	if connection.AgentForwarding {
		argv = append(argv, "-A")
	}
	return append(argv, "--", sftpDestination(connection.User, connection.Host))
}

// Connection-specific transport options apply to terminal sessions, SFTP,
// and saved jump hops. Unset values retain OpenSSH configuration and defaults.
func connectionOptions(connection profile.Connection, client, portFlag string) []string {
	argv := []string{client}
	if connection.Port > 0 {
		argv = append(argv, portFlag, strconv.Itoa(connection.Port))
	}
	if connection.IdentityFile != "" {
		argv = append(argv, "-i", connection.IdentityFile)
	}
	if connection.JumpHost != "" {
		argv = append(argv, "-J", connection.JumpHost)
	}
	return argv
}

func sftpDestination(user, host string) string {
	// Unlike ssh, sftp treats a colon as the separator before a remote path.
	// Brackets keep an IPv6 literal intact as the destination host.
	if strings.Contains(host, ":") && !(strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]")) {
		host = "[" + host + "]"
	}
	return destination(user, host)
}

func destination(user, host string) string {
	if user != "" {
		return user + "@" + host
	}
	return host
}
