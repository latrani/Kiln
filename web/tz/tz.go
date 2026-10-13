// Package tz finds the browser's time zone for the wasm build. Go's wasm
// runtime fixes the local offset at start-up, so without it times on the
// far side of a daylight-saving change are an hour off.
package tz

import (
	"time"
	_ "time/tzdata" // the browser has no zoneinfo files
)

// Zone is the IANA time zone name, or nil when there's none to use.
func Zone(name string) *time.Location {
	if name == "" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}
