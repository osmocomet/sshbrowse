//go:build windows

package app

import (
	"errors"
	"math"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

func (m *SidebarMenus) show(items []sidebarMenuItem, data string, x, y float64) error {
	if !isFiniteCoordinate(x) || !isFiniteCoordinate(y) {
		return errors.New("invalid sidebar menu position")
	}
	var selected sidebarMenuItem
	var showErr error
	application.InvokeSync(func() {
		hwnd := w32.HWND(m.window.NativeWindow())
		menu := w32.NewPopupMenu()
		if menu == 0 {
			showErr = errors.New("create sidebar menu")
			return
		}
		defer w32.DestroyMenu(menu) // Menu lifetime ends after TrackPopupMenu returns.
		for index, item := range items {
			if item.event == "" {
				if !w32.AppendMenu(menu, w32.MF_SEPARATOR, 0, nil) {
					showErr = errors.New("append sidebar menu separator")
					return
				}
				continue
			}
			if !w32.AppendMenu(menu, w32.MF_STRING, uintptr(index+1), w32.MustStringToUTF16Ptr(item.label)) {
				showErr = errors.New("append sidebar menu item")
				return
			}
		}
		// DOM client coordinates are CSS pixels. Win32 popup coordinates are
		// physical screen pixels, including on a scaled display.
		dpi := w32.GetDpiForWindow(hwnd)
		if dpi == 0 {
			showErr = errors.New("read sidebar menu DPI")
			return
		}
		scale := float64(dpi) / 96
		screenX, screenY := w32.ClientToScreen(hwnd, int(math.Round(x*scale)), int(math.Round(y*scale)))
		w32.SetForegroundWindow(hwnd) // The click already foregrounded this window.
		command := w32.TrackPopupMenuCommand(menu, w32.TPM_LEFTALIGN|w32.TPM_TOPALIGN, int32(screenX), int32(screenY), hwnd, nil)
		if command > 0 && int(command) <= len(items) {
			selected = items[command-1]
		}
		w32.PostMessage(hwnd, w32.WM_NULL, 0, 0) // Win32 recommends this after dismissing a popup menu.
	})
	if showErr != nil {
		return showErr
	}
	if selected.event != "" {
		m.app.Event.Emit(selected.event, data)
	}
	return nil
}

func isFiniteCoordinate(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
