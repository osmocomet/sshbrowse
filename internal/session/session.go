//go:build darwin || linux

// Package session runs a command under a pseudo-terminal and streams its output.
package session

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

const readBufferSize = 32 * 1024

// How long Close waits for the process to exit after hangup before killing it.
const exitGracePeriod = 2 * time.Second

// A delayed terminal acknowledgement may take 30 seconds. This is one total
// allowance after process exit, including any surviving PTY writers.
const drainTimeout = 60 * time.Second

const readPollInterval = 100 * time.Millisecond

// How long Close waits after SIGKILL and for the bounded output drain.
const finalExitTimeout = 3 * time.Second

// Options describes the process to start and where its output goes.
type Options struct {
	Argv []string // command and arguments, e.g. {"/usr/bin/ssh", "lab-server"}
	Dir  string   // empty inherits the application's working directory
	Cols int
	Rows int
	// OnData receives terminal output from a single goroutine. The slice is
	// only valid for the duration of the call.
	// Returning false stops the process because its output can no longer be
	// delivered without losing ordering.
	OnData func(data []byte) bool
	// CancelData must release a pending OnData when output is no longer needed.
	CancelData func()
	// OnExit is called once, after the last OnData, with the exit code
	// (-1 if the process was killed by a signal).
	OnExit func(exitCode int)
}

// Session is one running process attached to a pseudo-terminal.
type Session struct {
	cmd           *exec.Cmd
	terminal      *os.File // master side of the pty
	terminalFD    int
	terminalMu    sync.Mutex
	readStop      chan struct{}
	readStopOnce  sync.Once
	cancelData    func()
	drainLimit    time.Duration
	readDone      chan struct{}
	reaped        chan struct{} // closed once Wait has returned; the pid is invalid after this
	exited        chan struct{} // closed just before OnExit is called
	groupMu       sync.Mutex
	groupReleased bool  // protected by groupMu; no group signal may race with Wait
	cleanupErr    error // written before exited closes; returned by Close
}

// Start launches the command in a new pseudo-terminal of the given size.
func Start(opts Options) (*Session, error) {
	return startWithDrainLimit(opts, drainTimeout)
}

func startWithDrainLimit(opts Options, limit time.Duration) (*Session, error) {
	if len(opts.Argv) == 0 {
		return nil, errors.New("session: empty argv")
	}
	if err := checkSize(opts.Cols, opts.Rows); err != nil {
		return nil, err
	}
	cmd := exec.Command(opts.Argv[0], opts.Argv[1:]...)
	cmd.Dir = opts.Dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	size := &pty.Winsize{Cols: uint16(opts.Cols), Rows: uint16(opts.Rows)}
	terminal, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return nil, fmt.Errorf("session: start %s: %w", opts.Argv[0], err)
	}
	terminalFD := int(terminal.Fd())
	var readinessErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, readinessErr = waitReadable(terminalFD, 0)
		if !errors.Is(readinessErr, unix.EINTR) {
			break
		}
	}
	if readinessErr != nil {
		if err := terminal.Close(); err != nil {
			readinessErr = errors.Join(readinessErr, fmt.Errorf("close failed PTY: %w", err))
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			readinessErr = errors.Join(readinessErr, fmt.Errorf("stop failed PTY session: %w", err))
		}
		_ = cmd.Wait() // A killed command exits nonzero; readinessErr is the startup failure.
		return nil, fmt.Errorf("session: monitor PTY output: %w", readinessErr)
	}

	s := &Session{
		cmd:        cmd,
		terminal:   terminal,
		terminalFD: terminalFD,
		readStop:   make(chan struct{}),
		cancelData: opts.CancelData,
		drainLimit: limit,
		readDone:   make(chan struct{}),
		reaped:     make(chan struct{}),
		exited:     make(chan struct{}),
	}
	go s.readLoop(opts.OnData)
	go s.waitForExit(opts.OnExit)
	return s, nil
}

// Write sends keyboard input to the process.
func (s *Session) Write(data []byte) error {
	_, err := s.terminal.Write(data)
	return err
}

// Resize changes the terminal size; the process gets SIGWINCH.
func (s *Session) Resize(cols, rows int) error {
	if err := checkSize(cols, rows); err != nil {
		return err
	}
	return pty.Setsize(s.terminal, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// Close hangs up the process the way a closing terminal window does: SIGHUP to
// its whole process group, then SIGKILL if it is still alive after the grace
// period. A process that has already been reaped is never signalled, because
// its pid may belong to someone else by now.
func (s *Session) Close() error {
	s.stopOutput()
	select {
	case <-s.reaped:
	default:
		_ = s.signalGroup(syscall.SIGHUP) // Exit cleanup checks the group before reaping.
		select {
		case <-s.reaped:
		case <-time.After(exitGracePeriod):
			_ = s.signalGroup(syscall.SIGKILL) // Exit cleanup checks the group before reaping.
			select {
			case <-s.reaped:
			case <-time.After(finalExitTimeout):
				_ = s.closeTerminal() // Closing the PTY is the last available way to release the process.
				return errors.New("session: process did not exit after SIGKILL")
			}
		}
	}
	select {
	case <-s.exited:
	case <-time.After(finalExitTimeout):
		_ = s.closeTerminal() // The process is gone; only a stuck output drain can remain.
		return errors.New("session: output drain did not finish")
	}
	return errors.Join(s.closeTerminal(), s.cleanupErr)
}

// The process is a session leader (pty sets Setsid), so its pid is also its
// process group id. Signalling the group reaches children such as a
// ProxyCommand. ESRCH means the group is already gone, which is fine.
func (s *Session) signalGroup(sig syscall.Signal) error {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if s.groupReleased {
		return nil
	}
	if err := signalProcessGroup(s.cmd.Process.Pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		log.Printf("session: signal process group: %v", err)
		return err
	}
	return nil
}

func (s *Session) reap() {
	s.groupMu.Lock()
	s.groupReleased = true
	s.groupMu.Unlock()
	_ = s.cmd.Wait() // Exit status is reported through ProcessState.
	close(s.reaped)
}

// The reader uses timed polling, so it can stop before this closes the master.
func (s *Session) closeTerminal() error {
	s.stopReader()
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	err := s.terminal.Close()
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

// Timed readiness checks make an idle PTY reader cancellable without changing
// the descriptor's blocking mode (and therefore its write behavior).
func (s *Session) readTerminal(buffer []byte) (int, error) {
	for {
		s.terminalMu.Lock()
		select {
		case <-s.readStop:
			s.terminalMu.Unlock()
			return 0, os.ErrClosed
		default:
		}
		ready, waitErr := waitReadable(s.terminalFD, readPollInterval)
		if errors.Is(waitErr, unix.EINTR) || (waitErr == nil && !ready) {
			s.terminalMu.Unlock()
			continue
		}
		if waitErr != nil {
			s.terminalMu.Unlock()
			return 0, waitErr
		}
		n, readErr := unix.Read(s.terminalFD, buffer)
		s.terminalMu.Unlock()
		if errors.Is(readErr, unix.EINTR) {
			continue
		}
		if errors.Is(readErr, unix.EIO) || (readErr == nil && n == 0) {
			return n, io.EOF
		}
		return n, readErr
	}
}

func (s *Session) stopReader() {
	s.readStopOnce.Do(func() { close(s.readStop) })
}

func (s *Session) stopOutput() {
	if s.cancelData != nil {
		s.cancelData()
	}
	s.stopReader()
}

func (s *Session) stopAfterOutputFailure() {
	go func() {
		if err := s.Close(); err != nil {
			log.Printf("session: close after output failure: %v", err)
		}
	}()
}

func (s *Session) waitForExit(onExit func(int)) {
	// Observe exit without reaping first. The leader must retain its PID until
	// the PTY is drained and any surviving members of its group are stopped.
	waitErr := observeProcessExit(s.cmd.Process.Pid)
	if waitErr != nil {
		log.Printf("session: monitor process exit: %v", waitErr)
		// The leader is still unreaped, so stop its group before losing the PID.
		s.stopOutput()
		_ = s.signalGroup(syscall.SIGKILL) // The monitor error already controls this path.
		s.reap()
	}
	if !waitForDrain(s.readDone, s.drainLimit) {
		s.stopOutput()
		// A caller that ignores CancelData cannot hold session shutdown open.
		_ = waitForDrain(s.readDone, finalExitTimeout)
		if waitErr == nil {
			_ = s.stopGroupAfterDrain(true) // The final group check below determines cleanupErr.
		}
	}
	if err := s.closeTerminal(); err != nil {
		log.Printf("session: close PTY after process exit: %v", err)
	}
	if waitErr == nil {
		s.cleanupErr = s.stopGroupAfterDrain(false)
		s.reap()
	}
	close(s.exited)
	onExit(s.cmd.ProcessState.ExitCode())
}

func checkSize(cols, rows int) error {
	if cols < 1 || cols > 65535 || rows < 1 || rows > 65535 {
		return fmt.Errorf("session: invalid terminal size %dx%d", cols, rows)
	}
	return nil
}
