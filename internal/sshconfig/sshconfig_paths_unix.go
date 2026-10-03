//go:build darwin || linux

package sshconfig

var systemConfigPath = "/etc/ssh/ssh_config"

func systemConfigPaths() []string {
	return []string{systemConfigPath}
}

func sameConfigPath(left, right string) bool {
	return left == right
}
