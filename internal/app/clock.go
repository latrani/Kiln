package app

import (
	"time"

	"github.com/latrani/Kiln/internal/str"
)

// TimeOfDay is t's local time of day, as log mode shows it.
func TimeOfDay(t time.Time) string { return t.Local().Format("15:04") } //str:ok: a time layout

// Clock is t as a time of day, with the day first if it wasn't today.
func Clock(t, now time.Time) string {
	t = t.Local()
	if DayOf(t) != DayOf(now) {
		return t.Format(str.DateDayTime())
	}
	return TimeOfDay(t)
}

// ConnectedSince says when a connection came up at: the time, with the
// day first if that wasn't today.
func ConnectedSince(at, now time.Time) string {
	at = at.Local()
	if DayOf(at) != DayOf(now) {
		return str.ViewConnectedSinceDay(at.Format(str.DateDay()), TimeOfDay(at))
	}
	return str.ViewConnectedSince(TimeOfDay(at))
}
