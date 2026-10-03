package session

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func observeProcessExit(pid int) error {
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

func signalProcessGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}

const maxGroupScanEntries = 4096
const groupScanBatchSize = 128

// liveGroupMembers checks only the process group owned by the PTY session. The
// session leader remains unreaped until this check completes, so pgid cannot
// be reused while cleanup is deciding whether that group still has live work.
func liveGroupMembers(pgid int, deadline time.Time) (bool, error) {
	proc, err := os.Open("/proc")
	if err != nil {
		return false, fmt.Errorf("open /proc: %w", err)
	}
	defer proc.Close()

	entriesScanned := 0
	for {
		entries, readErr := proc.ReadDir(groupScanBatchSize)
		for _, entry := range entries {
			entriesScanned++
			if entriesScanned > maxGroupScanEntries {
				return false, fmt.Errorf("process group scan exceeded %d /proc entries", maxGroupScanEntries)
			}
			if time.Now().After(deadline) {
				return false, errors.New("process group scan exceeded cleanup deadline")
			}

			pid, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}

			stat, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return false, fmt.Errorf("read process %d stat: %w", pid, err)
			}

			state, processGroup, err := parseLinuxProcessStat(stat)
			if err != nil {
				return false, fmt.Errorf("parse process %d stat: %w", pid, err)
			}
			if processGroup == pgid && state != 'Z' && state != 'X' && state != 'x' {
				return true, nil
			}
		}
		if errors.Is(readErr, io.EOF) {
			if time.Now().After(deadline) {
				return false, errors.New("process group scan exceeded cleanup deadline")
			}
			return false, nil
		}
		if readErr != nil {
			return false, fmt.Errorf("read /proc entries: %w", readErr)
		}
	}
}

func parseLinuxProcessStat(stat []byte) (byte, int, error) {
	statText := string(stat)
	closeComm := strings.LastIndex(statText, ") ")
	if closeComm < 0 {
		return 0, 0, errors.New("missing command name terminator")
	}

	fields := strings.Fields(statText[closeComm+2:])
	if len(fields) < 3 {
		return 0, 0, errors.New("missing state or process group")
	}
	processGroup, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid process group: %w", err)
	}
	return fields[0][0], processGroup, nil
}

func (s *Session) stopGroupAfterDrain(timedOut bool) error {
	hangupErr := s.signalGroup(syscall.SIGHUP)
	if timedOut {
		time.Sleep(exitGracePeriod)
		return s.signalGroup(syscall.SIGKILL)
	}
	deadlineAt := time.Now().Add(exitGracePeriod)
	deadline := time.NewTimer(exitGracePeriod)
	defer deadline.Stop()
	ticker := time.NewTicker(readPollInterval)
	defer ticker.Stop()

	for {
		live, err := liveGroupMembers(s.cmd.Process.Pid, deadlineAt)
		if err != nil {
			log.Printf("session: inspect process group: %v", err)
			<-deadline.C
			return s.signalGroup(syscall.SIGKILL)
		}
		if !live {
			return hangupErr
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			return s.signalGroup(syscall.SIGKILL)
		}
	}
}
