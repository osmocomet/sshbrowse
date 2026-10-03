//go:build linux && cgo && !android && !server

package main

/*
#cgo pkg-config: glib-2.0
#include <glib.h>
#include <stdlib.h>
*/
import "C"
import "unsafe"

// setLinuxApplicationName sets GLib's display name separately from the
// program name Wails uses as the Wayland app ID. Call it before Wails startup.
func setLinuxApplicationName(name string) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	C.g_set_application_name(cName)
}
