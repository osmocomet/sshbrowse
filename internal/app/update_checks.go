package app

import (
	"context"
	"errors"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"

	"sshbrowse/internal/buildinfo"
)

const (
	EventStartupUpdateCheck = "update:startup-check"
	EventUpdateCheckStarted = "update:check-started"
	EventUpdateCheckResult  = "update:check-result"
	startupUpdateTimeout    = 20 * time.Second
)

type UpdateCheckResult struct {
	Automatic  bool   `json:"automatic"`
	Checked    bool   `json:"checked"`
	Version    string `json:"version"`
	ReleaseURL string `json:"releaseURL"`
	Error      string `json:"error"`
}

// RegisterUpdateChecks adds a frontend-ready startup check and returns the
// manual action shared by the native and frontend menus.
func RegisterUpdateChecks(wailsApp *application.App, executablePath string, initErr error, coordinator *updateCoordinator) func() {
	info := updateInfoFor(runtime.GOOS, runtime.GOARCH, executablePath, buildinfo.VersionValue(), initErr)
	var check func(context.Context) (*updater.Release, error)
	switch info.Availability {
	case "supported":
		check = wailsApp.Updater.Check
	case "package-manager":
		// This provider only resolves releases. It is never registered with an
		// Updater, so Linux cannot download or replace package-managed files.
		provider, err := github.New(github.Config{
			Repository:   githubRepository,
			AssetMatcher: linuxReleaseAssetMatcher,
		})
		if err != nil {
			log.Printf("Configure release notifications: %v", err)
			break
		}
		check = func(ctx context.Context) (*updater.Release, error) {
			return provider.Check(ctx, updater.CheckRequest{
				CurrentVersion: info.Version,
				Platform:       runtime.GOOS,
				Arch:           runtime.GOARCH,
			})
		}
	}
	if coordinator == nil {
		coordinator = &updateCoordinator{}
	}

	run := func(automatic bool) {
		result := UpdateCheckResult{Automatic: automatic}
		if check != nil {
			timeout := startupUpdateTimeout
			if !automatic {
				timeout = 10 * time.Minute
			}
			release, started, err := checkUpdateRelease(wailsApp.Updater, coordinator, func(ctx context.Context) (*updater.Release, error) {
				if !automatic {
					wailsApp.Event.Emit(EventUpdateCheckStarted)
				}
				return check(ctx)
			}, timeout)
			result.Checked = started
			if err != nil {
				result.Error = err.Error()
				log.Printf("Check for updates: %v", err)
			} else if release != nil {
				result.Version = release.Version
				result.ReleaseURL, _ = release.Metadata["github.release.htmlURL"].(string)
			}
		}
		wailsApp.Event.Emit(EventUpdateCheckResult, result)
	}

	var startupOnce sync.Once
	wailsApp.Event.On(EventStartupUpdateCheck, func(*application.CustomEvent) {
		startupOnce.Do(func() { go run(true) })
	})
	return func() {
		wailsApp.Event.Emit(EventMenuSettings)
		if check != nil {
			go run(false)
		}
	}
}

func checkUpdateRelease(wailsUpdater *updater.Updater, coordinator *updateCoordinator, check func(context.Context) (*updater.Release, error), timeout time.Duration) (*updater.Release, bool, error) {
	var release *updater.Release
	checked := false
	started, err := coordinator.runUpdate(func() error {
		if wailsUpdater != nil && wailsUpdater.State() == updater.StateReady {
			return nil
		}
		checked = true
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		var err error
		release, err = check(ctx)
		return err
	})
	if !started {
		return nil, false, errors.New("another update operation is in progress; try again shortly")
	}
	return release, checked, err
}

func linuxReleaseAssetMatcher(request updater.CheckRequest, assets []github.ReleaseAsset) int {
	if request.Platform != "linux" || request.Arch != "amd64" {
		return -1
	}
	for index, asset := range assets {
		if strings.HasPrefix(asset.Name, "sshbrowse_") && strings.HasSuffix(asset.Name, "_amd64.deb") ||
			strings.HasPrefix(asset.Name, "sshbrowse-") && strings.HasSuffix(asset.Name, ".x86_64.rpm") {
			return index
		}
	}
	return -1
}
