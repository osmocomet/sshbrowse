package session

import "time"

func waitForDrain(readDone <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-readDone:
		return true
	case <-timer.C:
		return false
	}
}
