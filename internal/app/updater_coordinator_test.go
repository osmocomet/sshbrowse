package app

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateCoordinatorSerializesStagingAndRestart(t *testing.T) {
	coordinator := &updateCoordinator{}
	updateStarted := make(chan struct{})
	finishUpdate := make(chan struct{})
	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		started, err := coordinator.runUpdate(func() error {
			close(updateStarted)
			<-finishUpdate
			return nil
		})
		if !started || err != nil {
			t.Errorf("runUpdate = (%v, %v), want update to start", started, err)
		}
	}()
	<-updateStarted

	var confirmed func(bool)
	if coordinator.restart(func(decide func(bool)) { confirmed = decide }, func() error {
		t.Error("restart during staging launched a helper")
		return nil
	}, nil) {
		t.Fatal("restart during staging was accepted")
	}
	if started, err := coordinator.runUpdate(func() error {
		t.Error("overlapping staging operation started")
		return nil
	}); started || err != nil {
		t.Fatalf("overlapping runUpdate = (%v, %v), want it skipped", started, err)
	}

	close(finishUpdate)
	<-updateDone

	var helperCalls atomic.Int32
	if !coordinator.restart(func(decide func(bool)) { confirmed = decide }, func() error {
		helperCalls.Add(1)
		return nil
	}, nil) {
		t.Fatal("restart after staging was rejected")
	}
	if confirmed == nil {
		t.Fatal("restart did not request confirmation")
	}
	if coordinator.restart(func(func(bool)) { t.Error("duplicate restart requested confirmation") }, func() error {
		t.Error("duplicate restart launched a helper")
		return nil
	}, nil) {
		t.Fatal("duplicate restart was accepted")
	}
	if started, err := coordinator.runUpdate(func() error {
		t.Error("staging started during restart confirmation")
		return nil
	}); started || err != nil {
		t.Fatalf("runUpdate during restart = (%v, %v), want it skipped", started, err)
	}

	confirmed(true)
	confirmed(true)
	waitForUpdateCoordinator(t, func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		return coordinator.restartPending
	})
	if helperCalls.Load() != 1 {
		t.Fatalf("helper calls = %d, want 1", helperCalls.Load())
	}
	if started, err := coordinator.runUpdate(func() error { return nil }); started || err != nil {
		t.Fatalf("runUpdate after helper launch = (%v, %v), want it skipped", started, err)
	}
}

func TestUpdateCoordinatorCancellationAndFailureAllowRetry(t *testing.T) {
	coordinator := &updateCoordinator{}
	var confirmed func(bool)
	if !coordinator.restart(func(decide func(bool)) { confirmed = decide }, func() error {
		t.Error("cancelled restart launched a helper")
		return nil
	}, nil) {
		t.Fatal("restart attempt was rejected")
	}
	confirmed(false)
	if started, err := coordinator.runUpdate(func() error { return nil }); !started || err != nil {
		t.Fatalf("runUpdate after cancellation = (%v, %v), want it started", started, err)
	}

	guard := &QuitGuard{sessions: &Sessions{}}
	var failed func(bool)
	failureReported := make(chan struct{})
	if !coordinator.restart(guard.confirmForRestart, func() error {
		return errors.New("helper spawn failed")
	}, func(error) { close(failureReported) }) {
		t.Fatal("restart after cancellation was rejected")
	}
	select {
	case <-failureReported:
	case <-time.After(time.Second):
		t.Fatal("restart failure was not reported")
	}
	waitForUpdateCoordinator(t, func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		return !coordinator.restartRunning && !coordinator.restartPending
	})
	if guard.confirmed.Load() {
		t.Fatal("failed helper launch authorized quitting")
	}

	if !coordinator.restart(func(decide func(bool)) { failed = decide }, func() error { return nil }, nil) {
		t.Fatal("retry after helper failure was rejected")
	}
	failed(true)
	waitForUpdateCoordinator(t, func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		return coordinator.restartPending
	})
}

func TestQuitGuardAuthorizesOnlyAfterUpdaterHelperStarts(t *testing.T) {
	guard := &QuitGuard{sessions: &Sessions{}}
	confirmed := false
	guard.confirmForRestart(func(decision bool) { confirmed = decision })
	if !confirmed || guard.confirmed.Load() {
		t.Fatal("confirmation should approve restart without authorizing quit early")
	}
	guard.authorizeRestartQuit()
	if !guard.confirmed.Load() {
		t.Fatal("helper startup did not authorize restart quit")
	}
}

func waitForUpdateCoordinator(t *testing.T, condition func() bool) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("coordinator did not reach the expected state")
		}
	}
}
