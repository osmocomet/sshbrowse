//go:build darwin || linux

package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	fileLockTimeout = 10 * time.Second
	fileLockPoll    = 10 * time.Millisecond
)

type unixFileLock struct {
	file *os.File
}

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
	lock := &unixFileLock{file: file}
	deadline := time.Now().Add(fileLockTimeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, errors.Join(fmt.Errorf("acquire profile lock: %w", err), file.Close())
		}
		if time.Now().After(deadline) {
			return nil, errors.Join(fmt.Errorf("timed out waiting for profile lock %s", path), file.Close())
		}
		time.Sleep(fileLockPoll)
	}
}

func (lock *unixFileLock) Close() error {
	unlockErr := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	closeErr := lock.file.Close()
	return errors.Join(unlockErr, closeErr)
}
