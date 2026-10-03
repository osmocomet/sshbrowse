package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestStartupUpdateCheckHasDeadlineAndDoesNotDownload(t *testing.T) {
	var downloads int
	provider := staticUpdateProvider{
		release:       &updater.Release{Version: "2.0.0", Metadata: map[string]any{"github.release.htmlURL": "https://github.com/osmocomet/sshbrowse/releases/tag/v2.0.0"}},
		downloadCalls: &downloads,
	}
	host := &testUpdaterHost{}
	wailsUpdater := updater.New(host)
	if err := wailsUpdater.Init(updater.Config{CurrentVersion: "1.0.0", Providers: []updater.Provider{provider}, Window: updater.WindowNone}); err != nil {
		t.Fatal(err)
	}
	release, started, err := checkUpdateRelease(wailsUpdater, &updateCoordinator{}, func(ctx context.Context) (*updater.Release, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > startupUpdateTimeout {
			t.Fatal("startup check has no bounded deadline")
		}
		return wailsUpdater.Check(ctx)
	}, startupUpdateTimeout)
	if err != nil || !started || release == nil || release.Version != "2.0.0" || downloads != 0 || wailsUpdater.State() != updater.StateAvailable || host.openCalls != 0 {
		t.Fatalf("check = (%v, %v, %v), downloads = %d, state = %s, windows = %d", release, started, err, downloads, wailsUpdater.State(), host.openCalls)
	}
}

func TestStartupUpdateCheckCancellationReleasesCoordinator(t *testing.T) {
	coordinator := &updateCoordinator{}
	_, started, err := checkUpdateRelease(nil, coordinator, func(ctx context.Context) (*updater.Release, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, time.Millisecond)
	if !started || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("check started = %v, error = %v", started, err)
	}
	started, err = coordinator.runUpdate(func() error { return nil })
	if !started || err != nil {
		t.Fatalf("check cancellation left coordinator busy: %v, %v", started, err)
	}
}

func TestManualCheckDuringStartupReportsBusyAndCanRetry(t *testing.T) {
	coordinator := &updateCoordinator{}
	var checks int
	check := func(context.Context) (*updater.Release, error) {
		checks++
		return &updater.Release{Version: "2.0.0"}, nil
	}
	coordinator.runUpdate(func() error {
		// The startup operation holds the same coordinator as a manual check.
		release, checked, err := checkUpdateRelease(nil, coordinator, check, startupUpdateTimeout)
		if release != nil || checked || err == nil || !strings.Contains(err.Error(), "in progress") || checks != 0 {
			t.Fatalf("overlapping manual check = (%v, %v, %v), calls = %d; want explicit busy feedback", release, checked, err, checks)
		}
		return errors.New("startup check failed")
	})
	release, checked, err := checkUpdateRelease(nil, coordinator, check, startupUpdateTimeout)
	if err != nil || !checked || release == nil || release.Version != "2.0.0" || checks != 1 {
		t.Fatalf("manual retry = (%v, %v, %v), calls = %d", release, checked, err, checks)
	}
}

func TestLinuxReleaseCheckOnlyFetchesMetadata(t *testing.T) {
	var requests int
	client := &http.Client{Transport: releaseTestTransport(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Path != "/repos/osmocomet/sshbrowse/releases/latest" {
			t.Errorf("unexpected download request: %s", request.URL.Path)
			return nil, errors.New("unexpected download request")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v2.0.0","html_url":"https://github.com/osmocomet/sshbrowse/releases/tag/v2.0.0","assets":[{"name":"sshbrowse_2.0.0_amd64.deb","browser_download_url":"https://example.invalid/package.deb"}]}`)),
		}, nil
	})}
	provider, err := github.New(github.Config{Repository: githubRepository, HTTPClient: client, AssetMatcher: linuxReleaseAssetMatcher})
	if err != nil {
		t.Fatal(err)
	}
	release, started, err := checkUpdateRelease(nil, &updateCoordinator{}, func(ctx context.Context) (*updater.Release, error) {
		return provider.Check(ctx, updater.CheckRequest{CurrentVersion: "1.0.0", Platform: "linux", Arch: "amd64"})
	}, startupUpdateTimeout)
	if err != nil || !started || release == nil || requests != 1 || release.Metadata["github.release.htmlURL"] != "https://github.com/osmocomet/sshbrowse/releases/tag/v2.0.0" {
		t.Fatalf("Linux check = (%v, %v, %v), requests = %d", release, started, err, requests)
	}
}

type releaseTestTransport func(*http.Request) (*http.Response, error)

func (transport releaseTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestLinuxReleaseAssetMatcherRequiresNativePackage(t *testing.T) {
	for _, name := range []string{"sshbrowse_2.0.0_amd64.deb", "sshbrowse-2.0.0.x86_64.rpm"} {
		assets := []github.ReleaseAsset{{Name: "SHA256SUMS"}, {Name: name}}
		if linuxReleaseAssetMatcher(updater.CheckRequest{Platform: "linux", Arch: "amd64"}, assets) != 1 {
			t.Fatalf("did not select %s", name)
		}
		if linuxReleaseAssetMatcher(updater.CheckRequest{Platform: "linux", Arch: "arm64"}, assets) != -1 {
			t.Fatal("selected a package for an unsupported architecture")
		}
	}
	if linuxReleaseAssetMatcher(updater.CheckRequest{Platform: "linux", Arch: "amd64"}, []github.ReleaseAsset{{Name: "update.zip"}}) != -1 {
		t.Fatal("selected a non-package artifact")
	}
}
