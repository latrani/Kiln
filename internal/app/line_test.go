package app

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
)

func testRules(t *testing.T) Rules {
	t.Helper()
	cls, err := classify.New([]config.ClassifyRule{{Tag: "page", Pattern: `^PAGE:`}}, "Kit", nil)
	if err != nil {
		t.Fatal(err)
	}
	return Rules{Classifier: cls, Judge: rules.Judge{Attention: []string{"page"}}}
}

func TestServerLineIsClassified(t *testing.T) {
	r := testRules(t)
	l := r.Line(logstore.Entry{Dir: logstore.In, Text: "PAGE: \x1b[31mhi\x1b[0m\x07"})
	if l.Kind != Server {
		t.Errorf("Kind = %v, want Server", l.Kind)
	}
	if l.Plain() != "PAGE: hi" {
		t.Errorf("Plain = %q", l.Plain())
	}
	if l.Text() != "PAGE: \x1b[31mhi\x1b[0m" {
		t.Errorf("Text = %q, want the server's SGR kept and the bell dropped", l.Text())
	}
	if !slices.Contains(l.TagNames(), "page") || !l.Attention {
		t.Errorf("tags %v attention %v, want page and attention", l.TagNames(), l.Attention)
	}
}

// Lines you sent and Kiln's own lines are never classified, even when
// they'd match.
func TestEchoAndSysAreNotClassified(t *testing.T) {
	r := testRules(t)
	for _, c := range []struct {
		dir  logstore.Dir
		kind Kind
	}{{logstore.Out, Echo}, {logstore.Sys, Sys}} {
		l := r.Line(logstore.Entry{Dir: c.dir, Text: "PAGE: hi"})
		if l.Kind != c.kind || l.Tags != nil || l.Attention || l.Quiet {
			t.Errorf("%c: %+v, want kind %v and no tags or verdict", c.dir, l, c.kind)
		}
	}
}

func TestDaysDividesAndSkipsEcho(t *testing.T) {
	r := testRules(t)
	d1 := time.Date(2026, 9, 24, 23, 0, 0, 0, time.Local)
	d2 := d1.Add(2 * time.Hour)
	es := []logstore.Entry{
		{Dir: logstore.In, Time: d1, Text: "a"},
		{Dir: logstore.Out, Time: d1, Text: "sent"},
		{Dir: logstore.In, Time: d2, Text: "b"},
	}
	kinds := func(ls []Line) []Kind {
		var ks []Kind
		for _, l := range ls {
			ks = append(ks, l.Kind)
		}
		return ks
	}
	if got, want := kinds(r.Days(es, false, false)), []Kind{Server, Day, Server}; !slices.Equal(got, want) {
		t.Errorf("Days(echo off, mid-day) = %v, want %v", got, want)
	}
	if got, want := kinds(r.Days(es, true, true)), []Kind{Day, Server, Echo, Day, Server}; !slices.Equal(got, want) {
		t.Errorf("Days(echo on, starts day) = %v, want %v", got, want)
	}
	if ls := r.Days(es, false, true); ls[0].Day != "2026-09-24" || ls[2].Day != DayOf(d2) {
		t.Errorf("day dividers say %q and %q", ls[0].Day, ls[2].Day)
	}
}

func TestRelineUsesNewRules(t *testing.T) {
	old := testRules(t)
	l := old.Line(logstore.Entry{Dir: logstore.In, Text: "PAGE: hi"})
	cls, err := classify.New(nil, "Kit", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := (Rules{Classifier: cls}).Reline(l); got.Attention || slices.Contains(got.TagNames(), "page") {
		t.Errorf("Reline kept the old rules' verdict: %+v", got)
	}
	day := Line{Kind: Day, Day: "2026-09-24"}
	if got := old.Reline(day); got.Kind != Day || got.Day != day.Day {
		t.Errorf("Reline(day) = %+v, want it unchanged", got)
	}
}

// A line keeps its entry, not copies of its text: Text and Plain are
// worked out from the entry when asked.
func TestLineStoresNoTextCopies(t *testing.T) {
	typ := reflect.TypeOf(Line{})
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Type.Kind() == reflect.String && f.Name != "Day" {
			t.Errorf("Line stores a string field %s", f.Name)
		}
	}
}

func TestRelineKeepsChrome(t *testing.T) {
	r := testRules(t)
	end := Line{Kind: HistoryEnd, Entry: logstore.Entry{Time: time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)}}
	if got := r.Reline(end); got.Kind != HistoryEnd || !got.Entry.Time.Equal(end.Entry.Time) {
		t.Errorf("Reline(history end) = %+v", got)
	}
}
