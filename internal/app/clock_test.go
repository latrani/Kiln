package app

import (
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/str"
)

func TestClockAndConnectedSince(t *testing.T) {
	now := time.Date(2026, 9, 24, 21, 30, 0, 0, time.Local)
	today := time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)
	before := time.Date(2026, 9, 23, 8, 5, 0, 0, time.Local)
	if got := TimeOfDay(today); got != "21:14" {
		t.Errorf("TimeOfDay = %q", got)
	}
	if got := Clock(today, now); got != "21:14" {
		t.Errorf("Clock(today) = %q", got)
	}
	if got, want := Clock(before, now), before.Format(str.DateDayTime()); got != want {
		t.Errorf("Clock(yesterday) = %q, want %q", got, want)
	}
	if got, want := ConnectedSince(today, now), str.ViewConnectedSince("21:14"); got != want {
		t.Errorf("ConnectedSince(today) = %q, want %q", got, want)
	}
	if got, want := ConnectedSince(before, now), str.ViewConnectedSinceDay(before.Format(str.DateDay()), "08:05"); got != want {
		t.Errorf("ConnectedSince(yesterday) = %q, want %q", got, want)
	}
}
