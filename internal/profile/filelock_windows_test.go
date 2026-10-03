//go:build windows

package profile

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWindowsProfileLocksAllowIndependentStores(t *testing.T) {
	firstPath := filepath.Join(t.TempDir(), "first", "connections.json")
	secondPath := filepath.Join(t.TempDir(), "second", "connections.json")
	first, err := acquireFileLock(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := acquireFileLock(secondPath)
	if err != nil {
		releaseFileLock(first)
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsProfileLocksSerializeConcurrentStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile with spaces", "connections.json")
	const writerCount = 16
	start := make(chan struct{})
	errors := make(chan error, writerCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(writerCount)
	for index := 0; index < writerCount; index++ {
		index := index
		go func() {
			defer waitGroup.Done()
			<-start
			_, err := NewStore(path).Save(Connection{
				Name: fmt.Sprintf("connection-%d", index),
				Host: "host.example",
			})
			errors <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	connections, err := NewStore(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != writerCount {
		t.Fatalf("connections = %d, want %d", len(connections), writerCount)
	}
}

func TestWindowsProfileLockReleasesAndReacquires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile with spaces", "connections.json")
	first, err := acquireFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsProfileLockReleasesAfterProfileReadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile with spaces", "connections.json")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	if _, err := store.List(); err == nil {
		t.Fatal("listing a directory as the profile file succeeded")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(Connection{Name: "after failure", Host: "host.example"}); err != nil {
		t.Fatalf("profile lock was not released after a failed read: %v", err)
	}
}

func TestWindowsProfileLockSerializesProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile with spaces", "connections.json")
	marker := filepath.Join(t.TempDir(), "child acquired")
	first, err := acquireFileLock(path)
	if err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestWindowsProfileLockHelper")
	command.Env = append(os.Environ(),
		"SSHBROWSE_PROFILE_LOCK_HELPER=1",
		"SSHBROWSE_PROFILE_LOCK_PATH="+path,
		"SSHBROWSE_PROFILE_LOCK_MARKER="+marker,
	)
	if err := command.Start(); err != nil {
		releaseFileLock(first)
		t.Fatal(err)
	}
	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		_ = command.Process.Kill()
		<-waitResult
		releaseFileLock(first)
		t.Fatalf("child acquired the profile lock before release: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waitResult:
		if err != nil {
			t.Fatalf("profile lock helper: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = command.Process.Kill()
		<-waitResult
		t.Fatal("profile lock helper did not finish after release")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("profile lock helper did not acquire after release: %v", err)
	}
}

func TestWindowsProfileLockHelper(t *testing.T) {
	if os.Getenv("SSHBROWSE_PROFILE_LOCK_HELPER") != "1" {
		return
	}
	lock, err := acquireFileLock(os.Getenv("SSHBROWSE_PROFILE_LOCK_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SSHBROWSE_PROFILE_LOCK_MARKER"), []byte("acquired"), 0o600); err != nil {
		releaseFileLock(lock)
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}
