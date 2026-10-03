package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestSupportsUpdaterRequiresStableReleaseVersion(t *testing.T) {
	installedMacPath := "/Applications/sshbrowse.app/Contents/MacOS/sshbrowse"
	if supportsUpdaterFor("darwin", "arm64", installedMacPath, "0.0.0-dev") {
		t.Fatal("development macOS builds must not initialize the updater")
	}
	if !supportsUpdaterFor("darwin", "arm64", installedMacPath, "0.1.0") {
		t.Fatal("a release macOS app should support updates")
	}
	if !supportsUpdaterFor("windows", "amd64", `C:\Users\test\AppData\Local\Programs\SSHBrowse\sshbrowse.exe`, "0.1.0") {
		t.Fatal("a release Windows build should support updates")
	}
}

func TestSupportsUpdaterPlatformAndArchitecture(t *testing.T) {
	if supportsUpdaterFor("linux", "amd64", "/opt/sshbrowse", "0.1.0") {
		t.Fatal("Linux self-update must remain disabled")
	}
	if supportsUpdaterFor("darwin", "386", "/Applications/sshbrowse.app/Contents/MacOS/sshbrowse", "0.1.0") {
		t.Fatal("unsupported macOS architectures must not support updates")
	}
	if supportsUpdaterFor("windows", "arm64", `C:\Programs\sshbrowse.exe`, "0.1.0") {
		t.Fatal("unsupported Windows architectures must not support updates")
	}
	if supportsUpdaterFor("freebsd", "amd64", "/opt/sshbrowse", "0.1.0") {
		t.Fatal("unsupported platforms must not support updates")
	}
}

func TestUpdateInfoDistinguishesDevelopmentAndPackageManagerBuilds(t *testing.T) {
	macPath := "/Applications/sshbrowse.app/Contents/MacOS/sshbrowse"
	tests := []struct {
		platform, arch, path, version, availability, message string
	}{
		{"darwin", "arm64", macPath, "0.0.0-dev", "development", "development builds"},
		{"linux", "amd64", "/usr/bin/sshbrowse", "0.0.0-dev", "development", "development builds"},
		{"linux", "amd64", "/usr/bin/sshbrowse", "0.1.0", "package-manager", "package manager"},
		{"darwin", "arm64", macPath, "0.1.0", "supported", "Check GitHub"},
		{"windows", "amd64", `C:\Users\test\sshbrowse.exe`, "0.1.0", "supported", "Check GitHub"},
		{"darwin", "arm64", "/tmp/sshbrowse", "0.1.0", "unsupported", "unavailable"},
	}
	for _, test := range tests {
		info := updateInfoFor(test.platform, test.arch, test.path, test.version, nil)
		if info.Version != test.version || info.Availability != test.availability || !strings.Contains(info.Message, test.message) {
			t.Errorf("updateInfoFor(%s, %s, %s, %s) = %#v", test.platform, test.arch, test.path, test.version, info)
		}
	}
}

func TestReleaseAssetMatcherUsesUpdaterArchives(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "SHA256SUMS"},
		{Name: "SSHBrowse-Windows-x64-Setup.exe"},
		{Name: "SSHBrowse-Windows-x64-update.zip"},
	}

	if got := releaseAssetMatcher(updater.CheckRequest{Platform: "windows", Arch: "amd64"}, assets); got != 2 {
		t.Fatalf("Windows updater asset index = %d, want 2", got)
	}
	if got := releaseAssetMatcher(updater.CheckRequest{Platform: "windows", Arch: "arm64"}, assets); got != -1 {
		t.Fatalf("Windows ARM64 updater asset index = %d, want -1", got)
	}
}

func TestReleaseAssetMatcherUsesUniversalMacArchive(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "SSHBrowse-macOS-universal.dmg"},
		{Name: "SSHBrowse-macOS-universal.zip"},
	}

	for _, arch := range []string{"amd64", "arm64"} {
		if got := releaseAssetMatcher(updater.CheckRequest{Platform: "darwin", Arch: arch}, assets); got != 1 {
			t.Fatalf("macOS %s updater asset index = %d, want 1", arch, got)
		}
	}
}

func TestReleaseAssetMatcherDoesNotSelectInstallersOrLinuxPackages(t *testing.T) {
	assets := []github.ReleaseAsset{
		{Name: "sshbrowse_0.1.0_amd64.deb"},
		{Name: "sshbrowse-0.1.0.x86_64.rpm"},
	}

	if got := releaseAssetMatcher(updater.CheckRequest{Platform: "linux", Arch: "amd64"}, assets); got != -1 {
		t.Fatalf("Linux updater asset index = %d, want -1", got)
	}
}

func TestChecksumRequiredProviderRejectsMissingOrMalformedDigest(t *testing.T) {
	for _, verification := range []*updater.Verification{
		nil,
		{DigestAlgo: "sha512", Digest: make([]byte, sha256.Size)},
		{DigestAlgo: "sha256", Digest: make([]byte, sha256.Size-1)},
	} {
		provider := checksumRequiredProvider{provider: staticUpdateProvider{release: &updater.Release{
			Artifact:     updater.Artifact{Filename: "update.zip"},
			Verification: verification,
		}}}
		if release, err := provider.Check(context.Background(), updater.CheckRequest{}); err == nil || release != nil {
			t.Fatalf("Check = (%#v, %v), want checksum rejection", release, err)
		}
	}
}

func TestChecksumRequiredProviderAcceptsValidSHA256Digest(t *testing.T) {
	digest := sha256.Sum256([]byte("valid update"))
	release := &updater.Release{
		Artifact: updater.Artifact{Filename: "update.zip"},
		Verification: &updater.Verification{
			DigestAlgo: "sha256",
			Digest:     digest[:],
		},
	}
	provider := checksumRequiredProvider{provider: staticUpdateProvider{release: release}}
	got, err := provider.Check(context.Background(), updater.CheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got != release {
		t.Fatalf("Check release = %#v, want original verified release", got)
	}
}

func TestUpdaterRejectsArtifactWhoseSHA256DoesNotMatch(t *testing.T) {
	wantedDigest := sha256.Sum256([]byte("expected bytes"))
	provider := checksumRequiredProvider{provider: staticUpdateProvider{
		release: &updater.Release{
			Version:  "2.0.0",
			Artifact: updater.Artifact{Filename: "update.bin"},
			Verification: &updater.Verification{
				DigestAlgo: "sha256",
				Digest:     wantedDigest[:],
			},
		},
		content: []byte("different bytes"),
	}}
	host := &testUpdaterHost{}
	wailsUpdater := updater.New(host)
	if err := wailsUpdater.Init(updater.Config{CurrentVersion: "1.0.0", Providers: []updater.Provider{provider}, Window: updater.WindowNone}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := wailsUpdater.CheckAndInstall(context.Background()); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("CheckAndInstall error = %v, want digest mismatch", err)
	}
	if wailsUpdater.State() == updater.StateReady || host.hasEvent(updater.EventUpdateReady) {
		t.Fatal("unverified update was staged")
	}
}

func TestWindowlessCheckEmitsNoUpdateInline(t *testing.T) {
	host := &testUpdaterHost{}
	wailsUpdater := updater.New(host)
	if err := wailsUpdater.Init(updater.Config{
		CurrentVersion: "1.0.0",
		Providers:      []updater.Provider{staticUpdateProvider{}},
		Window:         updater.WindowNone,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	runUpdateCheck(&application.App{Updater: wailsUpdater}, &updateCoordinator{})
	if host.openCalls != 0 || !host.hasEvent(updater.EventNoUpdate) {
		t.Fatalf("open window calls = %d, events = %v; want no window and inline no-update event", host.openCalls, host.events)
	}
}

func TestWindowlessCheckWaitsForExplicitDownload(t *testing.T) {
	content := []byte("verified update")
	digest := sha256.Sum256(content)
	var checks, downloads int
	host := &testUpdaterHost{}
	wailsUpdater := updater.New(host)
	provider := checksumRequiredProvider{provider: staticUpdateProvider{
		release: &updater.Release{
			Version:  "2.0.0",
			Artifact: updater.Artifact{Filename: "update.bin"},
			Verification: &updater.Verification{
				DigestAlgo: "sha256",
				Digest:     digest[:],
			},
		},
		content:       content,
		checkCalls:    &checks,
		downloadCalls: &downloads,
	}}
	if err := wailsUpdater.Init(updater.Config{
		CurrentVersion: "1.0.0",
		Providers:      []updater.Provider{provider},
		Window:         updater.WindowNone,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wailsApp := &application.App{Updater: wailsUpdater}
	coordinator := &updateCoordinator{}
	runUpdateCheck(wailsApp, coordinator)
	if checks != 1 || downloads != 0 || wailsUpdater.State() != updater.StateAvailable || !host.hasEvent(updater.EventUpdateAvailable) {
		t.Fatalf("check state = %s, checks = %d, downloads = %d, events = %v; want available without download", wailsUpdater.State(), checks, downloads, host.events)
	}
	runUpdateDownload(wailsApp, coordinator)
	staged := wailsUpdater.DownloadedPath()
	if staged == "" || wailsUpdater.State() != updater.StateReady {
		t.Fatalf("staged path = %q, state = %s; want ready update", staged, wailsUpdater.State())
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(filepath.Dir(staged)); err != nil {
			t.Errorf("remove staged update: %v", err)
		}
	})
	got, err := os.ReadFile(staged)
	if err != nil || string(got) != string(content) {
		t.Fatalf("staged contents = %q, error = %v", got, err)
	}
	if host.openCalls != 0 || !host.hasEvent(updater.EventUpdateAvailable) || !host.hasEvent(updater.EventUpdateReady) {
		t.Fatalf("open window calls = %d, events = %v; want inline available and ready events", host.openCalls, host.events)
	}
	runUpdateCheck(wailsApp, coordinator)
	runUpdateDownload(wailsApp, coordinator)
	if checks != 1 || downloads != 1 || wailsUpdater.DownloadedPath() != staged {
		t.Fatalf("repeat check after ready changed staging: checks = %d, downloads = %d, path = %q", checks, downloads, wailsUpdater.DownloadedPath())
	}
}

func TestFailedCheckCanRetryWithoutDownloading(t *testing.T) {
	checkError := errors.New("network unavailable")
	var checks, downloads int
	host := &testUpdaterHost{}
	wailsUpdater := updater.New(host)
	provider := staticUpdateProvider{
		release:       &updater.Release{Version: "2.0.0"},
		checkError:    &checkError,
		checkCalls:    &checks,
		downloadCalls: &downloads,
	}
	if err := wailsUpdater.Init(updater.Config{CurrentVersion: "1.0.0", Providers: []updater.Provider{provider}, Window: updater.WindowNone}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	wailsApp := &application.App{Updater: wailsUpdater}
	coordinator := &updateCoordinator{}
	runUpdateCheck(wailsApp, coordinator)
	if checks != 1 || downloads != 0 || !host.hasEvent(updater.EventError) {
		t.Fatalf("failed check: checks = %d, downloads = %d, events = %v", checks, downloads, host.events)
	}
	runUpdateDownload(wailsApp, coordinator)
	if downloads != 0 {
		t.Fatal("failed check allowed a download")
	}
	checkError = nil
	runUpdateCheck(wailsApp, coordinator)
	if checks != 2 || downloads != 0 || wailsUpdater.State() != updater.StateAvailable {
		t.Fatalf("retry: state = %s, checks = %d, downloads = %d; want available without download", wailsUpdater.State(), checks, downloads)
	}
}

type staticUpdateProvider struct {
	release       *updater.Release
	content       []byte
	checkError    *error
	checkCalls    *int
	downloadCalls *int
}

func (p staticUpdateProvider) Name() string { return "test" }

func (p staticUpdateProvider) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	if p.checkCalls != nil {
		(*p.checkCalls)++
	}
	if p.checkError != nil && *p.checkError != nil {
		return nil, *p.checkError
	}
	return p.release, nil
}

func (p staticUpdateProvider) Download(_ context.Context, _ *updater.Release, destination io.Writer, _ func(int64, int64)) error {
	if p.downloadCalls != nil {
		(*p.downloadCalls)++
	}
	_, err := destination.Write(p.content)
	return err
}

type testUpdaterHost struct {
	events    []string
	openCalls int
}

func (h *testUpdaterHost) Emit(name string, _ ...any) bool {
	h.events = append(h.events, name)
	return true
}
func (*testUpdaterHost) OnEvent(string, func(any)) func() { return func() {} }
func (h *testUpdaterHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	h.openCalls++
	return testUpdaterWindow{}
}
func (*testUpdaterHost) Quit() {}

func (h *testUpdaterHost) hasEvent(name string) bool {
	for _, event := range h.events {
		if event == name {
			return true
		}
	}
	return false
}

type testUpdaterWindow struct{}

func (testUpdaterWindow) EmitEvent(string, ...any) bool { return false }
func (testUpdaterWindow) Show()                         {}
func (testUpdaterWindow) Close()                        {}
