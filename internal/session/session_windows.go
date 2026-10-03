//go:build windows

package session

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const readBufferSize = 32 * 1024

// Drain output after the process exits, including delayed acknowledgements,
// while bounding any surviving writer's effect on session shutdown.
const (
	drainTimeout     = 60 * time.Second
	finalExitTimeout = 3 * time.Second
)

// Options describes the process to start and where its output goes.
type Options struct {
	Argv []string // command and arguments, e.g. {`C:\Windows\System32\OpenSSH\ssh.exe`, "lab-server"}
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
	// OnExit is called once, after the last OnData, with the process exit code.
	OnExit func(exitCode int)
}

// Session is one process attached to a Windows pseudo console. The job object
// owns the process tree so closing a session cannot leave ssh or a shell child
// behind.
type Session struct {
	process windows.Handle
	thread  windows.Handle
	job     windows.Handle

	handleMu sync.Mutex
	console  windows.Handle

	input  *os.File
	output *os.File

	inputMu          sync.Mutex
	closeInputOnce   sync.Once
	closeOutputOnce  sync.Once
	closeConsoleOnce sync.Once
	stopOnce         sync.Once
	cleanupOnce      sync.Once

	cleanupMu  sync.Mutex
	cleanupErr error

	readDone   chan struct{}
	reaped     chan struct{}
	exited     chan struct{}
	cancelData func()
}

// Start launches the command in a new ConPTY of the given size.
func Start(opts Options) (*Session, error) {
	if len(opts.Argv) == 0 {
		return nil, errors.New("session: empty argv")
	}
	if err := checkSize(opts.Cols, opts.Rows); err != nil {
		return nil, err
	}

	job, err := newJob()
	if err != nil {
		return nil, fmt.Errorf("session: create process job: %w", err)
	}
	closeJob := true
	defer func() {
		if closeJob {
			_ = windows.CloseHandle(job)
		}
	}()

	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	closePipeHandles := func() {
		closeWindowsHandle(&inputRead)
		closeWindowsHandle(&inputWrite)
		closeWindowsHandle(&outputRead)
		closeWindowsHandle(&outputWrite)
	}
	defer func() {
		if closeJob {
			closePipeHandles()
		}
	}()

	pipeSecurity := &windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}
	if err := windows.CreatePipe(&inputRead, &inputWrite, pipeSecurity, 0); err != nil {
		return nil, fmt.Errorf("session: create ConPTY input pipe: %w", err)
	}
	if err := windows.CreatePipe(&outputRead, &outputWrite, pipeSecurity, 0); err != nil {
		return nil, fmt.Errorf("session: create ConPTY output pipe: %w", err)
	}
	// The host keeps these ends. The pseudo console owns the other ends after
	// CreatePseudoConsole and CreateProcess has been called.
	if err := windows.SetHandleInformation(inputWrite, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, fmt.Errorf("session: protect ConPTY input pipe: %w", err)
	}
	if err := windows.SetHandleInformation(outputRead, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, fmt.Errorf("session: protect ConPTY output pipe: %w", err)
	}

	var console windows.Handle
	if err := windows.CreatePseudoConsole(
		windows.Coord{X: int16(opts.Cols), Y: int16(opts.Rows)},
		inputRead,
		outputWrite,
		0,
		&console,
	); err != nil {
		return nil, fmt.Errorf("session: create ConPTY: %w", err)
	}
	closeConsole := func() {
		if console != 0 {
			windows.ClosePseudoConsole(console)
			console = 0
		}
	}
	defer func() {
		if closeJob {
			closeConsole()
		}
	}()

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("session: allocate ConPTY process attributes: %w", err)
	}
	defer attributes.Delete()
	if err := attributes.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE expects the HPCON handle value,
		// not a pointer to the local handle variable.
		pseudoConsoleAttributeValue(console),
		unsafe.Sizeof(console),
	); err != nil {
		return nil, fmt.Errorf("session: configure ConPTY process attributes: %w", err)
	}

	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(opts.Argv))
	if err != nil {
		return nil, fmt.Errorf("session: encode command line: %w", err)
	}
	var currentDirectory *uint16
	if opts.Dir != "" {
		currentDirectory, err = windows.UTF16PtrFromString(opts.Dir)
		if err != nil {
			return nil, fmt.Errorf("session: encode working directory: %w", err)
		}
	}
	startup := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			// STARTF_USESTDHANDLES with NULL values is intentional. Windows can
			// otherwise duplicate a console parent's standard handles even when
			// bInheritHandles is false, bypassing ConPTY when the parent output
			// is redirected (as it is in CI).
			Flags: windows.STARTF_USESTDHANDLES,
		},
		ProcThreadAttributeList: attributes.List(),
	}
	var processInfo windows.ProcessInformation
	if err := windows.CreateProcess(
		nil,
		commandLine,
		nil,
		nil,
		false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_SUSPENDED,
		nil,
		currentDirectory,
		&startup.StartupInfo,
		&processInfo,
	); err != nil {
		return nil, fmt.Errorf("session: start %s: %w", opts.Argv[0], err)
	}

	// CreateProcess has copied the pseudo-console attribute. The process starts
	// suspended so it cannot create an untracked child before entering the job.
	if err := windows.AssignProcessToJobObject(job, processInfo.Process); err != nil {
		terminateAndReap(processInfo.Process)
		return nil, errors.Join(fmt.Errorf("session: assign process tree: %w", err), closeProcessInformation(processInfo))
	}
	if _, err := windows.ResumeThread(processInfo.Thread); err != nil {
		_ = windows.TerminateJobObject(job, 1)
		terminateAndReap(processInfo.Process)
		return nil, errors.Join(fmt.Errorf("session: resume process: %w", err), closeProcessInformation(processInfo))
	}

	// The pseudo console has its own references to these ends. Keeping them in
	// the host would prevent EOF and can deadlock ClosePseudoConsole.
	closeWindowsHandle(&inputRead)
	closeWindowsHandle(&outputWrite)
	input := os.NewFile(uintptr(inputWrite), "ConPTY input")
	if input != nil {
		inputWrite = 0
	}
	output := os.NewFile(uintptr(outputRead), "ConPTY output")
	if output != nil {
		outputRead = 0
	}
	if input == nil || output == nil {
		if input != nil {
			_ = input.Close()
		}
		if output != nil {
			_ = output.Close()
		}
		_ = windows.TerminateJobObject(job, 1)
		terminateAndReap(processInfo.Process)
		returnErr := closeProcessInformation(processInfo)
		closeConsole()
		return nil, errors.Join(errors.New("session: wrap ConPTY pipe handles"), returnErr)
	}

	s := &Session{
		process:    processInfo.Process,
		thread:     processInfo.Thread,
		job:        job,
		console:    console,
		input:      input,
		output:     output,
		readDone:   make(chan struct{}),
		reaped:     make(chan struct{}),
		exited:     make(chan struct{}),
		cancelData: opts.CancelData,
	}
	job = 0
	console = 0
	closeJob = false
	go s.readLoop(opts.OnData)
	go s.waitForExit(opts.OnExit)
	return s, nil
}

func pseudoConsoleAttributeValue(console windows.Handle) unsafe.Pointer {
	// HPCON is a HANDLE value. Reinterpret the uintptr bits as the pointer
	// value expected by UpdateProcThreadAttribute without passing &console.
	return *(*unsafe.Pointer)(unsafe.Pointer(&console))
}

func newJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return 0, errors.Join(err, windows.CloseHandle(job))
	}
	return job, nil
}

// Write sends keyboard input to the process. A WriteFile call may be partial,
// so keep writing until the entire browser/xterm input event is delivered.
func (s *Session) Write(data []byte) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	for len(data) > 0 {
		n, err := s.input.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return ioErrNoProgress
		}
		data = data[n:]
	}
	return nil
}

// Resize changes the ConPTY size. The child observes the new dimensions
// through the console API in the same way a native Windows terminal does.
func (s *Session) Resize(cols, rows int) error {
	if err := checkSize(cols, rows); err != nil {
		return err
	}
	s.handleMu.Lock()
	console := s.console
	if console == 0 {
		s.handleMu.Unlock()
		return os.ErrClosed
	}
	err := windows.ResizePseudoConsole(console, windows.Coord{X: int16(cols), Y: int16(rows)})
	s.handleMu.Unlock()
	return err
}

// Close terminates the complete process tree and waits for the bounded output
// drain. It never waits while holding a handle or input lock.
func (s *Session) Close() error {
	s.stopOnce.Do(func() {
		s.stopOutput()
		select {
		case <-s.reaped:
		default:
			s.terminateTree()
		}
		s.closeInput()
	})

	select {
	case <-s.exited:
		return s.cleanupError()
	case <-time.After(finalExitTimeout):
		s.terminateTree()
		s.closeInput()
		s.closeOutput()
		s.closeConsole()
		select {
		case <-s.exited:
			return s.cleanupError()
		case <-time.After(finalExitTimeout):
			return errors.New("session: process or output drain did not finish")
		}
	}
}

func (s *Session) readLoop(onData func([]byte) bool) {
	defer close(s.readDone)
	buffer := make([]byte, readBufferSize)
	for {
		n, err := s.output.Read(buffer)
		if n > 0 && onData != nil && !onData(buffer[:n]) {
			s.stopAfterOutputFailure()
			return
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) stopAfterOutputFailure() {
	s.stopOnce.Do(func() {
		s.terminateTree()
		s.closeInput()
		s.closeOutput()
		s.closeConsole()
	})
}

func (s *Session) waitForExit(onExit func(int)) {
	result, waitErr := windows.WaitForSingleObject(s.process, windows.INFINITE)
	exitCode := -1
	if waitErr != nil {
		s.recordCleanupError(fmt.Errorf("wait for process: %w", waitErr))
	} else if result == windows.WAIT_OBJECT_0 {
		var code uint32
		if err := windows.GetExitCodeProcess(s.process, &code); err != nil {
			s.recordCleanupError(fmt.Errorf("read process exit code: %w", err))
		} else {
			exitCode = int(code)
		}
	}
	close(s.reaped)
	// Closing input lets ConPTY finish and close its output after the child exits.
	s.closeInput()

	if !waitForDrain(s.readDone, drainTimeout) {
		s.stopOutput()
		// Close the output end before ClosePseudoConsole. This drains ordinary
		// output first while still bounding a child that retained a handle.
		s.closeOutput()
		s.closeConsole()
		_ = waitForDrain(s.readDone, finalExitTimeout)
	}

	s.cleanup()
	close(s.exited)
	if onExit != nil {
		onExit(exitCode)
	}
}

func (s *Session) stopOutput() {
	if s.cancelData != nil {
		s.cancelData()
	}
}

func (s *Session) terminateTree() {
	s.handleMu.Lock()
	defer s.handleMu.Unlock()
	job, process := s.job, s.process
	if job != 0 {
		// The process can already be reaped when a close races natural exit.
		_ = windows.TerminateJobObject(job, 1)
	}
	if process != 0 {
		// The job handles descendants; this fallback also covers an unusual job
		// assignment race or a process that is already outside the job.
		_ = windows.TerminateProcess(process, 1)
	}
}

func (s *Session) closeInput() {
	s.closeInputOnce.Do(func() {
		if err := s.input.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			s.recordCleanupError(fmt.Errorf("close ConPTY input: %w", err))
		}
	})
}

func (s *Session) closeOutput() {
	s.closeOutputOnce.Do(func() {
		if err := s.output.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			s.recordCleanupError(fmt.Errorf("close ConPTY output: %w", err))
		}
	})
}

func (s *Session) closeConsole() {
	s.closeConsoleOnce.Do(func() {
		s.handleMu.Lock()
		console := s.console
		s.console = 0
		s.handleMu.Unlock()
		if console != 0 {
			windows.ClosePseudoConsole(console)
		}
	})
}

func (s *Session) cleanup() {
	s.cleanupOnce.Do(func() {
		s.closeInput()
		s.closeOutput()
		s.closeConsole()
		s.handleMu.Lock()
		job, process, thread := s.job, s.process, s.thread
		s.job, s.process, s.thread = 0, 0, 0
		s.handleMu.Unlock()
		closeWindowsHandleWithError(job, "close process job", s.recordCleanupError)
		closeWindowsHandleWithError(process, "close process handle", s.recordCleanupError)
		closeWindowsHandleWithError(thread, "close process thread", s.recordCleanupError)
	})
}

func (s *Session) recordCleanupError(err error) {
	if err == nil {
		return
	}
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	s.cleanupErr = errors.Join(s.cleanupErr, err)
}

func (s *Session) cleanupError() error {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	return s.cleanupErr
}

func checkSize(cols, rows int) error {
	if cols < 1 || cols > 32767 || rows < 1 || rows > 32767 {
		return fmt.Errorf("session: invalid terminal size %dx%d", cols, rows)
	}
	return nil
}

var ioErrNoProgress = errors.New("session: no progress writing ConPTY input")

func closeWindowsHandle(handle *windows.Handle) {
	if *handle == 0 {
		return
	}
	// These handles are cleanup after a failed startup; the returned startup
	// error is the actionable failure and no handle is reused in this path.
	_ = windows.CloseHandle(*handle)
	*handle = 0
}

func closeWindowsHandleWithError(handle windows.Handle, label string, record func(error)) {
	if handle == 0 {
		return
	}
	if err := windows.CloseHandle(handle); err != nil {
		record(fmt.Errorf("%s: %w", label, err))
	}
}

func closeProcessInformation(info windows.ProcessInformation) error {
	var closeErr error
	if info.Process != 0 {
		closeErr = errors.Join(closeErr, windows.CloseHandle(info.Process))
	}
	if info.Thread != 0 {
		closeErr = errors.Join(closeErr, windows.CloseHandle(info.Thread))
	}
	return closeErr
}

func terminateAndReap(process windows.Handle) {
	if process == 0 {
		return
	}
	// Startup cleanup is already returning the process-creation failure; the
	// process may have exited between the failure and these best-effort calls.
	_ = windows.TerminateProcess(process, 1)
	_, _ = windows.WaitForSingleObject(process, 1000)
}
