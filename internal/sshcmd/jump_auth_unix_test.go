//go:build darwin || linux

package sshcmd

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"sshbrowse/internal/profile"
)

// This opt-in test runs the generated saved-jump ProxyCommand through real
// OpenSSH clients and local sshd instances. It uses only temporary identities,
// configuration, agent sockets, and loopback listeners.
func TestSavedJumpAgentAuthentication(t *testing.T) {
	if os.Getenv("SSHBROWSE_SSH_INTEGRATION") != "1" {
		t.Skip("set SSHBROWSE_SSH_INTEGRATION=1 to run the local OpenSSH integration test")
	}

	sshdPath := integrationBinary(t, "sshd")
	sshAgentPath := integrationBinary(t, "ssh-agent")
	sshAddPath := integrationBinary(t, "ssh-add")
	sshKeygenPath := integrationBinary(t, "ssh-keygen")
	realSSHPath := SSHPath
	if realSSHPath == "" || SFTPPath == "" {
		t.Fatal("OpenSSH client paths are empty")
	}
	if _, err := os.Stat(realSSHPath); err != nil {
		t.Fatalf("SSHPath is unavailable: %v", err)
	}
	if _, err := os.Stat(SFTPPath); err != nil {
		t.Fatalf("SFTPPath is unavailable: %v", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("resolve current account: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	var processes []*integrationProcess
	defer func() {
		for _, process := range processes {
			process.wait(t)
		}
		cancel()
	}()

	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	sshHome := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshHome, 0700); err != nil {
		t.Fatal(err)
	}

	jumpPrivate, jumpPublic := generateIntegrationKey(t, ctx, sshKeygenPath, root, "jump")
	targetPrivate, targetPublic := generateIntegrationKey(t, ctx, sshKeygenPath, root, "target")
	hostPrivate, hostPublic := generateIntegrationKey(t, ctx, sshKeygenPath, root, "host")

	jumpAuthorizedKeys := filepath.Join(root, "jump_authorized_keys")
	targetAuthorizedKeys := filepath.Join(root, "target_authorized_keys")
	copyIntegrationFile(t, jumpPublic, jumpAuthorizedKeys, 0600)
	copyIntegrationFile(t, targetPublic, targetAuthorizedKeys, 0600)

	jumpPort, targetPort := integrationPorts(t)
	knownHosts := filepath.Join(root, "known_hosts")
	knownHostLine := integrationKnownHost(t, hostPublic)
	knownHostData := fmt.Sprintf("[127.0.0.1]:%d %s\n[127.0.0.1]:%d %s\n", jumpPort, knownHostLine, targetPort, knownHostLine)
	if err := os.WriteFile(knownHosts, []byte(knownHostData), 0600); err != nil {
		t.Fatal(err)
	}
	sshConfig := strings.Join([]string{
		"Host env+target",
		"  HostName 127.0.0.1",
		"  ProxyUseFdpass yes",
		"",
		"Host *",
		"  User nonexistent-sshbrowse-user",
		"  ProxyUseFdpass yes",
		"  IdentitiesOnly yes",
		"  BatchMode yes",
		"  ConnectTimeout 5",
		"  StrictHostKeyChecking yes",
		"  UserKnownHostsFile " + knownHosts,
		"  UpdateHostKeys no",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(sshHome, "config"), []byte(sshConfig), 0600); err != nil {
		t.Fatal(err)
	}
	sshWrapper := filepath.Join(root, "ssh-with-fixture-config")
	wrapperScript := "#!/bin/sh\nexec " + quoteProxyCommand([]string{realSSHPath, "-F", filepath.Join(sshHome, "config")}) + " \"$@\"\n"
	if err := os.WriteFile(sshWrapper, []byte(wrapperScript), 0700); err != nil {
		t.Fatal(err)
	}
	SSHPath = sshWrapper
	defer func() { SSHPath = realSSHPath }()

	socketDirectory, err := os.MkdirTemp("/tmp", "sb-")
	if err != nil {
		t.Fatalf("create short-lived agent socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDirectory) })
	jumpSocket := filepath.Join(socketDirectory, "a.sock")
	agentProcess := startIntegrationProcess(t, &processes, sshAgentPath, "-D", "-a", jumpSocket)
	if err := waitForAgent(ctx, sshAddPath, jumpSocket, agentProcess); err != nil {
		t.Fatalf("wait for isolated ssh-agent: %v", err)
	}
	for _, privateKey := range []string{jumpPrivate, targetPrivate} {
		if output, err := runIntegrationCommand(ctx, isolatedSSHEnv(home, jumpSocket), sshAddPath, "-t", "120", privateKey); err != nil {
			t.Fatalf("load an ephemeral key into ssh-agent: %v (%s)", err, safeCommandOutput(output))
		}
	}
	// Keep only the public identities on disk. Successful authentication below
	// therefore requires the private keys held by the local agent.
	for _, privateKey := range []string{jumpPrivate, targetPrivate} {
		if err := os.Remove(privateKey); err != nil {
			t.Fatalf("remove temporary client private key: %v", err)
		}
	}

	jumpConfig := integrationSSHDConfig(root, "jump", jumpPort, hostPrivate, jumpAuthorizedKeys, currentUser.Username, targetPort)
	targetConfig := integrationSSHDConfig(root, "target", targetPort, hostPrivate, targetAuthorizedKeys, currentUser.Username, 0)
	var jumpServer, targetServer *integrationProcess
	for _, fixture := range []struct {
		name   string
		config string
	}{
		{name: "jump", config: jumpConfig},
		{name: "target", config: targetConfig},
	} {
		name, config := fixture.name, fixture.config
		configPath := filepath.Join(root, name+"_sshd_config")
		if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		if output, err := runIntegrationCommand(ctx, nil, sshdPath, "-t", "-f", configPath); err != nil {
			t.Fatalf("validate temporary %s sshd config: %v (%s)", name, err, safeCommandOutput(output))
		}
		server := startIntegrationProcess(t, &processes, sshdPath, "-D", "-e", "-f", configPath)
		if name == "jump" {
			jumpServer = server
		} else {
			targetServer = server
		}
	}
	if err := waitForListener(ctx, jumpPort, jumpServer); err != nil {
		t.Fatalf("wait for temporary jump sshd: %v", err)
	}
	if err := waitForListener(ctx, targetPort, targetServer); err != nil {
		t.Fatalf("wait for temporary target sshd: %v", err)
	}

	jump := profile.Connection{
		ID:           "jump",
		Host:         "127.0.0.1",
		User:         currentUser.Username,
		Port:         jumpPort,
		IdentityFile: jumpPublic,
	}
	target := profile.Connection{
		ID:               "target",
		Host:             "env+target",
		User:             currentUser.Username,
		Port:             targetPort,
		IdentityFile:     targetPublic,
		JumpConnectionID: jump.ID,
		AgentForwarding:  false,
	}

	sshArgv, err := ConnectionArgv(target, []profile.Connection{jump}, false)
	if err != nil {
		t.Fatalf("build SSH argv: %v", err)
	}
	sshArgv = insertOptions(sshArgv,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile="+knownHosts,
		"-o", "UpdateHostKeys=no",
	)
	sshArgv = append(sshArgv, "printf sshbrowse-agent-authenticated")
	sshOutput, err := runIntegrationCommand(ctx, isolatedSSHEnv(home, jumpSocket), sshArgv[0], sshArgv[1:]...)
	if err != nil {
		t.Fatalf("SSH through saved jump failed: %v (%s)", err, safeCommandOutput(sshOutput))
	}
	if !bytes.Contains(sshOutput, []byte("sshbrowse-agent-authenticated")) {
		t.Fatalf("SSH through saved jump did not run the remote command: %s", safeCommandOutput(sshOutput))
	}

	sftpArgv, err := ConnectionArgv(target, []profile.Connection{jump}, true)
	if err != nil {
		t.Fatalf("build SFTP argv: %v", err)
	}
	sftpArgv = insertOptions(sftpArgv,
		"-S", SSHPath,
		"-F", filepath.Join(sshHome, "config"),
		"-b", "-",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile="+knownHosts,
		"-o", "UpdateHostKeys=no",
	)
	sftpOutput, err := runIntegrationCommandWithInput(ctx, isolatedSSHEnv(home, jumpSocket), strings.NewReader("pwd\n"), sftpArgv[0], sftpArgv[1:]...)
	if err != nil {
		t.Fatalf("SFTP through saved jump failed: %v (%s)", err, safeCommandOutput(sftpOutput))
	}
	if !bytes.Contains(sftpOutput, []byte("Remote working directory:")) {
		t.Fatalf("SFTP through saved jump did not run the batch command: %s", safeCommandOutput(sftpOutput))
	}

}

type integrationProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	waitErr error
	stderr  *limitedBuffer
}

func integrationBinary(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("opted-in local OpenSSH integration dependency %q is unavailable: %v", name, err)
	}
	return path
}

func generateIntegrationKey(t *testing.T, ctx context.Context, keygen, directory, name string) (string, string) {
	t.Helper()
	privateKey := filepath.Join(directory, name+"_key")
	if output, err := runIntegrationCommand(ctx, nil, keygen, "-q", "-t", "ed25519", "-N", "", "-f", privateKey); err != nil {
		t.Fatalf("generate temporary %s key: %v (%s)", name, err, safeCommandOutput(output))
	}
	return privateKey, privateKey + ".pub"
}

func copyIntegrationFile(t *testing.T, source, destination string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, mode); err != nil {
		t.Fatal(err)
	}
}

func integrationPorts(t *testing.T) (int, int) {
	t.Helper()
	jumpListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback jump port: %v", err)
	}
	targetListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		_ = jumpListener.Close()
		t.Fatalf("reserve loopback target port: %v", err)
	}
	jumpPort := jumpListener.Addr().(*net.TCPAddr).Port
	targetPort := targetListener.Addr().(*net.TCPAddr).Port
	if err := jumpListener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := targetListener.Close(); err != nil {
		t.Fatal(err)
	}
	return jumpPort, targetPort
}

func integrationKnownHost(t *testing.T, publicKey string) string {
	t.Helper()
	data, err := os.ReadFile(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		t.Fatalf("temporary host public key is malformed")
	}
	return fields[0] + " " + fields[1]
}

func integrationSSHDConfig(directory, name string, port int, hostKey, authorizedKeys, username string, permitOpenPort int) string {
	lines := []string{
		"Port " + strconv.Itoa(port),
		"ListenAddress 127.0.0.1",
		"AddressFamily inet",
		"HostKey " + hostKey,
		"PidFile " + filepath.Join(directory, name+"_sshd.pid"),
		"AuthorizedKeysFile " + authorizedKeys,
		"PubkeyAuthentication yes",
		"AuthenticationMethods publickey",
		"PasswordAuthentication no",
		"KbdInteractiveAuthentication no",
		"UsePAM no",
		"PermitRootLogin yes",
		"AllowUsers " + username,
		"StrictModes no",
		"AllowTcpForwarding yes",
		"PermitTTY no",
		"X11Forwarding no",
		"PrintMotd no",
		"UseDNS no",
		"LogLevel ERROR",
		"Subsystem sftp internal-sftp",
	}
	if permitOpenPort != 0 {
		lines = append(lines, "PermitOpen 127.0.0.1:"+strconv.Itoa(permitOpenPort))
	}
	return strings.Join(lines, "\n") + "\n"
}

func startIntegrationProcess(t *testing.T, processes *[]*integrationProcess, executable string, args ...string) *integrationProcess {
	t.Helper()
	stderr := newLimitedBuffer()
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Bound pipe draining if a server child outlives its parent.
	cmd.WaitDelay = 2 * time.Second
	cmd.Stderr = stderr
	process := &integrationProcess{cmd: cmd, done: make(chan struct{}), stderr: stderr}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start local OpenSSH fixture process %q: %v", filepath.Base(executable), err)
	}
	*processes = append(*processes, process)
	go func() {
		process.waitErr = cmd.Wait()
		close(process.done)
	}()
	return process
}

func (p *integrationProcess) wait(t *testing.T) {
	t.Helper()
	if p.cmd.Process != nil {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		if p.cmd.Process != nil {
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		}
		select {
		case <-p.done:
		case <-time.After(3 * time.Second):
		}
		t.Errorf("local OpenSSH fixture process %q did not stop promptly", filepath.Base(p.cmd.Path))
	}
}

func waitForAgent(ctx context.Context, sshAdd, socket string, agent *integrationProcess) error {
	for {
		if err := agent.exited(); err != nil {
			return err
		}
		output, err := runIntegrationCommand(ctx, isolatedSSHEnv("", socket), sshAdd, "-l")
		if err == nil || bytes.Contains(output, []byte("The agent has no identities")) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func waitForListener(ctx context.Context, port int, process *integrationProcess) error {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for {
		if err := process.exited(); err != nil {
			return err
		}
		connection, err := net.DialTimeout("tcp4", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", address, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *integrationProcess) exited() error {
	select {
	case <-p.done:
		if p.waitErr != nil {
			return fmt.Errorf("fixture process %q exited: %w (%s)", filepath.Base(p.cmd.Path), p.waitErr, safeCommandOutput(p.stderr.Bytes()))
		}
		return fmt.Errorf("fixture process %q exited unexpectedly", filepath.Base(p.cmd.Path))
	default:
		return nil
	}
}

func isolatedSSHEnv(home, socket string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "SSH_AUTH_SOCK=") || strings.HasPrefix(entry, "SSH_AGENT_PID=") || strings.HasPrefix(entry, "HOME=") {
			continue
		}
		environment = append(environment, entry)
	}
	if home != "" {
		environment = append(environment, "HOME="+home)
	}
	if socket != "" {
		environment = append(environment, "SSH_AUTH_SOCK="+socket)
	}
	return environment
}

func runIntegrationCommand(ctx context.Context, environment []string, executable string, args ...string) ([]byte, error) {
	return runIntegrationCommandWithInput(ctx, environment, nil, executable, args...)
}

func runIntegrationCommandWithInput(ctx context.Context, environment []string, input *strings.Reader, executable string, args ...string) ([]byte, error) {
	output := newLimitedBuffer()
	cmd := exec.Command(executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Bound pipe draining if a server child outlives its parent.
	cmd.WaitDelay = 2 * time.Second
	if environment != nil {
		cmd.Env = environment
	}
	if input != nil {
		cmd.Stdin = input
	}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return output.Bytes(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return output.Bytes(), err
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
			}
		}
		return output.Bytes(), ctx.Err()
	}
}

func safeCommandOutput(output []byte) string {
	// Fixture commands should emit only status text. Cap it so unexpected output
	// cannot flood test logs or include arbitrarily large server diagnostics.
	const maxOutputBytes = 4 * 1024
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes]
	}
	return strings.TrimSpace(string(output))
}

type limitedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func newLimitedBuffer() *limitedBuffer {
	return &limitedBuffer{}
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	const maxOutputBytes = 4 * 1024
	inputLength := len(data)
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := maxOutputBytes - b.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = b.buffer.Write(data)
	}
	return inputLength, nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buffer.Bytes()...)
}
