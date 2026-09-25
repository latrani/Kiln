package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
)

func at(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, time.Local) }

func TestLoadOlderReturnsWholeDays(t *testing.T) {
	dir := t.TempDir()
	// A legacy day file, then session files: two on the 23rd, one running
	// from the 23rd into the 24th, and one more on the 24th.
	os.WriteFile(filepath.Join(dir, "2026-09-22.log"), []byte(logstore.Header+"\n"+
		logstore.Format(logstore.Entry{Time: at(22, 12), Dir: logstore.In, Text: "22 legacy"})+"\n"), 0o600)
	w := logstore.NewWriter(dir, "kit")
	for _, sess := range [][]logstore.Entry{
		{{Time: at(23, 9), Dir: logstore.In, Text: "23 a"}},
		{{Time: at(23, 12), Dir: logstore.In, Text: "23 b"}},
		{{Time: at(23, 22), Dir: logstore.In, Text: "23 c"}, {Time: at(24, 2), Dir: logstore.In, Text: "24 a"}},
		{{Time: at(24, 12), Dir: logstore.In, Text: "24 b"}},
	} {
		for _, e := range sess {
			w.Append(e)
		}
		w.NewSession()
	}
	w.Close()
	r, err := NewReader(dir, "kit")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for {
		es, day, ok, err := r.LoadOlder()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		var texts []string
		for _, e := range es {
			texts = append(texts, e.Text)
		}
		got = append(got, day+": "+strings.Join(texts, ", "))
	}
	want := []string{"2026-09-24: 24 a, 24 b", "2026-09-23: 23 a, 23 b, 23 c", "2026-09-22: 22 legacy"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("days =\n%q\nwant\n%q", got, want)
	}
	if !r.Exhausted() {
		t.Error("not exhausted")
	}
}

func TestExhaustedOnlyWhenBufferEmpty(t *testing.T) {
	dir := t.TempDir()
	w := logstore.NewWriter(dir, "kit")
	w.Append(logstore.Entry{Time: at(23, 22), Dir: logstore.In, Text: "23"})
	w.Append(logstore.Entry{Time: at(24, 2), Dir: logstore.In, Text: "24"}) // same session, next day
	w.Close()
	r, _ := NewReader(dir, "kit")
	if _, day, _, _ := r.LoadOlder(); day != "2026-09-24" || r.Exhausted() {
		t.Fatalf("day %s, exhausted %v; the 23rd is still to come", day, r.Exhausted())
	}
	if _, day, _, _ := r.LoadOlder(); day != "2026-09-23" || !r.Exhausted() {
		t.Errorf("day %s, exhausted %v", day, r.Exhausted())
	}
}

func TestNoLogs(t *testing.T) {
	r, err := NewReader(filepath.Join(t.TempDir(), "nobody"), "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := r.LoadOlder(); ok || !r.Exhausted() {
		t.Error("expected empty, exhausted reader")
	}
}
