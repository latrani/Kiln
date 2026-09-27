package logstore

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var pdt = time.FixedZone("PDT", -7*3600)

// kit is fm/kit's layout under root, with the given templates.
func kit(root, dir, name string) Layout {
	return Layout{Root: root, Dir: dir, Name: name, World: "fm", Char: "kit", CharName: "Kit"}
}

// rel is paths relative to root.
func rel(root string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i], _ = filepath.Rel(root, p)
		out[i] = filepath.ToSlash(out[i])
	}
	return out
}

func TestLayoutPath(t *testing.T) {
	at := time.Date(2026, 9, 16, 20, 28, 58, 0, pdt)
	for _, c := range []struct{ dir, name, wantDir, wantName string }{
		{"", "", "/data/logs/fm/kit", "2026-09-16 202858 kit"},
		{"/x/Mucks/{world}/{name}/%Y/%m", "%Y-%m-%d.%H.%M.%S", "/x/Mucks/fm/Kit/2026/09", "2026-09-16.20.28.58"},
		{"mine/{world}", "{name} %Y%m%d", "/data/logs/mine/fm", "Kit 20260916"},
	} {
		dir, name := kit("/data/logs", c.dir, c.name).Path(at)
		if dir != c.wantDir || name != c.wantName {
			t.Errorf("Path(%q, %q) = %q, %q; want %q, %q", c.dir, c.name, dir, name, c.wantDir, c.wantName)
		}
	}
	for dir, want := range map[string]string{
		"":                        "/data/logs/fm/kit",
		"/x/{world}/{name}/%Y/%m": "/x/fm/Kit",
		"/x/%Y/{world}":           "/x",
		"/x/{world}/log-%Y":       "/x/fm",
	} {
		if got := kit("/data/logs", dir, "").ScanRoot(); got != want {
			t.Errorf("ScanRoot(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestWriterCreatesFileWithHeader(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(kit(root, "", ""))
	if err := w.Append(Entry{time.Date(2026, 9, 24, 21, 0, 5, 0, pdt), In, "hello"}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	b, err := os.ReadFile(filepath.Join(root, "fm", "kit", "2026-09-24 210005 kit.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := Header + "\n#kiln-char fm/kit\n2026-09-24T21:00:05.000-07:00 <\thello\n"
	if string(b) != want {
		t.Errorf("file = %q, want %q", b, want)
	}
}

func TestWriterBucketsAndSessions(t *testing.T) {
	root := t.TempDir()
	l := kit(root, "{world}/{name}/%Y/%m", "%Y-%m-%d.%H.%M.%S")
	w := NewWriter(l)
	defer w.Close()
	ts := time.Date(2026, 9, 30, 23, 59, 0, 0, pdt)
	w.Append(Entry{ts, In, "one"})
	w.Append(Entry{ts.Add(2 * time.Minute), In, "still one, into October"})
	w.NewSession()
	w.Append(Entry{ts.Add(time.Hour), In, "two"})
	w.NewSession()
	w.Append(Entry{ts.Add(time.Hour), In, "same second: appended to two"})
	files, err := Files(l)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fm/Kit/2026/09/2026-09-30.23.59.00.log", "fm/Kit/2026/10/2026-10-01.00.59.00.log"}
	if got := rel(root, files); !reflect.DeepEqual(got, want) {
		t.Fatalf("Files = %q, want %q", got, want)
	}
	if es, _ := ReadFile(files[1]); len(es) != 2 {
		t.Errorf("second file has %d entries, want 2 (the same name is this character's own log)", len(es))
	}
}

func TestWriterDailyNameAppends(t *testing.T) {
	root := t.TempDir()
	l := kit(root, "", "%Y-%m-%d")
	for _, h := range []int{9, 21} { // two runs of Kiln, one file for the day
		w := NewWriter(l)
		w.Append(Entry{time.Date(2026, 9, 24, h, 0, 0, 0, pdt), In, "hi"})
		w.Close()
	}
	files, _ := Files(l)
	if len(files) != 1 {
		t.Fatalf("files = %q, want one", files)
	}
	b, _ := os.ReadFile(files[0])
	if n := strings.Count(string(b), Header); n != 1 {
		t.Errorf("%d headers:\n%s", n, b)
	}
	if es, _ := ReadFile(files[0]); len(es) != 2 {
		t.Errorf("%d entries, want 2", len(es))
	}
}

func TestWriterNeverWritesOthersFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "shared")
	os.MkdirAll(dir, 0o700)
	foreign := filepath.Join(dir, "2026-09-24.log")
	os.WriteFile(foreign, []byte("some other client's log\n"), 0o600)
	rook := Layout{Root: root, Dir: "shared", Name: "%Y-%m-%d", World: "fm", Char: "rook", CharName: "Rook"}
	w := NewWriter(rook) // takes " (2)": the plain name is foreign
	w.Append(Entry{time.Date(2026, 9, 24, 9, 0, 0, 0, pdt), In, "rook"})
	w.Close()
	w = NewWriter(Layout{Root: root, Dir: "shared", Name: "%Y-%m-%d", World: "fm", Char: "kit", CharName: "Kit"})
	w.Append(Entry{time.Date(2026, 9, 24, 10, 0, 0, 0, pdt), In, "kit"}) // " (3)": " (2)" is Rook's
	w.Close()
	if b, _ := os.ReadFile(foreign); string(b) != "some other client's log\n" {
		t.Errorf("foreign file changed: %q", b)
	}
	rf, _ := Files(rook)
	kf, _ := Files(kit(root, "shared", "%Y-%m-%d"))
	if got := rel(root, rf); !reflect.DeepEqual(got, []string{"shared/2026-09-24 (2).log"}) {
		t.Errorf("Rook's files = %q", got)
	}
	if got := rel(root, kf); !reflect.DeepEqual(got, []string{"shared/2026-09-24 (3).log"}) {
		t.Errorf("Kit's files = %q", got)
	}
}

func TestFilesFindsOldLogsAndSkipsOthers(t *testing.T) {
	root := t.TempDir()
	put := func(path, content string) {
		p := filepath.Join(root, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(content), 0o600)
	}
	entry := func(day int) string {
		return Format(Entry{time.Date(2026, 9, day, 12, 0, 0, 0, pdt), In, "x"}) + "\n"
	}
	old := Header + "\n"
	put("fm/kit/2026-09-20.log", old+entry(20))                                  // a per-day log
	put("fm/kit/2026-09-21 120000 kit.log", old+entry(21))                       // a per-session log
	put("fm/kit/2026/09/whatever.log", Header+"\n#kiln-char fm/kit\n"+entry(22)) // a new one, any name
	put("fm/kit/2026/09/rook.log", Header+"\n#kiln-char fm/rook\n"+entry(23))    // someone else's
	put("fm/kit/2026-09-19 120000 rook.log", old+entry(19))                      // an old one of someone else's
	put("fm/kit/notes.log", "not a log\n")                                       // another program's
	put("fm/kit/2026-09-18.txt", old+entry(18))                                  // not .log
	files, err := Files(kit(root, "{world}/{char}/%Y/%m", ""))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fm/kit/2026-09-20.log", "fm/kit/2026-09-21 120000 kit.log", "fm/kit/2026/09/whatever.log"}
	if got := rel(root, files); !reflect.DeepEqual(got, want) {
		t.Errorf("Files = %q\nwant    %q", got, want)
	}
}

func TestWriterConcurrentAppend(t *testing.T) {
	root := t.TempDir()
	l := kit(root, "", "")
	w := NewWriter(l)
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
	files, _ := Files(l)
	if len(files) != 1 {
		t.Fatalf("%d files, want 1", len(files))
	}
	if es, _ := ReadFile(files[0]); len(es) != 400 {
		t.Errorf("read back %d entries, want 400", len(es))
	}
}

func TestFilesMissingDirIsEmpty(t *testing.T) {
	files, err := Files(kit(filepath.Join(t.TempDir(), "nope"), "", ""))
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
