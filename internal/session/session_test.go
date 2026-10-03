//go:build darwin || linux

package session

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

const sessionTestTimeout = 5 * time.Second

func TestInteractiveSessionInputResizeOutputAndExit(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required for the local PTY test")
	}

	data := make(chan []byte, 32)
	exited := make(chan int, 1)
	running, err := Start(Options{
		Argv: []string{shell, "-i"},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			data <- append([]byte(nil), chunk...)
			return true
		},
		OnExit: func(exitCode int) { exited <- exitCode },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()

	output := waitForOutput(t, data, "", "READY", sessionTestTimeout, func() {
		if err := running.Write([]byte("printf 'READY\\n'\n")); err != nil {
			t.Fatalf("write startup command: %v", err)
		}
	})
	if !strings.Contains(output, "READY") {
		t.Fatalf("startup output %q does not contain READY", output)
	}

	if err := running.Resize(100, 40); err != nil {
		t.Fatalf("resize PTY: %v", err)
	}
	if err := running.Write([]byte("printf 'INPUT:%s\\n' hello\nstty size\n")); err != nil {
		t.Fatalf("write interactive commands: %v", err)
	}
	output = waitForOutput(t, data, output, "40 100", sessionTestTimeout, nil)
	if !strings.Contains(output, "INPUT:hello") {
		t.Fatalf("interactive output %q does not contain INPUT:hello", output)
	}

	if err := running.Write([]byte("exit\n")); err != nil {
		t.Fatalf("write exit command: %v", err)
	}
	select {
	case exitCode := <-exited:
		if exitCode != 0 {
			t.Fatalf("shell exit code = %d, want 0", exitCode)
		}
	case <-time.After(sessionTestTimeout):
		t.Fatal("interactive shell did not exit")
	}
}

func TestCloseKillsRunningProcessGroupAndDrainsOutput(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required for the local PTY test")
	}

	childExitPath := filepath.Join(t.TempDir(), "child-exited")
	data := make(chan []byte, 8)
	exited := make(chan int, 1)
	running, err := Start(Options{
		Argv: []string{
			shell,
			"-c",
			`printf RUNNING; (trap 'printf CHILD_EXITED > "$1"; exit 0' HUP TERM; printf 'CHILD_READY\n'; while :; do sleep 1; done) & child=$!; printf 'CHILD:%s\n' "$child"; wait "$child"`,
			"sshbrowse-session-test",
			childExitPath,
		},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			data <- append([]byte(nil), chunk...)
			return true
		},
		OnExit: func(exitCode int) { exited <- exitCode },
	})
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = running.Close()
		}
	}()

	// The parent can report a PID before the child installs its signal trap.
	output := waitForOutput(t, data, "", "CHILD_READY", sessionTestTimeout, nil)
	output = waitForOutput(t, data, output, "CHILD:", sessionTestTimeout, nil)
	childPID, err := childPIDFromOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := running.Close(); err != nil {
		t.Fatalf("close running session: %v", err)
	}
	closed = true
	select {
	case <-exited:
	case <-time.After(sessionTestTimeout):
		t.Fatal("closed session did not deliver its exit callback")
	}
	if running.cmd.ProcessState == nil {
		t.Fatal("closed session process was not reaped")
	}
	waitForFileContent(t, childExitPath, "CHILD_EXITED", sessionTestTimeout)
	waitForProcessExit(t, childPID, sessionTestTimeout)
}

func TestSessionCanRestartAfterNaturalExit(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required for the local PTY test")
	}

	for _, marker := range []string{"FIRST", "SECOND"} {
		data := make(chan []byte, 4)
		exited := make(chan int, 1)
		running, err := Start(Options{
			Argv: []string{shell, "-c", "printf " + marker},
			Cols: 80,
			Rows: 24,
			OnData: func(chunk []byte) bool {
				data <- append([]byte(nil), chunk...)
				return true
			},
			OnExit: func(exitCode int) { exited <- exitCode },
		})
		if err != nil {
			t.Fatalf("start %s session: %v", marker, err)
		}
		waitForOutput(t, data, "", marker, sessionTestTimeout, nil)
		select {
		case exitCode := <-exited:
			if exitCode != 0 {
				t.Fatalf("%s session exit code = %d, want 0", marker, exitCode)
			}
		case <-time.After(sessionTestTimeout):
			t.Fatalf("%s session did not exit", marker)
		}
		if running.cmd.ProcessState == nil {
			t.Fatalf("%s session process was not reaped", marker)
		}
		if _, err := running.terminal.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("%s session PTY is still open: %v", marker, err)
		}
	}
}

func TestNaturalExitDrainsOutputAfterDelayedAcknowledgement(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required for the local PTY test")
	}

	firstChunk := make(chan struct{})
	release := make(chan struct{})
	chunks := make(chan []byte, 8)
	exited := make(chan int, 1)
	running, err := Start(Options{
		Argv: []string{shell, "-c", `printf 'FIRST\n'; read go; printf 'TRAILING\n'`},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			chunks <- append([]byte(nil), chunk...)
			select {
			case <-firstChunk:
			default:
				close(firstChunk)
				<-release
			}
			return true
		},
		OnExit: func(exitCode int) { exited <- exitCode },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	select {
	case <-firstChunk:
	case <-time.After(sessionTestTimeout):
		t.Fatal("session did not deliver its first output chunk")
	}
	if err := running.Write([]byte("go\n")); err != nil {
		t.Fatalf("release shell output: %v", err)
	}
	// Darwin waits for queued terminal output on process exit while OnData is
	// blocked; Linux exits with that output still buffered in the PTY.
	if runtime.GOOS == "linux" {
		waitForParentExit(t, running)
	}
	select {
	case <-exited:
		t.Fatal("session exited before delayed output was acknowledged")
	case <-time.After(2200 * time.Millisecond):
	}
	close(release)

	select {
	case exitCode := <-exited:
		if exitCode != 0 {
			t.Fatalf("shell exit code = %d, want 0", exitCode)
		}
	case <-time.After(sessionTestTimeout):
		t.Fatal("session did not drain trailing output")
	}
	var output strings.Builder
	chunkCount := len(chunks)
	for range chunkCount {
		output.Write(<-chunks)
	}
	if chunkCount < 2 {
		t.Fatalf("received %d output chunk(s), want trailing output in a later chunk; output %q", chunkCount, output.String())
	}
	if !strings.Contains(output.String(), "FIRST") || !strings.Contains(output.String(), "TRAILING") {
		t.Fatalf("output before exit = %q, want both chunks", output.String())
	}
}

func TestNaturalExitDeadlineCancelsPendingDelivery(t *testing.T) {
	shell := testShell(t)
	pending := make(chan struct{})
	released := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	exited := make(chan int, 1)
	running, _ := startDrainTestSession(t, []string{shell, "-c", "printf READY"},
		100*time.Millisecond, func([]byte) bool {
			close(pending)
			<-released
			return false
		}, release, func(code int) { exited <- code })
	t.Cleanup(release)

	waitForTestChannel(t, pending, "pending output delivery")
	waitForParentExit(t, running)
	if code := waitForTestExit(t, exited); code != 0 {
		t.Fatalf("shell exit code = %d, want 0", code)
	}
	waitForTestChannel(t, released, "delivery cancellation")
	assertTestSessionDrained(t, running)
}

func TestCloseCancelsPendingDeliveryPromptly(t *testing.T) {
	shell := testShell(t)
	pending := make(chan struct{})
	released := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	exited := make(chan int, 1)
	running, _ := startDrainTestSession(t, []string{shell, "-c", "printf READY; exec sleep 30"},
		drainTimeout, func([]byte) bool {
			close(pending)
			<-released
			return false
		}, release, func(code int) { exited <- code })
	t.Cleanup(release)

	waitForTestChannel(t, pending, "pending output delivery")
	closed := make(chan error, 1)
	go func() { closed <- running.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("close pending session: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not interrupt pending output promptly")
	}
	waitForTestChannel(t, released, "delivery cancellation")
	if code := waitForTestExit(t, exited); code != -1 {
		t.Fatalf("closed shell exit code = %d, want -1", code)
	}
	assertTestSessionDrained(t, running)
}

func testShell(t *testing.T) string {
	t.Helper()
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is required for the local PTY test")
	}
	return shell
}

// Keep the PTY slave open in the test process to model a writer that survives
// the command without leaving an orphaned child process behind.
func startDrainTestSession(t *testing.T, argv []string, limit time.Duration,
	onData func([]byte) bool, cancelData func(), onExit func(int)) (*Session, *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		t.Fatal(err)
	}
	running := &Session{
		cmd:        cmd,
		terminal:   master,
		terminalFD: int(master.Fd()),
		readStop:   make(chan struct{}),
		cancelData: cancelData,
		drainLimit: limit,
		readDone:   make(chan struct{}),
		reaped:     make(chan struct{}),
		exited:     make(chan struct{}),
	}
	go running.readLoop(onData)
	go running.waitForExit(onExit)
	t.Cleanup(func() {
		_ = running.Close()
		_ = slave.Close()
	})
	return running, slave
}

func waitForTestChannel(t *testing.T, done <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(sessionTestTimeout):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitForParentExit(t *testing.T, running *Session) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- observeProcessExit(running.cmd.Process.Pid) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("observe shell exit: %v", err)
		}
	case <-time.After(sessionTestTimeout):
		t.Fatal("timed out waiting for shell exit")
	}
}

func waitForTestExit(t *testing.T, exited <-chan int) int {
	t.Helper()
	select {
	case code := <-exited:
		return code
	case <-time.After(sessionTestTimeout):
		t.Fatal("timed out waiting for exit callback")
		return 0
	}
}

func assertTestSessionDrained(t *testing.T, running *Session) {
	t.Helper()
	select {
	case <-running.readDone:
	default:
		t.Fatal("reader is still running after exit callback")
	}
	if _, err := running.terminal.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("PTY is still open after exit callback: %v", err)
	}
}

func waitForOutput(t *testing.T, data <-chan []byte, initial, want string, timeout time.Duration, before func()) string {
	t.Helper()
	var output bytes.Buffer
	output.WriteString(initial)
	if before != nil {
		before()
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if strings.Contains(output.String(), want) {
			return output.String()
		}
		select {
		case chunk := <-data:
			output.Write(chunk)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %q in PTY output %q", want, output.String())
		}
	}
}

func childPIDFromOutput(output string) (int, error) {
	marker := "CHILD:"
	index := strings.Index(output, marker)
	if index < 0 {
		return 0, errors.New("child PID marker is missing")
	}
	fields := strings.Fields(output[index+len(marker):])
	if len(fields) == 0 {
		return 0, errors.New("child PID is missing")
	}
	return strconv.Atoi(fields[0])
}

func waitForFileContent(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		content, err := os.ReadFile(path)
		if err == nil {
			if strings.Contains(string(content), want) {
				return
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("read child exit marker: %v", err)
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %q in %s", want, path)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForProcessExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatalf("check child process %d: %v", pid, err)
		}
		select {
		case <-deadline.C:
			t.Fatalf("child process %d still exists after cleanup", pid)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
