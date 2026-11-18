//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// launchedByDoubleClick reports whether the program owns its console window
// alone, which is the case when it was opened from Explorer rather than from
// a terminal.
func launchedByDoubleClick() bool {
	var ids [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids))) //nolint:gosec // the buffer outlives the call
	return n == 1
}
