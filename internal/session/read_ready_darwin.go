//go:build darwin

package session

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

func waitReadable(fd int, timeout time.Duration) (bool, error) {
	var readSet unix.FdSet
	const bitsPerWord = 32
	if fd < 0 || fd >= len(readSet.Bits)*bitsPerWord {
		return false, fmt.Errorf("PTY descriptor %d exceeds select capacity", fd)
	}
	readSet.Bits[fd/bitsPerWord] = 1 << uint(fd%bitsPerWord)
	timeoutValue := unix.NsecToTimeval(timeout.Nanoseconds())
	ready, err := unix.Select(fd+1, &readSet, nil, nil, &timeoutValue)
	return ready > 0, err
}
