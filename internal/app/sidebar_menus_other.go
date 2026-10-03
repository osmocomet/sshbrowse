//go:build !windows

package app

import "errors"

func (m *SidebarMenus) show(_ []sidebarMenuItem, _ string, _, _ float64) error {
	return errors.New("sidebar menu positioning is Windows-only")
}
