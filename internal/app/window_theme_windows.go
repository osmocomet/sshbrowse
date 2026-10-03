package app

import (
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// Wails applies CustomTheme at creation. Update the native caption when the
// web view changes appearance, and restore the right text colour on focus.
func registerWindowsChromeUpdates(wailsApp *application.App, window *application.WebviewWindow) {
	var selectedColours atomic.Value
	selectedColours.Store(windowsChromePalettes["classic"])
	apply := func(active bool) {
		colours := selectedColours.Load().(windowsChromeColours)
		application.InvokeAsync(func() {
			if w32.IsCurrentlyHighContrastMode() {
				return
			}
			hwnd := uintptr(window.NativeWindow())
			if hwnd == 0 {
				return
			}
			w32.SetTitleBarColour(hwnd, colours.background)
			w32.SetBorderColour(hwnd, colours.border)
			titleText := colours.inactiveText
			if active {
				titleText = colours.text
			}
			w32.SetTitleTextColour(hwnd, titleText)
		})
	}
	wailsApp.Event.On(EventWindowAppearanceTheme, func(event *application.CustomEvent) {
		name, ok := event.Data.(string)
		if !ok {
			return
		}
		selected, ok := windowsChromePalettes[name]
		if !ok {
			return
		}
		selectedColours.Store(selected)
		apply(window.IsFocused())
	})
	window.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) { apply(true) })
	window.OnWindowEvent(events.Common.WindowLostFocus, func(*application.WindowEvent) { apply(false) })
}
