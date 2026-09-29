package logstore

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/latrani/Kiln/internal/pathfmt"
	"github.com/latrani/Kiln/internal/str"
)

// Defaults for the log_dir and log_name settings.
//
//str:ok
const (
	DefaultDir  = "{world}/{char}"         // under the log root
	DefaultName = "%Y-%m-%d %H%M%S {char}" // plus ".log"
)

// Vars are the {placeholders} log_dir and log_name may use: the world
// id, the character's id, and its name.
var Vars = []string{"world", "char", "name"}

// charLine is the header's second line: whose log this is, so characters
// can share a folder whatever their files are named.
const charLine = "#kiln-char " //str:ok

// Layout says where one character's logs go: the log_dir and log_name
// templates (strftime codes and Vars; "" for the defaults), with a
// relative Dir taken from Root.
type Layout struct {
	Root, Dir, Name       string
	World, Char, CharName string
}

func (l Layout) vars() map[string]string {
	return map[string]string{"world": l.World, "char": l.Char, "name": l.CharName}
}

func (l Layout) key() string { return l.World + "/" + l.Char }

func (l Layout) dirTemplate() string {
	if l.Dir == "" {
		return DefaultDir
	}
	return l.Dir
}

func (l Layout) abs(d string) string {
	if !filepath.IsAbs(d) {
		d = filepath.Join(l.Root, d)
	}
	return d
}

// Path is where a session starting at t is logged: its folder, and its
// file name without ".log".
func (l Layout) Path(t time.Time) (dir, name string) {
	nt := l.Name
	if nt == "" {
		nt = DefaultName
	}
	return l.abs(pathfmt.Expand(l.dirTemplate(), l.vars(), t)), pathfmt.Expand(nt, l.vars(), t)
}

// ScanRoot is the folder holding all of the character's logs: log_dir up
// to its first dated part (%Y and the like), where the path stops being
// the same for every session.
func (l Layout) ScanRoot() string {
	parts := strings.Split(filepath.ToSlash(l.dirTemplate()), "/")
	for i, p := range parts {
		if pathfmt.HasTime(p) {
			parts = parts[:i]
			break
		}
	}
	d := pathfmt.Expand(strings.Join(parts, "/"), l.vars(), time.Time{})
	if d == "" {
		d = "."
	}
	return l.abs(filepath.FromSlash(d))
}

// Writer appends entries to one file per session, named by the Layout
// after the session's first entry. NewSession ends a session; the next
// Append starts another. When the name is already taken by this
// character's own log (a log_name with no seconds, say) it appends to
// that; by any other file, it adds " (2)", " (3)" and so on. It never
// writes to a file that isn't this character's. It is safe for
// concurrent use.
type Writer struct {
	mu sync.Mutex
	l  Layout
	f  *os.File
}

// NewWriter returns a Writer for the character l describes. No files are
// touched until the first Append.
func NewWriter(l Layout) *Writer { return &Writer{l: l} }

// Append writes e to the session's file, creating it (with the header) if
// this is the session's first entry.
func (w *Writer) Append(e Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		if err := w.create(e); err != nil {
			return err
		}
	}
	_, err := w.f.WriteString(Format(e) + "\n")
	return err
}

// create opens the session's file for e: a new one, or this character's
// own log already at that name.
func (w *Writer) create(e Entry) error {
	dir, base := w.l.Path(e.Time)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for n := 1; ; n++ {
		name := base + ".log"
		if n > 1 {
			name = fmt.Sprintf("%s (%d).log", base, n) //str:ok
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, os.ErrExist) {
			if h := readHead(path); h.ours && h.char == w.l.key() {
				if f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0); err == nil {
					w.f = f
				}
				return err
			}
			if n < 1000 {
				continue
			}
		}
		if err != nil {
			return err
		}
		if _, err := f.WriteString(Header + "\n" + charLine + w.l.key() + "\n"); err != nil {
			f.Close()
			return err
		}
		w.f = f
		return nil
	}
}

// NewSession ends the current session file; the next Append starts one.
func (w *Writer) NewSession() { w.Close() }

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

// head is what the start of a log file says.
type head struct {
	ours  bool      // it starts with Header
	char  string    // "world/char" from its #kiln-char line; "" in older logs
	first time.Time // its first entry's time; zero if none
}

// readHead reads the start of the file at path.
func readHead(path string) head {
	var h head
	f, err := os.Open(path)
	if err != nil {
		return h
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1024*1024)
	for i := 0; sc.Scan() && i < 8; i++ {
		line := strings.TrimSuffix(sc.Text(), "\r")
		switch {
		case i == 0:
			if line != Header {
				return h
			}
			h.ours = true
		case strings.HasPrefix(line, charLine):
			h.char = strings.TrimPrefix(line, charLine)
		case strings.HasPrefix(line, "#"):
		default:
			if e, err := Parse(line); err == nil {
				h.first = e.Time
				return h
			}
		}
	}
	return h
}

// Older versions of Kiln wrote logs with no #kiln-char line: per day
// ("2026-09-24.log" in the character's own folder), then per session
// ("2026-09-24 211403 Kit.log").
var dayFileRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.log$`) //str:ok

func (l Layout) oldName(path string) bool {
	name := filepath.Base(path)
	if dayFileRE.MatchString(name) {
		return filepath.Base(filepath.Dir(path)) == l.Char
	}
	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{6} ` + regexp.QuoteMeta(l.Char) + `(?: \(\d+\))?\.log$`) //str:ok
	return re.MatchString(name)
}

// Files lists the character's log files, oldest first, searching
// ScanRoot and every folder under it. A file counts when it starts with
// Header and names this character (or, from older versions, has one of
// their names), so other programs' files and other characters' logs in
// the same folders are skipped, whatever they're called. A missing
// folder yields no files, not an error.
func Files(l Layout) ([]string, error) {
	type file struct {
		path  string
		first time.Time
	}
	var files []file
	err := filepath.WalkDir(l.ScanRoot(), func(path string, d fs.DirEntry, err error) error {
		if err != nil { // a missing or unreadable folder: skip it
			if d != nil && d.IsDir() && path != l.ScanRoot() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".log") {
			return nil
		}
		h := readHead(path)
		if h.ours && (h.char == l.key() || h.char == "" && l.oldName(path)) {
			files = append(files, file{path, h.first})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		if !files[i].first.Equal(files[j].first) {
			return files[i].first.Before(files[j].first)
		}
		return files[i].path < files[j].path
	})
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.path
	}
	return out, nil
}

// ReadFile reads every entry from one log file. Lines that fail to parse
// (e.g. a line cut short by a crash) are skipped rather than failing the
// whole read.
func ReadFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
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
		return out, str.Wrap(str.LogstoreReading(filepath.Base(path), err), err)
	}
	return out, nil
}
