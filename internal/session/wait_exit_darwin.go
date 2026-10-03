package session

import (
	"errors"
	"fmt"
	"log"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func observeProcessExit(pid int) error {
	queue, err := unix.Kqueue()
	if err != nil {
		return err
	}
	defer unix.Close(queue) // Closing this notification-only descriptor cannot affect the session.

	change := unix.Kevent_t{
		Ident: uint64(pid), Filter: unix.EVFILT_PROC,
		Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT,
	}
	for {
		_, err = unix.Kevent(queue, []unix.Kevent_t{change}, nil, nil)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	// A child that exited before registration remains an unreaped zombie.
	if errors.Is(err, unix.ESRCH) {
		return nil
	}
	if err != nil {
		return err
	}
	// Darwin accepts EVFILT_PROC registration on an already-exited child but
	// does not replay NOTE_EXIT. The unreaped zombie is still safe to inspect.
	if exited, err := processIsZombie(pid); err != nil || exited {
		return err
	}
	var events [1]unix.Kevent_t
	for {
		count, waitErr := unix.Kevent(queue, nil, events[:], nil)
		if errors.Is(waitErr, unix.EINTR) {
			continue
		}
		if waitErr != nil {
			return waitErr
		}
		if count != 1 || events[0].Filter != unix.EVFILT_PROC || events[0].Fflags&unix.NOTE_EXIT == 0 {
			return fmt.Errorf("unexpected process exit event: %+v", events[0])
		}
		return nil
	}
}

const darwinZombieState = 5 // SZOMB from sys/proc.h

func processIsZombie(pid int) (bool, error) {
	process, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return false, err
	}
	return process.Proc.P_stat == darwinZombieState, nil
}

func liveGroupMembers(pgid int) (bool, error) {
	members, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.Proc.P_stat != darwinZombieState {
			return true, nil
		}
	}
	return false, nil
}

func signalProcessGroup(pgid int, sig syscall.Signal) error {
	// A zombie-only process group returns EPERM on Darwin. Inspect the group
	// while its unreaped leader still pins the process-group identity.
	live, err := liveGroupMembers(pgid)
	if err != nil {
		// The unreaped leader still pins pgid, so a group signal is safe even
		// when sysctl cannot tell us whether a live member remains.
		return syscall.Kill(-pgid, sig)
	}
	if !live {
		return nil
	}
	err = syscall.Kill(-pgid, sig)
	if errors.Is(err, syscall.EPERM) {
		// XNU can return EPERM while the last member is exiting, before sysctl
		// reports it as a zombie. Confirm that transition before treating EPERM
		// as harmless; retain the error if any live child remains in the group.
		for attempt := 0; attempt < 10; attempt++ {
			members, checkErr := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
			if checkErr != nil {
				return err
			}
			leaderExiting := false
			for _, member := range members {
				if member.Proc.P_stat == darwinZombieState {
					continue
				}
				if int(member.Proc.P_pid) != pgid {
					return err
				}
				leaderExiting = true
			}
			if !leaderExiting {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	return err
}

func (s *Session) stopGroupAfterDrain(_ bool) error {
	live, err := liveGroupMembers(s.cmd.Process.Pid)
	if err != nil {
		log.Printf("session: inspect process group: %v", err)
		_ = s.signalGroup(syscall.SIGHUP) // SIGKILL is the final cleanup result.
		time.Sleep(exitGracePeriod)
		return s.signalGroup(syscall.SIGKILL)
	}
	if !live {
		return nil
	}
	_ = s.signalGroup(syscall.SIGHUP) // A later live-group check determines whether cleanup succeeded.
	deadline := time.NewTimer(exitGracePeriod)
	defer deadline.Stop()
	ticker := time.NewTicker(readPollInterval)
	defer ticker.Stop()
	for {
		live, err = liveGroupMembers(s.cmd.Process.Pid)
		if err != nil {
			return s.signalGroup(syscall.SIGKILL)
		}
		if !live {
			return nil
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return s.signalGroup(syscall.SIGKILL)
		}
	}
}
