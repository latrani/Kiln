package logstore

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

var pdt = time.FixedZone("PDT", -7*3600)

// names is the base names of paths.
func names(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

func TestCharDir(t *testing.T) {
	for _, c := range []struct{ template, want string }{
		{"", "/data/logs/fm/kit"},
		{"/x/Mucks/{world}", "/x/Mucks/fm"},
		{"/x/{char}@{world}", "/x/kit@fm"},
		{"mine/{world}", "/data/logs/mine/fm"},
	} {
		if got := CharDir(c.template, "/data/logs", "fm", "kit"); got != c.want {
			t.Errorf("CharDir(%q) = %q, want %q", c.template, got, c.want)
		}
	}
}

func TestWriterCreatesSessionFileWithHeader(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir, "Kit")
	if err := w.Append(Entry{time.Date(2026, 9, 24, 21, 0, 5, 0, pdt), In, "hello"}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	b, err := os.ReadFile(filepath.Join(dir, "2026-09-24 210005 Kit.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := Header + "\n2026-09-24T21:00:05.000-07:00 <\thello\n"
	if string(b) != want {
		t.Errorf("file = %q, want %q", b, want)
	}
}

func TestWriterOneFilePerSession(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir, "Kit")
	defer w.Close()
	ts := time.Date(2026, 9, 24, 23, 59, 0, 0, pdt)
	w.Append(Entry{ts, In, "one"})
	w.Append(Entry{ts.Add(2 * time.Minute), In, "still one, past midnight"})
	w.NewSession()
	w.Append(Entry{ts.Add(time.Hour), In, "two"})
	w.NewSession()
	w.Append(Entry{ts.Add(time.Hour), In, "three, the same second"})
	files, err := Files(dir, "Kit")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-09-24 235900 Kit.log", "2026-09-25 005900 Kit.log", "2026-09-25 005900 Kit (2).log"}
	if !reflect.DeepEqual(names(files), want) {
		t.Fatalf("Files = %q, want %q", names(files), want)
	}
	es, _ := ReadFile(files[0])
	if len(es) != 2 {
		t.Errorf("first session has %d entries, want 2", len(es))
	}
}

func TestWriterNeverWritesForeignFiles(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "2026-09-24 210005 Kit.log")
	os.WriteFile(foreign, []byte("some other client's log\n"), 0o600)
	w := NewWriter(dir, "Kit")
	w.Append(Entry{time.Date(2026, 9, 24, 21, 0, 5, 0, pdt), In, "hello"})
	w.Close()
	if b, _ := os.ReadFile(foreign); string(b) != "some other client's log\n" {
		t.Errorf("foreign file changed: %q", b)
	}
	files, _ := Files(dir, "Kit")
	if want := []string{"2026-09-24 210005 Kit (2).log"}; !reflect.DeepEqual(names(files), want) {
		t.Errorf("Files = %q, want %q (the foreign file skipped)", names(files), want)
	}
}

func TestFilesSortsAndFilters(t *testing.T) {
	dir := t.TempDir()
	put := func(name, content string) { os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600) }
	ours := Header + "\n"
	put("2026-09-24.log", ours)                 // an older Kiln's day file
	put("2026-09-24 210000 Kit.log", ours)      // later that day
	put("2026-09-23 120000 Kit.log", ours)      // earlier
	put("2026-09-23 120000 Kit (10).log", ours) // numbered, sorted numerically
	put("2026-09-23 120000 Kit (2).log", ours)  //
	put("2026-09-23 120000 Rook.log", ours)     // another character sharing the folder
	put("2026-09-22 120000 Kit.log", "foreign") // right name, wrong format
	put("2026-09-22.log", "#kiln-log v10\n")    // not quite our header
	put("notes.txt", ours)                      // not a log name
	os.Mkdir(filepath.Join(dir, "2026-09-21.log"), 0o700)
	files, err := Files(dir, "Kit")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-09-23 120000 Kit.log", "2026-09-23 120000 Kit (2).log", "2026-09-23 120000 Kit (10).log",
		"2026-09-24.log", "2026-09-24 210000 Kit.log"}
	if !reflect.DeepEqual(names(files), want) {
		t.Errorf("Files = %q\nwant    %q", names(files), want)
	}
}

func TestWriterConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir, "c")
	defer w.Close()
	ts := time.Date(2026, 9, 24, 23, 59, 59, 0, pdt)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if err := w.Append(Entry{ts, In, "x"}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	files, _ := Files(dir, "c")
	if len(files) != 1 {
		t.Fatalf("%d files, want 1", len(files))
	}
	if es, _ := ReadFile(files[0]); len(es) != 400 {
		t.Errorf("read back %d entries, want 400", len(es))
	}
}

func TestFilesMissingDirIsEmpty(t *testing.T) {
	files, err := Files(filepath.Join(t.TempDir(), "nope"), "nobody")
	if err != nil || len(files) != 0 {
		t.Errorf("Files = %v, %v; want empty, nil", files, err)
	}
}

func TestReadFileSkipsCorruptLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "2026-09-24.log")
	content := Header + "\n" +
		"2026-09-24T21:00:00.000-07:00 <\tgood one\n" +
		"2026-09-24T21:0\n" + // crash-truncated
		"2026-09-24T21:00:02.000-07:00 >\tgood two\n"
	os.WriteFile(path, []byte(content), 0o600)
	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "good one" || got[1].Text != "good two" {
		t.Errorf("entries = %+v", got)
	}
}
