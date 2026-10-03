package session

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDelayedDeliveryBackpressuresLargeDarwinOutput(t *testing.T) {
	firstChunk := make(chan struct{})
	release := make(chan struct{})
	exited := make(chan int, 1)
	var output bytes.Buffer
	running, err := Start(Options{
		Argv: []string{testShell(t), "-c", `yes | head -c 2097152; printf FINAL`},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			select {
			case <-firstChunk:
			default:
				close(firstChunk)
				<-release
			}
			_, _ = output.Write(chunk) // bytes.Buffer.Write cannot fail.
			return true
		},
		OnExit: func(code int) { exited <- code },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		_ = running.Close()
	}()
	waitForTestChannel(t, firstChunk, "first output chunk")
	time.Sleep(250 * time.Millisecond) // Let the bounded read-ahead queue fill.
	close(release)
	if code := waitForTestExit(t, exited); code != 0 {
		t.Fatalf("large output shell exit code = %d, want 0", code)
	}
	if output.Len() < 2*1024*1024 || !strings.Contains(output.String(), "FINAL") {
		t.Fatalf("large output was truncated: received %d bytes, final marker %v", output.Len(), strings.Contains(output.String(), "FINAL"))
	}
}

func TestObserveAlreadyExitedDarwinLeader(t *testing.T) {
	cmd := exec.Command(testShell(t), "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cmd.Wait(); err != nil {
			t.Errorf("reap shell: %v", err)
		}
	}()
	deadline := time.After(sessionTestTimeout)
	for {
		zombie, err := processIsZombie(cmd.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		if zombie {
			break
		}
		select {
		case <-deadline:
			t.Fatal("shell did not become an unreaped zombie")
		case <-time.After(10 * time.Millisecond):
		}
	}

	done := make(chan error, 1)
	go func() { done <- observeProcessExit(cmd.Process.Pid) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("observe already-exited leader: %v", err)
		}
	case <-time.After(sessionTestTimeout):
		t.Fatal("kqueue missed an exit that preceded registration")
	}
}

func TestNaturalExitStopsHUPResistantPTYChild(t *testing.T) {
	shell := testShell(t)
	childReady := filepath.Join(t.TempDir(), "child-ready")
	data := make(chan []byte, 16)
	exited := make(chan int, 1)
	running, err := startWithDrainLimit(Options{
		Argv: []string{shell, "-c", `(trap '' HUP; : > "$1"; exec sleep 8) & child=$!; while [ ! -e "$1" ]; do sleep 0.01; done; printf 'CHILD:%s\nFINAL\n' "$child"; exit 0`, "sshbrowse-session-test", childReady},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			data <- append([]byte(nil), chunk...)
			return true
		},
		OnExit: func(code int) { exited <- code },
	}, 250*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()

	output := waitForOutput(t, data, "", "FINAL", sessionTestTimeout, nil)
	childPID, err := childPIDFromOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if code := waitForTestExit(t, exited); code != 0 {
		t.Fatalf("parent exit code = %d, want 0", code)
	}
	if !strings.Contains(output, "FINAL") {
		t.Fatalf("final PTY output was not delivered: %q", output)
	}
	waitForProcessExit(t, childPID, sessionTestTimeout)
	assertTestSessionDrained(t, running)
}
