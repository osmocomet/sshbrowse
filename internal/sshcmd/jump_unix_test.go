//go:build darwin || linux

package sshcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshbrowse/internal/profile"
)

// Execute the actual OpenSSH ProxyCommand path with a hop that only runs ssh -G.
// No endpoint is contacted. This checks shell/token quoting, alias resolution,
// and local agent inheritance even with ForwardAgent disabled.
func TestSavedJumpOpenSSHConfigAndAgent(t *testing.T) {
	realSSH := SSHPath
	if _, err := os.Stat(realSSH); err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	output := filepath.Join(dir, "hop-config")
	agentOutput := filepath.Join(dir, "agent")
	argvOutput := filepath.Join(dir, "argv")
	helper := filepath.Join(dir, "hop ssh")
	identity := filepath.Join(dir, "key's %d $(touch should-not-exist)")
	if err := os.WriteFile(identity, []byte("test identity"), 0600); err != nil {
		t.Fatal(err)
	}
	err := os.WriteFile(config, []byte("Host bastion-alias\n HostName bastion.invalid\n User config-user\n Port 2200\n IdentityAgent /tmp/config-agent.sock\n IdentitiesOnly yes\n ForwardAgent no\n ProxyUseFdpass yes\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s' \"$SSH_AUTH_SOCK\" > " + quoteProxyCommand([]string{agentOutput}) + "\n" +
		"printf '%s\n' \"$@\" > " + quoteProxyCommand([]string{argvOutput}) + "\n" +
		quoteProxyCommand([]string{realSSH, "-G", "-F", config}) + " \"$@\" > " + quoteProxyCommand([]string{output}) + "\nexit 1\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	SSHPath = helper
	defer func() { SSHPath = realSSH }()
	hop := profile.Connection{ID: "jump", Host: "bastion-alias", User: "saved-user", Port: 2222, IdentityFile: identity}
	argv, err := ConnectionArgv(profile.Connection{Host: "[2001:db8::1]", JumpConnectionID: "jump"}, []profile.Connection{hop}, false)
	if err != nil {
		t.Fatal(err)
	}
	argv[0] = realSSH
	argv = insertOptions(argv, "-F", "/dev/null", "-o", "BatchMode=yes", "-o", "ConnectTimeout=2")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "SSH_AUTH_SOCK=/tmp/local-test-agent.sock")
	if data, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("expected closed proxy; output %s", data)
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"hostname bastion.invalid", "user saved-user", "port 2222", "identityfile " + identity, "identityagent /tmp/config-agent.sock", "identitiesonly yes", "forwardagent no"} {
		if !strings.Contains(string(data), line+"\n") {
			t.Errorf("missing %q in hop config", line)
		}
	}
	hopArgs, err := os.ReadFile(argvOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hopArgs), "-W\n[2001:db8::1]:22\n") {
		t.Fatalf("IPv6 -W target was not normalized: %s", hopArgs)
	}
	agent, err := os.ReadFile(agentOutput)
	if err != nil {
		t.Fatal(err)
	}
	if string(agent) != "/tmp/local-test-agent.sock" {
		t.Fatalf("hop lost local agent: %q", agent)
	}
	// The outer client must leave a nested hop's %h/%p tokens intact until
	// that hop executes its own ProxyCommand.
	hop.JumpConnectionID = "inner"
	inner := profile.Connection{ID: "inner", Host: "inner.invalid", IdentityFile: "inner%d-key"}

	for _, sftp := range []bool{false, true} {
		if sftp {
			if _, err := os.Stat(SFTPPath); err != nil {
				t.Log("SFTP client unavailable:", err)
				continue
			}
		}
		argv, err = ConnectionArgv(profile.Connection{Host: "[2001:db8::1]", JumpConnectionID: "jump"}, []profile.Connection{hop, inner}, sftp)
		if err != nil {
			t.Fatal(err)
		}
		if !sftp {
			argv[0] = realSSH
		}
		argv = insertOptions(argv, "-F", "/dev/null", "-o", "BatchMode=yes", "-o", "ConnectTimeout=2")
		if sftp {
			argv = insertOptions(argv, "-S", realSSH)
		}
		for _, path := range []string{output, argvOutput} {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
		if data, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("expected closed nested proxy; output %s", data)
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		data, err = os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		nested, err := savedJumpProxy(hop.Host, []profile.Connection{inner})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "proxyusefdpass no\n") {
			t.Fatalf("nested proxy inherited FD passing: %s", data)
		}
		if !strings.Contains(string(data), "proxycommand "+nested+"\n") {
			t.Fatalf("nested proxy tokens or shell quoting changed (SFTP %t): %s", sftp, data)
		}
		hopArgs, err = os.ReadFile(argvOutput)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(hopArgs), "-W\n[2001:db8::1]:22\n") {
			t.Fatalf("IPv6 -W target was not normalized (SFTP %t): %s", sftp, hopArgs)
		}
	}

}
