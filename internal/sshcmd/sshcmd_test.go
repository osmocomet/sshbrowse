package sshcmd

import (
	"reflect"
	"slices"
	"testing"

	"sshbrowse/internal/profile"
)

// Each case is one saved connection and the exact ssh command it must produce.
func TestArgv(t *testing.T) {
	cases := []struct {
		name       string
		connection profile.Connection
		want       []string
	}{
		{
			name:       "host only",
			connection: profile.Connection{Host: "lab"},
			want:       []string{SSHPath, "--", "lab"},
		},
		{
			name:       "user and port",
			connection: profile.Connection{Host: "192.0.2.5", User: "user", Port: 2222},
			want:       []string{SSHPath, "-p", "2222", "--", "user@192.0.2.5"},
		},
		{
			name: "everything",
			connection: profile.Connection{
				Host:            "worker1",
				User:            "core",
				IdentityFile:    "~/.ssh/lab",
				JumpHost:        "bastion",
				AgentForwarding: true,
				X11Forwarding:   true,
				LocalForwards:   []string{"8080:localhost:80", "9090:localhost:9090"},
				RemoteForwards:  []string{"2200:localhost:22"},
				DynamicForwards: []string{"1080"},
			},
			want: []string{
				SSHPath,
				"-i", "~/.ssh/lab",
				"-J", "bastion",
				"-A", "-X",
				"-L", "8080:localhost:80",
				"-L", "9090:localhost:9090",
				"-R", "2200:localhost:22",
				"-D", "1080",
				"--", "core@worker1",
			},
		},
		{
			name:       "host starting with a dash stays a host",
			connection: profile.Connection{Host: "-oProxyCommand=evil"},
			want:       []string{SSHPath, "--", "-oProxyCommand=evil"},
		},
		{
			name:       "imported alias runs as plain ssh <alias>",
			connection: profile.Connection{Host: "leaf-1", Provenance: &profile.Provenance{SSHConfigAlias: "leaf-1"}},
			want:       []string{SSHPath, "--", "leaf-1"},
		},
	}

	for _, testCase := range cases {
		got := Argv(testCase.connection)
		if !slices.Equal(got, testCase.want) {
			t.Errorf("%s:\n got  %q\n want %q", testCase.name, got, testCase.want)
		}
	}
}

// sftp gets the connection flags it understands and none of the ssh-only ones.
func TestSFTPArgv(t *testing.T) {
	cases := []struct {
		name       string
		connection profile.Connection
		want       []string
	}{
		{
			name: "supported options",
			connection: profile.Connection{
				Host:            "worker1",
				User:            "core",
				Port:            2222,
				IdentityFile:    "~/.ssh/lab",
				JumpHost:        "bastion",
				AgentForwarding: true,
				X11Forwarding:   true,
				LocalForwards:   []string{"8080:localhost:80"},
				DynamicForwards: []string{"1080"},
			},
			want: []string{SFTPPath, "-P", "2222", "-i", "~/.ssh/lab", "-J", "bastion", "-A", "--", "core@worker1"},
		},
		{
			name:       "IPv6 host",
			connection: profile.Connection{Host: "2001:db8::1"},
			want:       []string{SFTPPath, "--", "[2001:db8::1]"},
		},
		{
			name:       "IPv6 host with user",
			connection: profile.Connection{Host: "2001:db8::1", User: "root"},
			want:       []string{SFTPPath, "--", "root@[2001:db8::1]"},
		},
		{
			name:       "already bracketed IPv6 host",
			connection: profile.Connection{Host: "[2001:db8::1]"},
			want:       []string{SFTPPath, "--", "[2001:db8::1]"},
		},
		{
			name:       "host starting with a dash stays a host",
			connection: profile.Connection{Host: "-oProxyCommand=evil"},
			want:       []string{SFTPPath, "--", "-oProxyCommand=evil"},
		},
	}

	for _, testCase := range cases {
		if got := SFTPArgv(testCase.connection); !slices.Equal(got, testCase.want) {
			t.Errorf("%s:\n got  %q\n want %q", testCase.name, got, testCase.want)
		}
	}
}

// Each case is something typed into the picker and the connection it must parse to.
func TestParseDestination(t *testing.T) {
	cases := []struct {
		text string
		want profile.Connection
	}{
		{"lab", profile.Connection{Name: "lab", Host: "lab"}},
		{"  lab  ", profile.Connection{Name: "lab", Host: "lab"}},
		{"user@192.0.2.5", profile.Connection{Name: "user@192.0.2.5", Host: "192.0.2.5", User: "user"}},
		{"lab:2222", profile.Connection{Name: "lab:2222", Host: "lab", Port: 2222}},
		{"core@worker1:22", profile.Connection{Name: "core@worker1:22", Host: "worker1", User: "core", Port: 22}},
		{"2001:db8::1", profile.Connection{Name: "2001:db8::1", Host: "2001:db8::1"}},
		{"[2001:db8::1]:2222", profile.Connection{Name: "[2001:db8::1]:2222", Host: "2001:db8::1", Port: 2222}},
		{"root@[::1]", profile.Connection{Name: "root@[::1]", Host: "::1", User: "root"}},
	}
	for _, testCase := range cases {
		got, err := ParseDestination(testCase.text)
		if err != nil {
			t.Errorf("%q: unexpected error %v", testCase.text, err)
			continue
		}
		// DeepEqual because Connection holds slices, which == cannot compare.
		if !reflect.DeepEqual(got, testCase.want) {
			t.Errorf("%q:\n got  %+v\n want %+v", testCase.text, got, testCase.want)
		}
	}

	for _, bad := range []string{"", "   ", "lab:0", "lab:99999", "lab:abc", "[::1", "user@"} {
		if _, err := ParseDestination(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}
