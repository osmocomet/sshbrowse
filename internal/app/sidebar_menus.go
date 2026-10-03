package app

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SidebarMenus places Windows native menus at the row that was clicked.
// Other platforms keep Wails' context menu handling.
type SidebarMenus struct {
	app    *application.App
	window *application.WebviewWindow
}

func NewSidebarMenus(wailsApp *application.App, window *application.WebviewWindow) *SidebarMenus {
	return &SidebarMenus{app: wailsApp, window: window}
}

type sidebarMenuItem struct {
	label string
	event string
}

var sidebarMenuItems = map[string][]sidebarMenuItem{
	"saved-connection": {
		{"Connect", EventMenuConnectionOpen},
		{"Connect with SFTP", EventMenuConnectionSFTP},
		{"Edit…", EventMenuConnectionEdit},
		{"Duplicate…", EventMenuConnectionDuplicate},
		{"New Folder…", EventMenuConnectionNewFolder},
		{"Move to Folder…", EventMenuConnectionMove},
		{},
		{"Delete…", EventMenuConnectionDelete},
	},
	"saved-folder": {
		{"Connect all", EventMenuFolderOpen},
		{},
		{"New connection…", EventMenuFolderNewConnection},
		{"New Subfolder…", EventMenuFolderCreate},
		{"Rename…", EventMenuFolderRename},
		{},
		{"Delete", EventMenuFolderDelete},
	},
	"saved-folder-empty": {
		{"New connection…", EventMenuFolderNewConnection},
		{"New Subfolder…", EventMenuFolderCreate},
		{"Rename…", EventMenuFolderRename},
		{},
		{"Delete", EventMenuFolderDelete},
	},
	"saved-sidebar": {
		{"New Folder…", EventMenuFolderCreate},
	},
}

func (m *SidebarMenus) Show(name, data string, x, y float64) error {
	items, ok := sidebarMenuItems[name]
	if !ok {
		return errors.New("unknown sidebar menu")
	}
	return m.show(items, data, x, y)
}
