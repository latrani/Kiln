// Package history pages a character's logs backward, one day at a time.
package history

import "github.com/latrani/Kiln/internal/logstore"

// Reader walks a character's day files from newest to oldest.
type Reader struct {
	root, world, char string
	days              []string // oldest first
	next              int      // index of the next day LoadOlder returns
}

// NewReader lists the character's log days. A character with no logs
// yields a Reader that is immediately exhausted.
func NewReader(root, world, char string) (*Reader, error) {
	days, err := logstore.Days(root, world, char)
	if err != nil {
		return nil, err
	}
	return &Reader{root: root, world: world, char: char, days: days, next: len(days) - 1}, nil
}

// LoadOlder returns the next older day's entries and its date
// ("YYYY-MM-DD"). ok is false once every day has been returned.
func (r *Reader) LoadOlder() (entries []logstore.Entry, day string, ok bool, err error) {
	if r.next < 0 {
		return nil, "", false, nil
	}
	day = r.days[r.next]
	r.next--
	entries, err = logstore.ReadDay(r.root, r.world, r.char, day)
	return entries, day, true, err
}

// Exhausted reports whether every day has been loaded.
func (r *Reader) Exhausted() bool { return r.next < 0 }

// Oldest is the earliest day with logs, or "" if there are none.
func (r *Reader) Oldest() string {
	if len(r.days) == 0 {
		return ""
	}
	return r.days[0]
}
