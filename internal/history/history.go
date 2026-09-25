// Package history pages a character's logs backward, one day at a time.
package history

import (
	"slices"

	"github.com/latrani/Kiln/internal/logstore"
)

// Reader walks a character's log files from newest to oldest, handing
// back one whole local day at a time however the days are split across
// files (a session file can hold several days, and a day several
// sessions).
type Reader struct {
	files []string         // oldest first
	next  int              // index of the next file to read
	buf   []logstore.Entry // read but not yet returned, oldest first
}

// NewReader lists char's log files in dir. A character with no logs
// yields a Reader that is immediately exhausted.
func NewReader(dir, char string) (*Reader, error) {
	files, err := logstore.Files(dir, char)
	if err != nil {
		return nil, err
	}
	return &Reader{files: files, next: len(files) - 1}, nil
}

func day(e logstore.Entry) string { return e.Time.Local().Format("2006-01-02") }

// LoadOlder returns the next older day's entries and its date
// ("YYYY-MM-DD"). ok is false once every day has been returned. A file
// that can't be read is reported once, and skipped by the next call.
func (r *Reader) LoadOlder() (entries []logstore.Entry, d string, ok bool, err error) {
	for len(r.buf) == 0 {
		if r.next < 0 {
			return nil, "", false, nil
		}
		if err := r.readNext(); err != nil {
			return nil, "", true, err
		}
	}
	d = day(r.buf[len(r.buf)-1])
	// The oldest read line is on d, so an older file may hold more of d.
	for r.next >= 0 && day(r.buf[0]) >= d {
		if err := r.readNext(); err != nil {
			return nil, "", true, err
		}
	}
	i := len(r.buf)
	for i > 0 && day(r.buf[i-1]) == d {
		i--
	}
	entries, r.buf = slices.Clone(r.buf[i:]), r.buf[:i]
	return entries, d, true, nil
}

// readNext reads the next older file into buf.
func (r *Reader) readNext() error {
	path := r.files[r.next]
	r.next--
	es, err := logstore.ReadFile(path)
	if err != nil {
		return err
	}
	r.buf = append(es, r.buf...)
	return nil
}

// Exhausted reports whether every day has been loaded.
func (r *Reader) Exhausted() bool { return r.next < 0 && len(r.buf) == 0 }
