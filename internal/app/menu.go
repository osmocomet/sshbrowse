package app

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	EventMenuNewTab              = "menu:newTab"
	EventMenuNewLocalTerminal    = "menu:newLocalTerminal"
	EventMenuNewConnection       = "menu:newConnection"
	EventMenuImportSSHConfig     = "menu:importSSHConfig"
	EventMenuCloseTab            = "menu:closeTab"
	EventMenuFontIncrease        = "menu:fontIncrease"
	EventMenuFontDecrease        = "menu:fontDecrease"
	EventMenuFontReset           = "menu:fontReset"
	EventMenuSettings            = "menu:settings"
	EventMenuToggleSidebar       = "menu:toggleSidebar"
	EventMenuToggleTiling        = "menu:toggleTiling"
	EventMenuAbout               = "menu:about"
	EventMenuCheckForUpdates     = "menu:checkForUpdates"
	EventMenuTerminalCopy        = "menu:terminalCopy"
	EventMenuTerminalPaste       = "menu:terminalPaste"
	EventMenuConnectionOpen      = "menu:connectionOpen"
	EventMenuConnectionEdit      = "menu:connectionEdit"
	EventMenuConnectionDuplicate = "menu:connectionDuplicate"
	EventMenuConnectionDelete    = "menu:connectionDelete"
	EventMenuConnectionMove      = "menu:connectionMove"
	EventMenuConnectionNewFolder = "menu:connectionNewFolder"
	EventMenuNewSFTP             = "menu:newSFTP"
	EventMenuConnectionSFTP      = "menu:connectionSFTP"
	EventMenuFolderOpen          = "menu:folderOpen"
	EventMenuFolderNewConnection = "menu:folderNewConnection"
	EventMenuFolderCreate        = "menu:folderCreate"
	EventMenuFolderRename        = "menu:folderRename"
	EventMenuFolderDelete        = "menu:folderDelete"
)

// BuildMenu supplies the native menu on macOS. Linux and Windows render the
// application menu in the frontend; context menus remain native on all hosts.
func BuildMenu(wailsApp *application.App, updateActions ...func()) *application.Menu {
	wailsApp.Event.On(EventMenuAbout, func(*application.CustomEvent) {
		wailsApp.Menu.ShowAbout()
	})
	var checkForUpdates func()
	if len(updateActions) > 0 {
		checkForUpdates = updateActions[0]
	}
	if checkForUpdates != nil {
		wailsApp.Event.On(EventMenuCheckForUpdates, func(*application.CustomEvent) {
			checkForUpdates()
		})
	}
	if runtime.GOOS != "darwin" {
		registerContextMenus(wailsApp)
		return nil
	}

	menu := buildNativeMenu(wailsApp, checkForUpdates)
	registerContextMenus(wailsApp)
	return menu
}

func buildNativeMenu(wailsApp *application.App, checkForUpdates func()) *application.Menu {
	menu := application.NewMenu()
	appMenu := menu.AddSubmenu("SSHBrowse")
	appMenu.Add("About SSHBrowse").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuAbout)
	})
	if checkForUpdates != nil {
		appMenu.Add("Check for Updates…").OnClick(func(*application.Context) {
			checkForUpdates()
		})
	}
	appMenu.AddSeparator()
	appMenu.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuSettings)
	})
	appMenu.AddSeparator()
	appMenu.AddRole(application.ServicesMenu)
	appMenu.AddSeparator()
	appMenu.AddRole(application.Hide)
	appMenu.AddRole(application.HideOthers)
	appMenu.AddRole(application.UnHide)
	appMenu.AddSeparator()
	appMenu.AddRole(application.Quit)

	shell := menu.AddSubmenu("Shell")
	shell.Add("New Tab…").SetAccelerator("CmdOrCtrl+t").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuNewTab)
	})
	shell.Add("New Local Terminal").SetAccelerator("CmdOrCtrl+Shift+t").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuNewLocalTerminal)
	})
	shell.Add("New SFTP Tab").SetAccelerator("CmdOrCtrl+Shift+p").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuNewSFTP)
	})
	shell.Add("New Connection…").SetAccelerator("CmdOrCtrl+n").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuNewConnection)
	})
	shell.Add("Import from SSH Config…").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuImportSSHConfig)
	})
	shell.AddSeparator()
	shell.Add("Close Tab").SetAccelerator("CmdOrCtrl+w").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuCloseTab)
	})

	// Keep AppKit's standard editing roles for native text fields and menus.
	edit := menu.AddSubmenu("Edit")
	edit.AddRole(application.Undo)
	edit.AddRole(application.Redo)
	edit.AddSeparator()
	edit.AddRole(application.Cut)
	edit.AddRole(application.Copy)
	edit.AddRole(application.Paste)
	edit.AddRole(application.SelectAll)

	view := menu.AddSubmenu("View")
	view.Add("Toggle Sidebar").SetAccelerator("CmdOrCtrl+b").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuToggleSidebar)
	})
	view.Add("Open in tiles").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuToggleTiling)
	})
	view.AddSeparator()
	view.Add("Zoom In").SetAccelerator("CmdOrCtrl+plus").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuFontIncrease)
	})
	view.Add("Zoom Out").SetAccelerator("CmdOrCtrl+-").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuFontDecrease)
	})
	view.Add("Reset Zoom").SetAccelerator("CmdOrCtrl+0").OnClick(func(*application.Context) {
		wailsApp.Event.Emit(EventMenuFontReset)
	})
	view.AddSeparator()
	view.AddRole(application.FullScreen)

	// Not the WindowMenu role: that registers the menu with AppKit as the
	// windows menu, and macOS then fills it with tiling, display and window-list
	// items that mean nothing for a single-window app.
	window := menu.AddSubmenu("Window")
	window.AddRole(application.Minimise)
	window.AddRole(application.Zoom)

	return menu
}

func registerContextMenus(wailsApp *application.App) {
	for name, items := range sidebarMenuItems {
		contextMenu := wailsApp.ContextMenu.New()
		for _, item := range items {
			if item.event == "" {
				contextMenu.AddSeparator()
				continue
			}
			contextMenu.Add(item.label).OnClick(func(context *application.Context) {
				wailsApp.Event.Emit(item.event, context.ContextMenuData())
			})
		}
		wailsApp.ContextMenu.Add(name, contextMenu)
	}

	terminalMenu := wailsApp.ContextMenu.New()
	terminalMenu.Add("Copy Selection").OnClick(func(context *application.Context) {
		wailsApp.Event.Emit(EventMenuTerminalCopy, context.ContextMenuData())
	})
	terminalMenu.Add("Paste Clipboard").OnClick(func(context *application.Context) {
		wailsApp.Event.Emit(EventMenuTerminalPaste, context.ContextMenuData())
	})
	wailsApp.ContextMenu.Add("terminal", terminalMenu)
}
