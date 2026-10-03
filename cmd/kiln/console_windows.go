package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList") //str:ok: a DLL and its function

// ownsConsole reports whether kiln is the only process attached to its
// console: Windows made the console just for kiln (a double-click in
// Explorer, the Run box) and will close it as soon as kiln exits.
func ownsConsole() bool {
	var pids [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}
