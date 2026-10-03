package profile

import "log"

type fileLock interface {
	Close() error
}

// Lock release is best-effort at process shutdown, but it is still reported:
// the operating system releases both supported lock types when the process
// exits, while a release failure is useful evidence during normal operation.
func releaseFileLock(lock fileLock) {
	if err := lock.Close(); err != nil {
		log.Printf("profile: release file lock: %v", err)
	}
}
