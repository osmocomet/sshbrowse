//go:build !windows

package app

import "github.com/wailsapp/wails/v3/pkg/application"

func registerWindowsChromeUpdates(*application.App, *application.WebviewWindow) {}
