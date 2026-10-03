package app

import (
	"context"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// guardedUpdaterHost routes restart through the app's coordinator and quit guard.
// The updater runs with WindowNone, so it never opens a framework window.
type guardedUpdaterHost struct {
	app                 *application.App
	quitGuard           *QuitGuard
	coordinator         *updateCoordinator
	restartUpdate       func(context.Context) error
	stopRestartListener func()
}

func newGuardedUpdaterHost(wailsApp *application.App, quitGuard *QuitGuard, coordinator *updateCoordinator) *guardedUpdaterHost {
	host := &guardedUpdaterHost{app: wailsApp, quitGuard: quitGuard, coordinator: coordinator}
	// Check and DownloadAndInstall do not register Wails' user-action listeners.
	host.stopRestartListener = host.OnEvent(updater.EventUserRestart, nil)
	return host
}

func (h *guardedUpdaterHost) Emit(name string, data ...any) bool {
	// All checks report through update:check-result. Downloads and restarts
	// retain their Wails events; check errors are handled by the shared runner.
	switch name {
	case updater.EventCheckStarted, updater.EventNoUpdate, updater.EventUpdateAvailable:
		return true
	case updater.EventError:
		if len(data) > 0 {
			if info, ok := data[0].(updater.ErrorInfo); ok && info.Stage == updater.StageCheck {
				return true
			}
		}
	}
	return h.app.Event.Emit(name, data...)
}

func (h *guardedUpdaterHost) OnEvent(name string, callback func(payload any)) func() {
	var handler func(any)
	switch name {
	case updater.EventUserRestart:
		handler = func(any) { go h.restart() }
	default:
		handler = callback
	}
	return h.app.Event.On(name, func(event *application.CustomEvent) {
		var payload any
		if event != nil {
			payload = event.Data
		}
		handler(payload)
	})
}

func (*guardedUpdaterHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	return nil
}

func (h *guardedUpdaterHost) Quit() {
	if h.quitGuard != nil {
		h.quitGuard.authorizeRestartQuit()
	}
	h.app.Quit()
}

func (h *guardedUpdaterHost) restart() {
	if h.restartUpdate == nil || h.coordinator == nil || h.quitGuard == nil {
		return
	}
	h.coordinator.restart(
		h.quitGuard.confirmForRestart,
		func() error { return h.restartUpdate(context.Background()) },
		func(err error) {
			log.Printf("Restart to apply update: %v", err)
			h.app.Event.Emit(updater.EventError, updater.ErrorInfo{
				Stage:   updater.Stage("restart"),
				Message: "Could not restart to apply the update: " + err.Error(),
			})
		},
	)
}
