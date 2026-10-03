// Package atomicfile replaces a small file through a completed temporary
// write. The platform-specific replacement keeps the old file in place when
// the write fails and handles Windows' non-replacing os.Rename behavior.
package atomicfile

import (
	"io"
	"os"
	"path/filepath"
)

// WriteReplace writes data to path with mode and replaces an existing path.
// It intentionally does not fsync the file or parent directory; callers keep
// the existing application's power-loss behavior while avoiding truncation.
func WriteReplace(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			// Best-effort cleanup; the previous file remains intact.
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	written, err := temporary.Write(data)
	if err != nil {
		// The write error is the useful failure; closing only releases the temp fd.
		_ = temporary.Close()
		return err
	}
	if written != len(data) {
		_ = temporary.Close()
		return io.ErrShortWrite
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replace(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}
