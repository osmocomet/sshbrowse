package sshcmd

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sshbrowse/internal/profile"
)

func TestSavedJumpArgv(t *testing.T) {
	hop := profile.Connection{ID: "jump", Host: "bastion-alias", User: "jump-user", Port: 2222, IdentityFile: "jump-key", AgentForwarding: true, X11Forwarding: true, LocalForwards: []string{"8080:localhost:80"}}
	target := profile.Connection{ID: "target", Host: "target.invalid", User: "target-user", IdentityFile: "target-key", JumpConnectionID: hop.ID}
	for _, sftp := range []bool{false, true} {
		argv, err := ConnectionArgv(target, []profile.Connection{hop}, sftp)
		if err != nil {
			t.Fatal(err)
		}
		var proxy string
		for _, arg := range argv {
			if strings.HasPrefix(arg, "ProxyCommand=") {
				proxy = arg
			}
		}
		if proxy == "" {
			t.Fatal("missing saved jump proxy")
		}
		want := quoteProxyCommand([]string{SSHPath, "-p", "2222", "-i", "jump-key", "-W", "[%h]:%p", "--", "jump-user@bastion-alias"})
		if proxy != "ProxyCommand="+want {
			t.Fatalf("proxy: %s; want %s", proxy, want)
		}
		if strings.Contains(proxy, "target-key") || strings.Contains(proxy, "-A") {
			t.Fatalf("hop inherited session settings: %s", proxy)
		}
		if argv[len(argv)-1] != "target-user@target.invalid" {
			t.Fatal(argv)
		}
	}
}

func TestSavedJumpReferences(t *testing.T) {
	a := profile.Connection{ID: "a", Host: "a.invalid", JumpConnectionID: "b"}
	b := profile.Connection{ID: "b", Host: "b.invalid"}
	if _, err := ConnectionArgv(a, []profile.Connection{b}, false); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		target profile.Connection
		saved  []profile.Connection
	}{
		{"deleted", a, nil},
		{"self", profile.Connection{ID: "a", Host: "a.invalid", JumpConnectionID: "a"}, []profile.Connection{a}},
		{"cycle", a, []profile.Connection{a, {ID: "b", Host: "b.invalid", JumpConnectionID: "a"}}},
		{"both modes", profile.Connection{Host: "target.invalid", JumpHost: "raw", JumpConnectionID: "b"}, []profile.Connection{b}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ConnectionArgv(tc.target, tc.saved, false); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	b.JumpConnectionID = "c"
	c := profile.Connection{ID: "c", Host: "c.invalid", JumpHost: "raw-jump"}
	if _, err := ConnectionArgv(a, []profile.Connection{b, c}, false); err != nil {
		t.Fatal(err)
	}
}

func TestRawJumpUnchanged(t *testing.T) {
	c := profile.Connection{Host: "target.invalid", JumpHost: "user@bastion.invalid:2222,alias"}
	for _, sftp := range []bool{false, true} {
		got, err := ConnectionArgv(c, nil, sftp)
		if err != nil {
			t.Fatal(err)
		}
		want := Argv(c)
		if sftp {
			want = SFTPArgv(c)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %q want %q", got, want)
		}
	}
}

func TestSavedJumpAgentOnly(t *testing.T) {
	target := profile.Connection{Host: "target.invalid", JumpConnectionID: "agent-hop", AgentForwarding: true}
	hop := profile.Connection{ID: "agent-hop", Host: "agent-alias"}
	argv, err := ConnectionArgv(target, []profile.Connection{hop}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := quoteProxyCommand([]string{SSHPath, "-W", "[%h]:%p", "--", "agent-alias"})
	found := false
	for _, arg := range argv {
		if arg == "ProxyCommand="+want {
			found = true
		}
	}
	if !found {
		t.Fatalf("agent-only hop should use OpenSSH defaults: %q", argv)
	}
}

func TestSavedJumpBoundsAndUnsafeTarget(t *testing.T) {
	for _, host := range []string{"target'$(touch bad)'", "target\n.invalid", "target;bad"} {
		if _, err := ConnectionArgv(profile.Connection{Host: host, JumpConnectionID: "hop"}, []profile.Connection{{ID: "hop", Host: "hop.invalid"}}, false); err == nil {
			t.Fatalf("unsafe target accepted: %q", host)
		}
	}
	saved := make([]profile.Connection, profile.MaxJumpHops+1)
	for i := range saved {
		saved[i] = profile.Connection{ID: fmt.Sprint(i), Host: "hop.invalid"}
		if i < len(saved)-1 {
			saved[i].JumpConnectionID = fmt.Sprint(i + 1)
		}
	}
	if _, err := ConnectionArgv(profile.Connection{Host: "target.invalid", JumpConnectionID: "0"}, saved, false); err == nil {
		t.Fatal("unbounded chain accepted")
	}
	saved = []profile.Connection{{ID: "hop", Host: "hop.invalid", IdentityFile: strings.Repeat("x", maxProxyCommandBytes)}}
	if _, err := ConnectionArgv(profile.Connection{Host: "target.invalid", JumpConnectionID: "hop"}, saved, false); err == nil {
		t.Fatal("oversized proxy accepted")
	}
}

func TestSavedJumpBracketedIPv6(t *testing.T) {
	hop := profile.Connection{ID: "hop", Host: "[2001:db8::2]", JumpConnectionID: "inner"}
	inner := profile.Connection{ID: "inner", Host: "inner.invalid"}
	target := profile.Connection{Host: "[2001:db8::1]", JumpConnectionID: "hop"}
	for _, sftp := range []bool{false, true} {
		got, err := ConnectionArgv(target, []profile.Connection{hop, inner}, sftp)
		if err != nil {
			t.Fatal(err)
		}
		want := "2001:db8::1"
		if sftp {
			want = "[2001:db8::1]"
		}
		if got[len(got)-1] != want {
			t.Fatalf("IPv6 destination: %q", got)
		}
		for _, arg := range got {
			if strings.HasPrefix(arg, "ProxyCommand=") && strings.Contains(arg, "[2001:db8::2]") {
				t.Fatalf("nested hop retained brackets: %s", arg)
			}
		}
	}
}

func TestSavedJumpAllowsOpenSSHAliases(t *testing.T) {
	for _, host := range []string{"env+target", "prod/eu", "lab=leaf", "café"} {
		target := profile.Connection{Host: host, JumpConnectionID: "hop"}
		hop := profile.Connection{ID: "hop", Host: host, JumpConnectionID: "inner"}
		inner := profile.Connection{ID: "inner", Host: "inner.invalid"}
		for _, sftp := range []bool{false, true} {
			if _, err := ConnectionArgv(target, []profile.Connection{hop, inner}, sftp); err != nil {
				t.Fatalf("valid alias %q rejected (SFTP %t): %v", host, sftp, err)
			}
		}
	}
}

func TestSavedJumpDisablesFDPassing(t *testing.T) {
	hop := profile.Connection{ID: "hop", Host: "hop.invalid", JumpConnectionID: "inner"}
	inner := profile.Connection{ID: "inner", Host: "inner.invalid"}
	for _, sftp := range []bool{false, true} {
		argv, err := ConnectionArgv(profile.Connection{Host: "target.invalid", JumpConnectionID: "hop"}, []profile.Connection{hop, inner}, sftp)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(argv, "ProxyUseFdpass=no") {
			t.Fatalf("outer proxy permits FD passing: %q", argv)
		}
		proxy := ""
		for _, arg := range argv {
			if strings.HasPrefix(arg, "ProxyCommand=") {
				proxy = arg
			}
		}
		if !strings.Contains(proxy, "ProxyUseFdpass=no") {
			t.Fatalf("nested proxy permits FD passing: %q", proxy)
		}
	}
}
