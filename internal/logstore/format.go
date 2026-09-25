// Package logstore reads and writes Kiln's plain-text log files.
//
// Each file holds one character's traffic for one local day:
//
//	#kiln-log v1
//	2026-09-24T21:14:03.120-07:00 <	Rook says, "Evening!"
//
// The prefix is fixed-width; everything after the first TAB is the raw
// line, unescaped, with ANSI bytes intact.
package logstore

import (
	"fmt"
	"strings"
	"time"
)

// Dir is the direction of a logged line.
type Dir byte

const (
	In  Dir = '<' // received from the server
	Out Dir = '>' // sent by the user
	Sys Dir = '*' // client/system event
)

// Header is the first line of every log file.
const Header = "#kiln-log v1"

// timeLayout always renders a numeric offset (never "Z") so the prefix
// stays fixed-width.
const timeLayout = "2006-01-02T15:04:05.000-07:00"

// Entry is one logged line.
type Entry struct {
	Time time.Time
	Dir  Dir
	Text string
}

// Format renders e as a single log line without a trailing newline.
// Any CR/LF inside Text is replaced with a space so one entry is always
// one line.
func Format(e Entry) string {
	text := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(e.Text)
	return e.Time.Format(timeLayout) + " " + string(rune(e.Dir)) + "\t" + text
}

// Parse is the inverse of Format.
func Parse(line string) (Entry, error) {
	tab := strings.IndexByte(line, '\t')
	if tab < 0 {
		return Entry{}, fmt.Errorf("logstore: no tab in %q", line)
	}
	prefix, text := line[:tab], line[tab+1:]
	sp := strings.LastIndexByte(prefix, ' ')
	if sp < 0 || sp != len(prefix)-2 {
		return Entry{}, fmt.Errorf("logstore: bad prefix %q", prefix)
	}
	t, err := time.Parse(timeLayout, prefix[:sp])
	if err != nil {
		return Entry{}, fmt.Errorf("logstore: bad time: %w", err)
	}
	d := Dir(prefix[sp+1])
	if d != In && d != Out && d != Sys {
		return Entry{}, fmt.Errorf("logstore: bad direction %q", prefix[sp+1])
	}
	return Entry{Time: t, Dir: d, Text: text}, nil
}
