package pathfmt

import (
	"testing"
	"time"
)

var at = time.Date(2026, 9, 6, 20, 8, 5, 0, time.UTC)

func TestExpand(t *testing.T) {
	vars := map[string]string{"world": "tapestries", "name": "Indi/2"}
	for tmpl, want := range map[string]string{
		"%Y-%m-%d.%H.%M.%S":     "2026-09-06.20.08.05",
		"{world}/{name}/%Y/%m":  "tapestries/Indi_2/2026/09",
		"%a %d %b %y, %I:%M %p": "Sun 06 Sep 26, 08:08 PM",
		"100%% {unknown} %":     "100% {unknown} %",
		"%A %B":                 "Sunday September",
	} {
		if got := Expand(tmpl, vars, at); got != want {
			t.Errorf("Expand(%q) = %q, want %q", tmpl, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	vars := []string{"world", "name"}
	for tmpl, ok := range map[string]bool{
		"{world}/%Y/%m": true,
		"%%literal":     true,
		"{char}":        false,
		"%Q":            false,
		"trailing %":    false,
		"plain":         true,
	} {
		if err := Check(tmpl, vars); (err == nil) != ok {
			t.Errorf("Check(%q) = %v", tmpl, err)
		}
	}
	if !HasTime("a/%Y") || HasTime("100%%") || HasTime("plain") {
		t.Error("HasTime wrong")
	}
}
