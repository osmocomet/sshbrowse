package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

func TestGuardedUpdaterHostRegistersRestartThroughCoordinatorAndQuitGuard(t *testing.T) {
	wailsApp := application.New(application.Options{Name: "SSHBrowse"})
	guard := &QuitGuard{sessions: &Sessions{}}
	coordinator := &updateCoordinator{}
	host := newGuardedUpdaterHost(wailsApp, guard, coordinator)
	// Wails reuses its singleton app when tests run more than once.
	t.Cleanup(host.stopRestartListener)
	restartStarted := make(chan struct{})
	finishRestart := make(chan struct{})
	host.restartUpdate = func(context.Context) error {
		close(restartStarted)
		<-finishRestart
		return errors.New("helper startup failed")
	}
	wailsApp.Event.Emit(updater.EventUserRestart)
	select {
	case <-restartStarted:
	case <-time.After(time.Second):
		t.Fatal("restart event did not reach the guarded host")
	}
	if started, err := coordinator.runUpdate(func() error {
		t.Error("update started while restart helper was running")
		return nil
	}); started || err != nil {
		t.Fatalf("update during restart = (%v, %v), want rejected", started, err)
	}
	close(finishRestart)
	waitForUpdateCoordinator(t, func() bool {
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		return !coordinator.restartRunning && !coordinator.restartPending
	})
	if guard.confirmed.Load() {
		t.Fatal("failed restart authorized the application to quit")
	}
}
