package app

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"sshbrowse/internal/atomicfile"
)

const (
	EventWindowFilesDropped    = "window:filesDropped"
	EventWindowAppearanceTheme = "window:appearanceTheme"
)

var windowFrameWriteMutex sync.Mutex

type windowsChromeColours struct {
	background   uint32
	text         uint32
	inactiveText uint32
	border       uint32
}

// Win32 COLORREF values (0x00BBGGRR), matched to app.css chrome, chrome-foreground,
// text-muted, and chrome-border roles.
var windowsChromePalettes = map[string]windowsChromeColours{
	"warm":     {0x1f2324, 0xcfd9de, 0x8a979e, 0x2e3334},
	"classic":  {0x262220, 0xe1ded9, 0xa09992, 0x3e3935},
	"moss":     {0x232522, 0xdfe6e3, 0x9aa799, 0x343a30},
	"fjord":    {0x2c2616, 0xe9ebdc, 0xa1a588, 0x474029},
	"oled":     {0x000000, 0xdfe5e4, 0x939b98, 0x181917},
	"contrast": {0x090909, 0xffffff, 0xd0d0d0, 0x707070},
}

const (
	defaultWindowWidth    = 1200
	defaultWindowHeight   = 800
	preferredWindowWidth  = 900
	preferredWindowHeight = 600
	minimumWindowWidth    = 640
	minimumWindowHeight   = 480
)

type FileDropEvent struct {
	Files    []string `json:"files"`
	TargetID string   `json:"targetId"`
}

// NewMainWindow keeps geometry separate from connection data. Only the last
// normal frame is restored; maximised, fullscreen and minimised states stay transient.
func NewMainWindow(wailsApp *application.App, statePath string, menu *application.Menu) *application.WebviewWindow {
	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "SSHBrowse", URL: "/",
		Width: defaultWindowWidth, Height: defaultWindowHeight,
		MinWidth: minimumWindowWidth, MinHeight: minimumWindowHeight,
		Hidden:         true,
		EnableFileDrop: true,
		Frameless:      runtime.GOOS == "linux",
		// Keep WebKit's normal clipboard/input context menu behavior; the
		// sidebar opts into Wails menus through its custom CSS variables.
		DefaultContextMenuDisabled: false,
		UseApplicationMenu:         runtime.GOOS == "darwin",
		Linux: application.LinuxWindow{
			Menu: menu,
		},
		BackgroundColour: application.NewRGB(27, 28, 30),
		Windows: application.WindowsWindow{
			Theme:       application.Dark,
			CustomTheme: windowsChromeTheme(),
			DisableMenu: true,
		},
		Mac: application.MacWindow{
			Appearance: application.NSAppearanceNameDarkAqua,
			// No title text; the web content reaches the top edge and leaves
			// room for the traffic lights itself.
			TitleBar: application.MacTitleBarHidden,
		},
	})
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		details := event.Context().DropTargetDetails()
		if details == nil || details.ElementID == "" {
			return
		}
		wailsApp.Event.Emit(EventWindowFilesDropped, FileDropEvent{
			Files:    event.Context().DroppedFiles(),
			TargetID: details.ElementID,
		})
	})
	if runtime.GOOS == "windows" {
		registerWindowsChromeUpdates(wailsApp, window)
	}
	var mutex sync.Mutex
	var normalFrame application.Rect
	var ready bool
	var initialise sync.Once
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		initialise.Do(func() {
			frame := readWindowFrame(statePath)
			screen, err := window.GetScreen()
			if err != nil {
				log.Printf("Read window screen: %v", err)
			}
			if runtime.GOOS == "linux" {
				// Wayland owns toplevel positioning. Restore the useful part of the
				// frame—the size—and let the compositor place the frameless window.
				if screen != nil {
					size := fitWindowSize(frame, screen.WorkArea)
					window.SetSize(size.Width, size.Height)
				} else if frame.Width >= minimumWindowWidth && frame.Height >= minimumWindowHeight {
					window.SetSize(frame.Width, frame.Height)
				}
			} else if screen != nil {
				// Retain a saved display when it still exists; otherwise recover
				// onto the current display rather than opening off-screen.
				for _, candidate := range wailsApp.Screen.GetAll() {
					if candidate.WorkArea.Contains(frame.Origin()) {
						screen = candidate
						break
					}
				}
				frame = fitWindowFrame(frame, screen.WorkArea)
				window.SetSize(frame.Width, frame.Height)
				window.SetPosition(frame.X, frame.Y)
			}
			mutex.Lock()
			normalFrame = window.Bounds()
			ready = true
			mutex.Unlock()
			window.Show()
			if runtime.GOOS == "linux" {
				// GTK presents the window without relying on a Wayland position;
				// explicitly focus it so the first terminal receives input.
				window.Focus()
			}
		})
	})
	remember := func(*application.WindowEvent) {
		if window.IsFullscreen() || window.IsMaximised() || window.IsMinimised() {
			return
		}
		frame := window.Bounds()
		mutex.Lock()
		defer mutex.Unlock()
		if ready && frame.Width >= minimumWindowWidth && frame.Height >= minimumWindowHeight {
			normalFrame = frame
		}
	}
	window.OnWindowEvent(events.Common.WindowDidMove, remember)
	window.OnWindowEvent(events.Common.WindowDidResize, remember)
	saveFrame := func() {
		mutex.Lock()
		frame := normalFrame
		mutex.Unlock()
		if frame.IsEmpty() {
			return
		}
		if err := writeWindowFrame(statePath, frame); err != nil {
			log.Printf("Save window frame: %v", err)
		}
	}
	if runtime.GOOS == "windows" {
		// Closing the only native window can end the Windows message loop
		// without running the application shutdown callbacks.
		window.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
			remember(nil)
			saveFrame()
		})
	}
	wailsApp.OnShutdown(saveFrame)
	return window
}

func windowsChromeTheme() application.ThemeSettings {
	colours := windowsChromePalettes["classic"]

	return application.ThemeSettings{
		DarkModeActive: &application.WindowTheme{
			TitleBarColour:  &colours.background,
			TitleTextColour: &colours.text,
			BorderColour:    &colours.border,
		},
		DarkModeInactive: &application.WindowTheme{
			TitleBarColour:  &colours.background,
			TitleTextColour: &colours.inactiveText,
			BorderColour:    &colours.border,
		},
	}
}

func fitWindowFrame(frame, area application.Rect) application.Rect {
	if frame.Width < minimumWindowWidth || frame.Height < minimumWindowHeight {
		frame.Width = min(defaultWindowWidth, area.Width*85/100)
		frame.Height = min(defaultWindowHeight, area.Height*85/100)
		if area.Width >= preferredWindowWidth {
			frame.Width = max(preferredWindowWidth, frame.Width)
		}
		if area.Height >= preferredWindowHeight {
			frame.Height = max(preferredWindowHeight, frame.Height)
		}
		frame.X = area.X + (area.Width-frame.Width)/2
		frame.Y = area.Y + (area.Height-frame.Height)/2
	}
	frame.Width = min(max(minimumWindowWidth, frame.Width), area.Width)
	frame.Height = min(max(minimumWindowHeight, frame.Height), area.Height)
	frame.X = max(area.X, min(frame.X, area.X+area.Width-frame.Width))
	frame.Y = max(area.Y, min(frame.Y, area.Y+area.Height-frame.Height))
	return frame
}

func fitWindowSize(frame, area application.Rect) application.Rect {
	fitted := fitWindowFrame(frame, area)
	return application.Rect{Width: fitted.Width, Height: fitted.Height}
}

func readWindowFrame(path string) application.Rect {
	file, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("Read window frame: %v", err)
		}
		return application.Rect{}
	}
	defer file.Close() // Read-only file; close cannot affect saved state.
	var frame application.Rect
	if err := json.NewDecoder(io.LimitReader(file, 1024)).Decode(&frame); err != nil {
		log.Printf("Read window frame: %v", err)
		return application.Rect{}
	}
	return frame
}

func writeWindowFrame(path string, frame application.Rect) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	// Concurrent replacements of the same file can fail on Windows.
	windowFrameWriteMutex.Lock()
	defer windowFrameWriteMutex.Unlock()
	return atomicfile.WriteReplace(path, data, 0600)
}
