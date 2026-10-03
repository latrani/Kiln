//go:build !windows

package main

// ownsConsole reports whether the window kiln runs in closes when it exits,
// which only happens on Windows.
func ownsConsole() bool { return false }
