package logstore

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWriterCreatesFileWithHeader(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(root, "furrymuck", "kit")
	pdt := time.FixedZone("PDT", -7*3600)
	if err := w.Append(Entry{time.Date(2026, 9, 24, 21, 0, 0, 0, pdt), In, "hello"}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	b, err := os.ReadFile(filepath.Join(root, "furrymuck", "kit", "2026-09-24.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := Header + "\n2026-09-24T21:00:00.000-07:00 <\thello\n"
	if string(b) != want {
		t.Errorf("file = %q, want %q", b, want)
	}
}

func TestWriterAppendsWithoutSecondHeader(t *testing.T) {
	root := t.TempDir()
	ts := time.Date(2026, 9, 24, 21, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ { // two separate writers = two app runs
		w := NewWriter(root, "w", "c")
		if err := w.Append(Entry{ts, In, "line"}); err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
	b, _ := os.ReadFile(filepath.Join(root, "w", "c", "2026-09-24.log"))
	if n := strings.Count(string(b), Header); n != 1 {
		t.Errorf("header count = %d, want 1\n%s", n, b)
	}
}

func TestWriterRollsOverAtMidnight(t *testing.T) {
	root := t.TempDir()
	w := NewWriter(root, "w", "c")
	defer w.Close()
	pdt := time.FixedZone("PDT", -7*3600)
	w.Append(Entry{time.Date(2026, 9, 24, 23, 59, 59, 0, pdt), In, "before"})
	w.Append(Entry{time.Date(2026, 9, 25, 0, 0, 1, 0, pdt), In, "after"})
	days, err := Days(root, "w", "c")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026-09-24", "2026-09-25"}; !reflect.DeepEqual(days, want) {
		t.Fatalf("Days = %v, want %v", days, want)
	}
	got, _ := ReadDay(root, "w", "c", "2026-09-25")
	if len(got) != 1 || got[0].Text != "after" {
		t.Errorf("2026-09-25 entries = %+v", got)
	}
}

func TestDaysMissingDirIsEmpty(t *testing.T) {
	days, err := Days(t.TempDir(), "nope", "nobody")
	if err != nil || len(days) != 0 {
		t.Errorf("Days = %v, %v; want empty, nil", days, err)
	}
}

func TestReadDaySkipsCorruptLines(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "w", "c")
	os.MkdirAll(dir, 0o700)
	content := Header + "\n" +
		"2026-09-24T21:00:00.000-07:00 <\tgood one\n" +
		"2026-09-24T21:0\n" + // crash-truncated
		"2026-09-24T21:00:02.000-07:00 >\tgood two\n"
	os.WriteFile(filepath.Join(dir, "2026-09-24.log"), []byte(content), 0o600)
	got, err := ReadDay(root, "w", "c", "2026-09-24")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "good one" || got[1].Text != "good two" {
		t.Errorf("entries = %+v", got)
	}
}
