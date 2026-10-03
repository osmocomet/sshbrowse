//go:build windows

package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const windowsSessionTestTimeout = 10 * time.Second

func TestPseudoConsoleAttributeValueUsesHandleValue(t *testing.T) {
	const console = windows.Handle(0x1234)
	if got := uintptr(pseudoConsoleAttributeValue(console)); got != uintptr(console) {
		t.Fatalf("ConPTY attribute value = %#x, want handle %#x", got, console)
	}
}

func TestConPTYRejectsUnsupportedSize(t *testing.T) {
	if _, err := Start(Options{Argv: []string{"not-started"}, Cols: 32768, Rows: 24}); err == nil {
		t.Fatal("invalid ConPTY size was accepted")
	}
}

func TestConPTYOutputResizeAndClose(t *testing.T) {
	powershell := testPowerShell(t)
	output := make(chan string, 32)
	exited := make(chan int, 1)
	running, err := Start(Options{
		Argv: []string{
			powershell,
			"-NoLogo",
			"-NoProfile",
			"-Command",
			"[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); [Console]::WriteLine('READY-日本語'); Start-Sleep -Seconds 30",
		},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			output <- string(chunk)
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
	waitForWindowsOutput(t, output, "READY-日本語")
	if err := running.Resize(100, 40); err != nil {
		t.Fatalf("resize ConPTY: %v", err)
	}
	if err := running.Close(); err != nil {
		t.Fatalf("close ConPTY: %v", err)
	}
	closed = true
	select {
	case <-exited:
	case <-time.After(windowsSessionTestTimeout):
		t.Fatal("closed ConPTY did not report process exit")
	}
}

func TestConPTYNaturalExitReportsCode(t *testing.T) {
	powershell := testPowerShell(t)
	exited := make(chan int, 1)
	running, err := Start(Options{
		Argv:   []string{powershell, "-NoLogo", "-NoProfile", "-Command", "exit 7"},
		Cols:   80,
		Rows:   24,
		OnExit: func(exitCode int) { exited <- exitCode },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	select {
	case exitCode := <-exited:
		if exitCode != 7 {
			t.Fatalf("PowerShell exit code = %d, want 7", exitCode)
		}
	case <-time.After(windowsSessionTestTimeout):
		t.Fatal("PowerShell did not exit")
	}
}

func TestConPTYNaturalExitDrainsDelayedAcknowledgement(t *testing.T) {
	powershell := testPowerShell(t)
	firstChunk := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseOutput := func() { releaseOnce.Do(func() { close(release) }) }
	chunks := make(chan string, 32)
	exited := make(chan int, 1)
	running, err := Start(Options{
		Argv: []string{
			powershell, "-NoLogo", "-NoProfile", "-Command",
			"[Console]::WriteLine('FIRST'); Start-Sleep -Milliseconds 200; [Console]::WriteLine('TRAILING'); exit 0",
		},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			chunks <- string(chunk)
			select {
			case <-firstChunk:
			default:
				close(firstChunk)
				<-release
			}
			return true
		},
		CancelData: releaseOutput,
		OnExit:     func(exitCode int) { exited <- exitCode },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	defer releaseOutput()

	select {
	case <-firstChunk:
	case <-time.After(windowsSessionTestTimeout):
		t.Fatal("ConPTY did not deliver its first output chunk")
	}
	select {
	case <-running.reaped:
	case <-time.After(windowsSessionTestTimeout):
		t.Fatal("PowerShell did not exit while output was awaiting acknowledgement")
	}
	select {
	case <-exited:
		t.Fatal("session exited before delayed output was acknowledged")
	case <-time.After(2200 * time.Millisecond):
	}
	releaseOutput()

	select {
	case exitCode := <-exited:
		if exitCode != 0 {
			t.Fatalf("PowerShell exit code = %d, want 0", exitCode)
		}
	case <-time.After(windowsSessionTestTimeout):
		t.Fatal("ConPTY did not drain trailing output")
	}
	var output strings.Builder
	chunkCount := len(chunks)
	for range chunkCount {
		output.WriteString(<-chunks)
	}
	if chunkCount < 2 || !strings.Contains(output.String(), "FIRST") || !strings.Contains(output.String(), "TRAILING") {
		t.Fatalf("received %d chunks before exit: %q, want first and trailing output", chunkCount, output.String())
	}
}

func TestConPTYReportsResizedDimensions(t *testing.T) {
	powershell := testPowerShell(t)
	output := make(chan string, 32)
	running, err := Start(Options{
		Argv: []string{
			powershell,
			"-NoLogo",
			"-NoProfile",
			"-Command",
			"[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); [Console]::WriteLine('READY'); $null = Read-Host; [Console]::WriteLine(('SIZE:{0}x{1}' -f [Console]::WindowWidth, [Console]::WindowHeight))",
		},
		Cols: 80,
		Rows: 24,
		OnData: func(chunk []byte) bool {
			output <- string(chunk)
			return true
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	waitForWindowsOutput(t, output, "READY")
	if err := running.Resize(100, 40); err != nil {
		t.Fatalf("resize ConPTY: %v", err)
	}
	if err := running.Write([]byte("ok\r\n")); err != nil {
		t.Fatalf("write resized input: %v", err)
	}
	waitForWindowsOutput(t, output, "SIZE:100x40")
	if err := running.Close(); err != nil {
		t.Fatalf("close resized ConPTY: %v", err)
	}
}

func TestConPTYPreservesSustainedUnicodeOutputOrder(t *testing.T) {
	powershell := testPowerShell(t)
	const lineCount = 512
	firstLine := "LINE-0001-ASCII"
	lastLine := fmt.Sprintf("LINE-%04d-ASCII", lineCount)
	unicodeLine := "UNICODE-日本語"
	var output strings.Builder
	var outputMu sync.Mutex
	exited := make(chan int, 1)
	running, err := Start(Options{
		Argv: []string{
			powershell,
			"-NoLogo",
			"-NoProfile",
			"-Command",
			fmt.Sprintf("[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); 1..%d | ForEach-Object { [Console]::WriteLine(('LINE-{0:D4}-ASCII' -f $_)) }; [Console]::WriteLine('%s'); Start-Sleep -Milliseconds 500", lineCount, unicodeLine),
		},
		Cols: 100,
		// Keep the console taller than the burst so ConPTY does not need to
		// render scrollback while this test checks the output stream.
		Rows: lineCount + 2,
		OnData: func(chunk []byte) bool {
			outputMu.Lock()
			output.Write(chunk)
			outputMu.Unlock()
			return true
		},
		OnExit: func(exitCode int) { exited <- exitCode },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	select {
	case exitCode := <-exited:
		if exitCode != 0 {
			t.Fatalf("PowerShell exit code = %d, want 0", exitCode)
		}
	case <-time.After(windowsSessionTestTimeout):
		t.Fatal("timed out waiting for sustained ConPTY output and process exit")
	}
	outputMu.Lock()
	received := strings.ReplaceAll(output.String(), "\r", "")
	outputMu.Unlock()
	first, last := strings.Index(received, firstLine), strings.Index(received, lastLine)
	if first < 0 || last < first || !strings.Contains(received, unicodeLine) {
		t.Fatalf("sustained output is incomplete or out of order: first=%d last=%d unicode=%t bytes=%d", first, last, strings.Contains(received, unicodeLine), len(received))
	}
}

func TestConPTYConcurrentResizeAndCloseIsBounded(t *testing.T) {
	powershell := testPowerShell(t)
	for iteration := 0; iteration < 3; iteration++ {
		running, err := Start(Options{
			Argv: []string{powershell, "-NoLogo", "-NoProfile", "-Command", "Start-Sleep -Seconds 30"},
			Cols: 80,
			Rows: 24,
		})
		if err != nil {
			t.Fatalf("start iteration %d: %v", iteration, err)
		}
		defer running.Close()

		var waitGroup sync.WaitGroup
		for worker := 0; worker < 4; worker++ {
			waitGroup.Add(1)
			go func(worker int) {
				defer waitGroup.Done()
				for resize := 0; resize < 32; resize++ {
					_ = running.Resize(80+((worker+resize)%40), 24+((worker+resize)%12))
				}
			}(worker)
		}

		closeErrors := make(chan error, 4)
		for closer := 0; closer < 4; closer++ {
			go func() { closeErrors <- running.Close() }()
		}
		waitGroup.Wait()
		for closer := 0; closer < 4; closer++ {
			select {
			case err := <-closeErrors:
				if err != nil {
					t.Fatalf("close iteration %d: %v", iteration, err)
				}
			case <-time.After(windowsSessionTestTimeout):
				t.Fatalf("close iteration %d did not finish", iteration)
			}
		}
	}
}

func testPowerShell(t *testing.T) string {
	t.Helper()
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = os.Getenv("WINDIR")
	}
	if root == "" {
		root = `C:\Windows`
	}
	path := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("Windows PowerShell is unavailable at %s: %v", path, err)
	}
	return path
}

func waitForWindowsOutput(t *testing.T, output <-chan string, want string) {
	t.Helper()
	var received strings.Builder
	deadline := time.NewTimer(windowsSessionTestTimeout)
	defer deadline.Stop()
	for {
		if strings.Contains(received.String(), want) {
			return
		}
		select {
		case chunk := <-output:
			received.WriteString(chunk)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %q in ConPTY output %q", want, received.String())
		}
	}
}
