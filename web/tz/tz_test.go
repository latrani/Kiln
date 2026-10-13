package tz

import (
	"testing"
	"time"
)

func TestZone(t *testing.T) {
	ny := Zone("America/New_York")
	if ny == nil {
		t.Fatal("no New York")
	}
	// Across a daylight-saving change the offset differs: a fixed offset
	// (Go's wasm default) can't do that.
	_, winter := time.Date(2026, 1, 15, 12, 0, 0, 0, ny).Zone()
	_, summer := time.Date(2026, 7, 15, 12, 0, 0, 0, ny).Zone()
	if winter == summer {
		t.Errorf("same offset in winter and summer: %d", winter)
	}
	if Zone("") != nil || Zone("Not/AZone") != nil {
		t.Error("an empty or unknown zone should be nil")
	}
}
