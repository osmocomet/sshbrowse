package sshcmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sshbrowse/internal/profile"
)

// ParseDestination turns what someone types after `ssh ` into an unsaved
// connection: [user@]host[:port], with IPv6 hosts in brackets when a port
// follows. The typed text becomes the name so the tab reads as typed.
func ParseDestination(text string) (profile.Connection, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return profile.Connection{}, errors.New("empty destination")
	}
	connection := profile.Connection{Name: text}

	rest := text
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		connection.User = rest[:at]
		rest = rest[at+1:]
	}

	portText := ""
	switch {
	case strings.HasPrefix(rest, "["):
		// [2001:db8::1]:2222
		closing := strings.Index(rest, "]")
		if closing < 0 {
			return profile.Connection{}, fmt.Errorf("unclosed bracket in %q", text)
		}
		connection.Host = rest[1:closing]
		portText = strings.TrimPrefix(rest[closing+1:], ":")
	case strings.Count(rest, ":") == 1:
		// host:2222. A bare IPv6 address has more than one colon and is left alone.
		connection.Host, portText, _ = strings.Cut(rest, ":")
	default:
		connection.Host = rest
	}

	if connection.Host == "" {
		return profile.Connection{}, fmt.Errorf("no host in %q", text)
	}
	if portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return profile.Connection{}, fmt.Errorf("bad port %q", portText)
		}
		connection.Port = port
	}
	return connection, nil
}
