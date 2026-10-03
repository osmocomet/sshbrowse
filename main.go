package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"sshbrowse/internal/app"
	"sshbrowse/internal/buildinfo"
	"sshbrowse/internal/profile"
)

// The production frontend build is embedded into the binary.
//
//go:embed all:frontend/dist
var assets embed.FS

func webviewUserDataPath() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	appData := os.Getenv("AppData")
	if appData == "" {
		return ""
	}
	return filepath.Join(appData, "sshbrowse.exe")
}

func main() {
	if handled, err := writeLicenseDocuments(os.Args, os.Stdout); handled {
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	setLinuxApplicationName("SSHBrowse")

	executablePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	instanceID := buildinfo.InstanceID(executablePath)
	storePath, err := profile.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	store := profile.NewStore(storePath)

	application.RegisterEvent[app.SessionData](app.EventSessionData)
	application.RegisterEvent[app.SessionExit](app.EventSessionExit)
	application.RegisterEvent[app.FileDropEvent](app.EventWindowFilesDropped)

	// The guard needs the app, and the app options need the guard; the
	// closure resolves that. Run starts after both exist.
	var previewLogger *slog.Logger
	if os.Getenv("SSHBROWSE_HEADLESS_PREVIEW") == "1" {
		previewLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	var quitGuard *app.QuitGuard
	var mainWindow *application.WebviewWindow
	wailsApp := application.New(application.Options{
		Logger:      previewLogger,
		Name:        "SSHBrowse",
		Description: buildinfo.Description(),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			ApplicationID: "io.github.osmocomet.sshbrowse",
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath: webviewUserDataPath(),
		},
		// The executable path is part of the identity so an installed app and
		// a development checkout can run together while sharing profile data.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: instanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if mainWindow == nil {
					return
				}
				mainWindow.Restore()
				mainWindow.Show()
				mainWindow.Focus()
			},
		},
		ShouldQuit: func() bool { return quitGuard.ShouldQuit() },
	})
	sessions := app.NewSessions(wailsApp, store)
	quitGuard = app.NewQuitGuard(wailsApp, sessions)
	wailsApp.RegisterService(application.NewService(sessions))
	wailsApp.RegisterService(application.NewService(app.NewConnections(store, wailsApp)))
	updatesSupported, updateCoordinator, updateErr := app.ConfigureUpdater(wailsApp, executablePath, quitGuard)
	if updateErr != nil {
		log.Printf("Configure updater: %v", updateErr)
	}
	app.RegisterUpdateInfo(wailsApp, executablePath, updateErr)
	menu := app.BuildMenu(wailsApp, app.NewUpdateAction(wailsApp, updatesSupported && updateErr == nil, updateCoordinator))
	if menu != nil {
		wailsApp.Menu.SetApplicationMenu(menu)
	}

	mainWindow = app.NewMainWindow(wailsApp, filepath.Join(filepath.Dir(storePath), "window.json"), menu)
	wailsApp.RegisterService(application.NewService(app.NewSidebarMenus(wailsApp, mainWindow)))
	quitGuard.GuardWindow(mainWindow)

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
