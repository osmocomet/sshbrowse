package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"path/filepath"
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
	githubRepository       = "osmocomet/sshbrowse"
	EventUpdateInfoRequest = "update:info-request"
	EventUpdateInfo        = "update:info"
	EventUpdateDownload    = "update:download-request"
)

type UpdateInfo struct {
	Version      string `json:"version"`
	Availability string `json:"availability"`
	Message      string `json:"message"`
}

func updateInfoFor(platform, arch, executablePath, version string, initErr error) UpdateInfo {
	info := UpdateInfo{Version: version}
	switch {
	case !isStableReleaseVersion(version):
		info.Availability = "development"
		info.Message = "In-app updates are unavailable for development builds."
	case platform == "linux":
		info.Availability = "package-manager"
		info.Message = "Updates for DEB and RPM installations are handled by your package manager."
	case !supportsUpdaterFor(platform, arch, executablePath, version):
		info.Availability = "unsupported"
		info.Message = "In-app updates are unavailable for this installation."
	case initErr != nil:
		info.Availability = "unavailable"
		info.Message = "The updater could not start: " + initErr.Error()
	default:
		info.Availability = "supported"
		info.Message = "Check GitHub Releases for a newer version."
	}
	return info
}

func RegisterUpdateInfo(wailsApp *application.App, executablePath string, initErr error) {
	info := updateInfoFor(runtime.GOOS, runtime.GOARCH, executablePath, buildinfo.VersionValue(), initErr)
	wailsApp.Event.On(EventUpdateInfoRequest, func(*application.CustomEvent) {
		wailsApp.Event.Emit(EventUpdateInfo, info)
	})
}

// ConfigureUpdater enables the first-party Wails updater only for artifacts
// that can be replaced safely in place. Package-manager installations are not
// configured, so they never overwrite files owned by a package manager.
func ConfigureUpdater(wailsApp *application.App, executablePath string, quitGuard *QuitGuard) (bool, *updateCoordinator, error) {
	if !supportsUpdater(executablePath) {
		return false, nil, nil
	}

	githubProvider, err := github.New(github.Config{
		Repository:    githubRepository,
		AssetMatcher:  releaseAssetMatcher,
		ChecksumAsset: "SHA256SUMS",
	})
	if err != nil {
		return false, nil, err
	}

	coordinator := &updateCoordinator{}
	host := newGuardedUpdaterHost(wailsApp, quitGuard, coordinator)
	wailsUpdater := updater.New(host)
	host.restartUpdate = wailsUpdater.Restart
	provider := checksumRequiredProvider{provider: githubProvider}
	if err := wailsUpdater.Init(updater.Config{
		CurrentVersion: buildinfo.VersionValue(),
		Providers:      []updater.Provider{provider},
		Window:         updater.WindowNone,
	}); err != nil {
		return false, nil, err
	}
	wailsApp.Updater = wailsUpdater
	wailsApp.Event.On(EventUpdateDownload, func(*application.CustomEvent) {
		go runUpdateDownload(wailsApp, coordinator)
	})
	return true, coordinator, nil
}

// NewUpdateAction returns the single user-triggered update action used by
// native menus and the frontend menu.
func NewUpdateAction(wailsApp *application.App, supported bool, coordinator *updateCoordinator) func() {
	return func() {
		wailsApp.Event.Emit(EventMenuSettings)
		if !supported {
			return
		}

		go runUpdateCheck(wailsApp, coordinator)
	}
}

func runUpdateCheck(wailsApp *application.App, coordinator *updateCoordinator) {
	if coordinator == nil {
		return
	}
	started, err := coordinator.runUpdate(func() error {
		if wailsApp.Updater.State() == updater.StateReady {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_, err := wailsApp.Updater.Check(ctx)
		return err
	})
	if started && err != nil {
		log.Printf("Check for updates: %v", err)
	}
}

func runUpdateDownload(wailsApp *application.App, coordinator *updateCoordinator) {
	if coordinator == nil {
		return
	}
	started, err := coordinator.runUpdate(func() error {
		if wailsApp.Updater.State() != updater.StateAvailable {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		return wailsApp.Updater.DownloadAndInstall(ctx)
	})
	if started && err != nil {
		log.Printf("Download update: %v", err)
	}
}

// checksumRequiredProvider rejects releases for which the GitHub provider did
// not find a usable checksum. Wails then verifies the downloaded bytes against
// this digest before it can stage the update.
type checksumRequiredProvider struct {
	provider updater.Provider
}

func (p checksumRequiredProvider) Name() string { return p.provider.Name() }

func (p checksumRequiredProvider) Check(ctx context.Context, request updater.CheckRequest) (*updater.Release, error) {
	release, err := p.provider.Check(ctx, request)
	if err != nil || release == nil {
		return release, err
	}
	verification := release.Verification
	if verification == nil || verification.DigestAlgo != "sha256" || len(verification.Digest) != sha256.Size {
		return nil, fmt.Errorf("update %s has no valid SHA-256 checksum", release.Artifact.Filename)
	}
	return release, nil
}

func (p checksumRequiredProvider) Download(ctx context.Context, release *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return p.provider.Download(ctx, release, dst, onProgress)
}

// updateCoordinator prevents a new staging operation from overlapping a
// restart helper, and admits only one helper at a time.
type updateCoordinator struct {
	mu             sync.Mutex
	updateRunning  bool
	restartRunning bool
	restartPending bool
}

func (c *updateCoordinator) runUpdate(update func() error) (bool, error) {
	c.mu.Lock()
	if c.updateRunning || c.restartRunning || c.restartPending {
		c.mu.Unlock()
		return false, nil
	}
	c.updateRunning = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.updateRunning = false
		c.mu.Unlock()
	}()
	return true, update()
}

func (c *updateCoordinator) restart(confirm func(func(bool)), restart func() error, onFailure func(error)) bool {
	c.mu.Lock()
	if c.updateRunning || c.restartRunning || c.restartPending {
		c.mu.Unlock()
		return false
	}
	c.restartRunning = true
	c.mu.Unlock()

	var decision sync.Once
	confirm(func(confirmed bool) {
		decision.Do(func() {
			if !confirmed {
				c.finishRestart(false)
				return
			}
			go func() {
				if err := restart(); err != nil {
					if onFailure != nil {
						onFailure(err)
					}
					c.finishRestart(false)
					return
				}
				c.finishRestart(true)
			}()
		})
	})
	return true
}

func (c *updateCoordinator) finishRestart(succeeded bool) {
	c.mu.Lock()
	c.restartRunning = false
	c.restartPending = succeeded
	c.mu.Unlock()
}

func supportsUpdater(executablePath string) bool {
	return supportsUpdaterFor(runtime.GOOS, runtime.GOARCH, executablePath, buildinfo.VersionValue())
}

func supportsUpdaterFor(platform, arch, executablePath, version string) bool {
	if !isStableReleaseVersion(version) {
		return false
	}

	switch platform {
	case "darwin":
		if arch != "amd64" && arch != "arm64" {
			return false
		}
		return strings.Contains(filepath.ToSlash(filepath.Clean(executablePath)), ".app/Contents/MacOS/")
	case "windows":
		if arch != "amd64" {
			return false
		}
		// Release installers use a per-user NSIS scope, so the installed EXE is
		// writable by the user and can be safely replaced by Wails' helper.
		return true
	case "linux":
		return false
	default:
		return false
	}
}

func isStableReleaseVersion(version string) bool {
	if version == "" || version == "0.0.0-dev" {
		return false
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func releaseAssetMatcher(request updater.CheckRequest, assets []github.ReleaseAsset) int {
	var expected string
	switch {
	case request.Platform == "darwin" && (request.Arch == "amd64" || request.Arch == "arm64"):
		expected = "SSHBrowse-macOS-universal.zip"
	case request.Platform == "windows" && request.Arch == "amd64":
		expected = "SSHBrowse-Windows-x64-update.zip"
	default:
		return -1
	}

	for index, asset := range assets {
		if asset.Name == expected {
			return index
		}
	}
	return -1
}
