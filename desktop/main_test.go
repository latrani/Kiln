package main

import (
	"errors"
	"testing"
)

func TestWindowsCancelIsWailsCancel(t *testing.T) {
	if !windowsCancel(errors.New("cancelled by user")) { // Wails' cfd.ErrorCancelled, word for word
		t.Error("the Windows dialog's cancel wasn't seen as one")
	}
	if windowsCancel(nil) || windowsCancel(errors.New("access denied")) {
		t.Error("a nil or other error was taken for a cancel")
	}
}
