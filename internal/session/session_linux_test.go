package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestNaturalExitDeadlineStopsSurvivingPTYWriter(t *testing.T) {
	shell := testShell(t)
	exited := make(chan int, 1)
	running, slave := startDrainTestSession(t, []string{shell, "-c", "printf READY"},
		250*time.Millisecond, func([]byte) bool { return true }, nil,
		func(code int) { exited <- code })

	stopWriter := make(chan struct{})
	writerDone := make(chan struct{})
	var writes atomic.Int32
	go func() {
		defer close(writerDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopWriter:
				return
			case <-ticker.C:
				if _, err := slave.Write([]byte("tick")); err != nil {
					return
				}
				writes.Add(1)
			}
		}
	}()
	t.Cleanup(func() {
		close(stopWriter)
		<-writerDone
	})

	waitForParentExit(t, running)
	writesAtExit := writes.Load()
	if code := waitForTestExit(t, exited); code != 0 {
		t.Fatalf("shell exit code = %d, want 0", code)
	}
	if writes.Load()-writesAtExit < 2 {
		t.Fatalf("writer made %d writes after shell exit, want repeated output across the drain", writes.Load()-writesAtExit)
	}
	assertTestSessionDrained(t, running)
}

func TestNaturalExitStopsChildHoldingPTY(t *testing.T) {
	shell := testShell(t)
	childReady := filepath.Join(t.TempDir(), "child-ready")
	data := make(chan []byte, 16)
	exited := make(chan int, 1)
	running, slave := startDrainTestSession(t,
		[]string{shell, "-c", `printf 'READY\n'; read go; (trap '' HUP; printf 'CHILD_READY\n'; : > "$1"; sleep 8) & child=$!; printf 'CHILD:%s\n' "$child"; while [ ! -e "$1" ]; do sleep 0.01; done`, "sshbrowse-session-test", childReady},
		100*time.Millisecond, func(chunk []byte) bool {
			data <- append([]byte(nil), chunk...)
			return true
		}, nil, func(code int) { exited <- code })
	if err := slave.Close(); err != nil {
		t.Fatal(err)
	}
	output := waitForOutput(t, data, "", "READY", sessionTestTimeout, nil)
	if err := running.Write([]byte("go\n")); err != nil {
		t.Fatal(err)
	}
	output = waitForOutput(t, data, output, "CHILD_READY", sessionTestTimeout, nil)
	output = waitForOutput(t, data, output, "CHILD:", sessionTestTimeout, nil)
	childPID, err := childPIDFromOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if code := waitForTestExit(t, exited); code != 0 {
		t.Fatalf("parent exit code = %d, want 0", code)
	}
	deadline := time.NewTimer(sessionTestTimeout)
	defer deadline.Stop()
	for {
		state, err := linuxProcessState(childPID)
		if errors.Is(err, os.ErrNotExist) || state == 'Z' || state == 'X' {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-deadline.C:
			t.Fatalf("PTY child %d remains in state %c after session exit", childPID, state)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func linuxProcessState(pid int) (byte, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	end := strings.LastIndex(string(data), ") ")
	if end < 0 || end+2 >= len(data) {
		return 0, fmt.Errorf("malformed process state for %d", pid)
	}
	return data[end+2], nil
}

func TestNaturalExitKillsHUPResistantRedirectedPTYChild(t *testing.T) {
	shell := testShell(t)
	childReady := filepath.Join(t.TempDir(), "child-ready")
	data := make(chan []byte, 16)
	exited := make(chan int, 1)
	running, slave := startDrainTestSession(t,
		[]string{shell, "-c", `(trap '' HUP; exec </dev/null >/dev/null 2>&1; : > "$1"; exec sleep 30) & child=$!; while [ ! -e "$1" ]; do sleep 0.01; done; printf 'CHILD:%s\nREADY\n' "$child"; read finish; printf 'FINAL\n'`, "sshbrowse-session-test", childReady},
		5*time.Second, func(chunk []byte) bool {
			data <- append([]byte(nil), chunk...)
			return true
		}, nil, func(code int) { exited <- code })
	if err := slave.Close(); err != nil {
		t.Fatal(err)
	}

	output := waitForOutput(t, data, "", "READY", sessionTestTimeout, nil)
	childPID, err := childPIDFromOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	childGroup, err := linuxProcessGroup(childPID)
	if err != nil {
		t.Fatalf("read child process group: %v", err)
	}
	if childGroup != running.cmd.Process.Pid {
		t.Fatalf("child process group = %d, want SSHBrowse-owned group %d", childGroup, running.cmd.Process.Pid)
	}
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })

	if err := running.Write([]byte("done\n")); err != nil {
		t.Fatal(err)
	}
	waitForParentExit(t, running)
	select {
	case <-running.readDone:
	case <-time.After(2 * time.Second):
		t.Fatal("PTY output did not drain after the redirected child's shell exited")
	}
	if code := waitForTestExit(t, exited); code != 0 {
		t.Fatalf("shell exit code = %d, want 0", code)
	}
	deadline := time.NewTimer(sessionTestTimeout)
	defer deadline.Stop()
	for {
		state, err := linuxProcessState(childPID)
		if errors.Is(err, os.ErrNotExist) || state == 'Z' || state == 'X' {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-deadline.C:
			t.Fatalf("HUP-resistant child %d remains in state %c after drained session exit", childPID, state)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func linuxProcessGroup(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	_, processGroup, err := parseLinuxProcessStat(data)
	return processGroup, err
}
