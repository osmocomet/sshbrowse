package app

import (
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestLinuxAndWindowsUseFrontendMenuAndRegisterContextMenus(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS uses its native application menu")
	}

	wailsApp := application.New(application.Options{Name: "SSHBrowse"})
	menu := BuildMenu(wailsApp)
	if menu != nil {
		t.Fatal("Linux and Windows should render the application menu in Svelte")
	}

	for _, name := range []string{"saved-connection", "saved-folder", "saved-folder-empty", "saved-sidebar", "terminal"} {
		if _, ok := wailsApp.ContextMenu.Get(name); !ok {
			t.Fatalf("context menu %q is not registered", name)
		}
	}
	terminalMenu, ok := wailsApp.ContextMenu.Get("terminal")
	if !ok {
		t.Fatal("terminal context menu is not registered")
	}
	for _, label := range []string{"Copy Selection", "Paste Clipboard"} {
		if terminalMenu.FindByLabel(label) == nil {
			t.Fatalf("terminal context menu item %q is missing", label)
		}
	}
	for _, test := range []struct {
		menu  string
		label string
	}{
		{menu: "saved-connection", label: "Connect"},
		{menu: "saved-connection", label: "Connect with SFTP"},
		{menu: "saved-connection", label: "Edit…"},
		{menu: "saved-connection", label: "Duplicate…"},
		{menu: "saved-connection", label: "New Folder…"},
		{menu: "saved-connection", label: "Move to Folder…"},
		{menu: "saved-connection", label: "Delete…"},
		{menu: "saved-folder", label: "Connect all"},
		{menu: "saved-folder", label: "New connection…"},
		{menu: "saved-folder", label: "New Subfolder…"},
		{menu: "saved-folder", label: "Rename…"},
		{menu: "saved-folder", label: "Delete"},
		{menu: "saved-folder-empty", label: "New connection…"},
		{menu: "saved-folder-empty", label: "New Subfolder…"},
		{menu: "saved-folder-empty", label: "Rename…"},
		{menu: "saved-folder-empty", label: "Delete"},
		{menu: "saved-sidebar", label: "New Folder…"},
	} {
		contextMenu, ok := wailsApp.ContextMenu.Get(test.menu)
		if !ok || contextMenu.FindByLabel(test.label) == nil {
			t.Fatalf("context menu %q is missing item %q", test.menu, test.label)
		}
	}
	emptyFolderMenu, _ := wailsApp.ContextMenu.Get("saved-folder-empty")
	for _, label := range []string{"Connect", "Connect all"} {
		if emptyFolderMenu.FindByLabel(label) != nil {
			t.Fatalf("empty folder menu must hide %q", label)
		}
	}
}

func TestMacOSKeepsNativeEditRolesAndAddsSettings(t *testing.T) {
	wailsApp := application.New(application.Options{Name: "SSHBrowse"})
	menu := buildNativeMenu(wailsApp, nil)
	for _, role := range []application.Role{
		application.Undo,
		application.Redo,
		application.Cut,
		application.Copy,
		application.Paste,
		application.SelectAll,
	} {
		if menu.FindByRole(role) == nil {
			t.Errorf("macOS native Edit role %v is missing", role)
		}
	}
	appMenu := menu.FindByLabel("SSHBrowse")
	if appMenu == nil || appMenu.GetSubmenu() == nil {
		t.Fatal("macOS application menu is missing")
	}
	settings := appMenu.GetSubmenu().FindByLabel("Settings…")
	if settings == nil {
		t.Fatal("macOS application menu is missing Settings…")
	}
	shortcut := "Ctrl+,"
	if runtime.GOOS == "darwin" {
		shortcut = "Cmd+,"
	}
	if settings.GetAccelerator() != shortcut {
		t.Fatalf("Settings shortcut = %q, want %q", settings.GetAccelerator(), shortcut)
	}
	if menu.FindByLabel("Settings") != nil {
		t.Fatal("macOS should not have a top-level Settings menu")
	}
}
