package logstore

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DefaultDir is the log directory template used when none is configured,
// relative to the log root.
const DefaultDir = "{world}/{char}"

// CharDir resolves a log directory template for one character: "{world}" and
// "{char}" (the character's id) are filled in. An empty template means
// DefaultDir under root; a relative one is also taken from root.
func CharDir(template, root, world, char string) string {
	if template == "" {
		template = DefaultDir
	}
	d := strings.NewReplacer("{world}", world, "{char}", char).Replace(template)
	if !filepath.IsAbs(d) {
		d = filepath.Join(root, d)
	}
	return d
}

// sessionLayout names a session file by its first line's time.
const sessionLayout = "2006-01-02 150405"

// Writer appends entries to one file per session in a character's log
// directory, named "YYYY-MM-DD HHMMSS <char>.log" after the session's
// first entry. NewSession ends a session; the next Append starts another.
// It never writes to a file it didn't create. It is safe for concurrent
// use.
type Writer struct {
	mu   sync.Mutex
	dir  string
	char string
	f    *os.File
}

// NewWriter returns a Writer for the character char logging to dir. No
// files are touched until the first Append.
func NewWriter(dir, char string) *Writer {
	return &Writer{dir: dir, char: char}
}

// Append writes e to the session's file, creating it (with Header) if
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

// create starts a new session file named after e. If that name is taken
// (by another session that started the same second, or by some other
// program's file), it adds " (2)", " (3)" and so on.
func (w *Writer) create(e Entry) error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	base := e.Time.Format(sessionLayout) + " " + w.char
	for n := 1; ; n++ {
		name := base + ".log"
		if n > 1 {
			name = fmt.Sprintf("%s (%d).log", base, n)
		}
		f, err := os.OpenFile(filepath.Join(w.dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, os.ErrExist) && n < 1000 {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := f.WriteString(Header + "\n"); err != nil {
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

// dayFileRE matches the per-day files older versions of Kiln wrote.
var dayFileRE = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.log$`)

// Files lists char's log files in dir, oldest first: session files and
// older per-day files. Files that don't start with Header are someone
// else's and are skipped, whatever their name, as are other characters'
// files when characters share a directory. A missing directory yields no
// files, not an error.
func Files(dir, char string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sessRE := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{6}) ` + regexp.QuoteMeta(char) + `(?: \((\d+)\))?\.log$`)
	type file struct {
		path, when string
		n          int
	}
	var files []file
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		f := file{path: filepath.Join(dir, e.Name())}
		if m := sessRE.FindStringSubmatch(e.Name()); m != nil {
			f.when, f.n = m[1], 1
			if m[2] != "" {
				f.n, _ = strconv.Atoi(m[2])
			}
		} else if m := dayFileRE.FindStringSubmatch(e.Name()); m != nil {
			f.when = m[1] // sorts before that day's session files
		} else {
			continue
		}
		if ours(f.path) {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].when != files[j].when {
			return files[i].when < files[j].when
		}
		return files[i].n < files[j].n
	})
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.path
	}
	return out, nil
}

// ours reports whether the file at path starts with Header.
func ours(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, len(Header)+1)
	n, _ := f.Read(b)
	return n == len(b) && string(b[:len(Header)]) == Header && (b[len(Header)] == '\n' || b[len(Header)] == '\r')
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
		return out, fmt.Errorf("logstore: reading %s: %w", filepath.Base(path), err)
	}
	return out, nil
}
