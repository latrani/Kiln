package history

import (
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/logstore"
)

func TestLoadOlderWalksBackward(t *testing.T) {
	root := t.TempDir()
	w := logstore.NewWriter(root, "fm", "kit")
	for _, d := range []int{22, 24, 23} {
		w.Append(logstore.Entry{Time: time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC), Dir: logstore.In, Text: "day"})
	}
	w.Close()
	r, err := NewReader(root, "fm", "kit")
	if err != nil {
		t.Fatal(err)
	}
	if r.Oldest() != "2026-09-22" {
		t.Errorf("Oldest = %q", r.Oldest())
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
		if len(es) != 1 {
			t.Errorf("%s: %d entries", day, len(es))
		}
		got = append(got, day)
	}
	if want := "2026-09-24 2026-09-23 2026-09-22"; join(got) != want {
		t.Errorf("days = %q, want %q", join(got), want)
	}
	if !r.Exhausted() {
		t.Error("not exhausted")
	}
}

func TestNoLogs(t *testing.T) {
	r, err := NewReader(t.TempDir(), "fm", "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := r.LoadOlder(); ok || !r.Exhausted() || r.Oldest() != "" {
		t.Error("expected empty, exhausted reader")
	}
}

func join(xs []string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}
