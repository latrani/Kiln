package logstore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const dayLayout = "2006-01-02"

// CharDir is the directory holding one character's day files.
func CharDir(root, world, char string) string {
	return filepath.Join(root, world, char)
}

// Writer appends entries to per-day files under CharDir. The day is taken
// from each entry's own timestamp (in its own location), so a session
// running past midnight rolls over to a new file automatically. It is safe
// for concurrent use.
type Writer struct {
	mu  sync.Mutex
	dir string
	day string
	f   *os.File
}

// NewWriter returns a Writer for one character. No files are touched
// until the first Append.
func NewWriter(root, world, char string) *Writer {
	return &Writer{dir: CharDir(root, world, char)}
}

// Append writes e to the day file for e.Time, creating it (with Header)
// if needed.
func (w *Writer) Append(e Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	day := e.Time.Format(dayLayout)
	if w.f == nil || day != w.day {
		if err := w.open(day); err != nil {
			return err
		}
	}
	_, err := w.f.WriteString(Format(e) + "\n")
	return err
}

func (w *Writer) open(day string) error {
	if w.f != nil {
		w.f.Close()
		w.f = nil
	}
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(w.dir, day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if st.Size() == 0 {
		if _, err := f.WriteString(Header + "\n"); err != nil {
			f.Close()
			return err
		}
	}
	w.f, w.day = f, day
	return nil
}

// Close closes the current file, if any.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// Days lists the days that have log files for a character, oldest first,
// as "YYYY-MM-DD" strings. A missing directory yields no days, not an error.
func Days(root, world, char string) ([]string, error) {
	ents, err := os.ReadDir(CharDir(root, world, char))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var days []string
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".log")
		if !ok || e.IsDir() {
			continue
		}
		days = append(days, name)
	}
	sort.Strings(days)
	return days, nil
}

// ReadDay reads every entry from one day file. Lines that fail to parse
// (e.g. a line cut short by a crash) are skipped rather than failing the
// whole read.
func ReadDay(root, world, char, day string) ([]Entry, error) {
	f, err := os.Open(filepath.Join(CharDir(root, world, char), day+".log"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		e, err := Parse(line)
		if err != nil {
			continue
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("logstore: reading %s: %w", day, err)
	}
	return out, nil
}
