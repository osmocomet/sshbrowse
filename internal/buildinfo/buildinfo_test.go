package buildinfo

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestDescription(t *testing.T) {
	fullCommit := "7029a7e1c9a178e5d7c4b31a2f6e0d9c8b7a6543"
	tests := []struct {
		name      string
		version   string
		commit    string
		buildInfo *debug.BuildInfo
		want      string
	}{
		{
			name:    "release full commit",
			version: "0.1.0",
			commit:  fullCommit,
			want:    "SSH session manager\nVersion 0.1.0\nCommit 7029a7e",
		},
		{
			name:    "release short commit",
			version: "0.1.0",
			commit:  "abc1234",
			want:    "SSH session manager\nVersion 0.1.0\nCommit abc1234",
		},
		{
			name:    "commit longer than seven characters",
			version: "0.1.0",
			commit:  "abc123456789",
			want:    "SSH session manager\nVersion 0.1.0\nCommit abc1234",
		},
		{
			name:    "missing commit",
			version: defaultVersion,
			want:    "SSH session manager\nVersion 0.0.0-dev",
		},
		{
			name:    "development fallback",
			version: defaultVersion,
			buildInfo: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: fullCommit},
				{Key: "vcs.modified", Value: "true"},
			}},
			want: "SSH session manager\nVersion 0.0.0-dev\nCommit 7029a7e (modified)",
		},
		{
			name:    "release commit takes precedence over fallback",
			version: "0.1.0",
			commit:  "abc1234",
			buildInfo: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: fullCommit},
				{Key: "vcs.modified", Value: "true"},
			}},
			want: "SSH session manager\nVersion 0.1.0\nCommit abc1234",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := description(test.version, test.commit, test.buildInfo); got != test.want {
				t.Fatalf("description() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInstanceIDUsesExecutableLocation(t *testing.T) {
	installed := InstanceID("/home/user/.local/bin/sshbrowse")
	development := InstanceID("/work/sshbrowse/bin/sshbrowse")
	if installed == development {
		t.Fatal("installed and development paths share a single-instance ID")
	}
	// D-Bus requires the final name segment to begin with a letter.
	if !strings.HasPrefix(installed, "local.sshbrowse.path.h") {
		t.Fatalf("instance ID = %q", installed)
	}
	if installed != InstanceID("/home/user/.local/bin/sshbrowse") {
		t.Fatal("instance ID is not stable")
	}
}
