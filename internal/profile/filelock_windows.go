//go:build windows

package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

const (
	fileLockTimeout = 10 * time.Second
	fileLockPoll    = 10 * time.Millisecond
)

type windowsFileLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

// LockFileEx associates the lock with the open file handle, not the OS thread.
// That lets a lock survive goroutine rescheduling between acquisition and release.
func acquireFileLock(path string) (fileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create profile lock directory: %w", err)
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open profile lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("protect profile lock: %w", err), file.Close())
	}
	lock := &windowsFileLock{file: file}
	deadline := time.Now().Add(fileLockTimeout)
	for {
		err = windows.LockFileEx(
			windows.Handle(file.Fd()),
			windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
			0, 1, 0, &lock.overlapped,
		)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return nil, errors.Join(fmt.Errorf("acquire profile lock: %w", err), file.Close())
		}
		if time.Now().After(deadline) {
			return nil, errors.Join(fmt.Errorf("timed out waiting for profile lock %s", path), file.Close())
		}
		time.Sleep(fileLockPoll)
	}
}

func (lock *windowsFileLock) Close() error {
	unlockErr := windows.UnlockFileEx(windows.Handle(lock.file.Fd()), 0, 1, 0, &lock.overlapped)
	closeErr := lock.file.Close()
	return errors.Join(unlockErr, closeErr)
}
