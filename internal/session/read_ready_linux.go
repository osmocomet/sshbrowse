//go:build linux

package session

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func waitReadable(fd int, timeout time.Duration) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	_, err := unix.Poll(fds, int(timeout/time.Millisecond))
	if err != nil {
		return false, err
	}
	if fds[0].Revents&unix.POLLNVAL != 0 {
		return false, os.ErrClosed
	}
	return fds[0].Revents != 0, nil
}
