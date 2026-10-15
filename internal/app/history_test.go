package app

import (
	"slices"
	"testing"
	"time"
)

func texts(ss []Sent) []string {
	var out []string
	for _, s := range ss {
		out = append(out, s.Text)
	}
	return out
}

func at(min int) time.Time { return now.Add(time.Duration(min) * time.Minute) }

func TestHistoryRecords(t *testing.T) {
	var h History
	h.Add("look", at(0))
	h.Add("look", at(1)) // a repeat of the last isn't recorded again
	h.Add("", at(2))
	h.Add("say hi", at(3))
	h.SetLogged([]Sent{{"older", at(-60)}})
	if got := h.Lines(); !slices.Equal(got, []string{"older", "look", "say hi"}) {
		t.Errorf("Lines = %q", got)
	}
	if h.Shown() != nil || h.IsOpen() {
		t.Error("a closed panel shows lines")
	}
}

func TestHistoryPanelFiltersAndKeepsTheNewestOfEach(t *testing.T) {
	var h History
	h.SetLogged([]Sent{{"say hi", at(-60)}, {"look", at(-50)}})
	h.Add("say bye", at(0))
	h.Add("look", at(1))
	h.Add("SAY hello", at(2))
	h.Open("")
	if got := texts(h.Shown()); !slices.Equal(got, []string{"say hi", "say bye", "look", "SAY hello"}) {
		t.Errorf("Shown = %q", got)
	}
	if s := h.Shown()[2]; !s.At.Equal(at(1)) {
		t.Errorf("look shown at %v, want its newest", s.At)
	}
	if h.Sel() != 3 {
		t.Errorf("Sel = %d, want the newest", h.Sel())
	}
	h.Move(-1)
	h.SetQuery("say")
	if got := texts(h.Shown()); !slices.Equal(got, []string{"say hi", "say bye", "SAY hello"}) {
		t.Errorf("filtered = %q", got)
	}
	if h.Sel() != 2 {
		t.Errorf("Sel = %d after the query changed, want the newest match", h.Sel())
	}
	h.SetQuery("nothing like it")
	if len(h.Shown()) != 0 || h.Sel() != -1 {
		t.Errorf("no match: %q, Sel %d", texts(h.Shown()), h.Sel())
	}
	if _, ok := h.Pick(); ok || !h.IsOpen() {
		t.Error("Pick with nothing shown picked, or closed the panel")
	}
}

func TestHistoryPanelMovesPicksAndCloses(t *testing.T) {
	var h History
	for i, v := range []string{"one", "two", "three"} {
		h.Add(v, at(i))
	}
	h.Open("draft")
	h.SetQuery("") // the draft starts as the query; clear it to see them all
	h.Move(-1)
	h.Move(-5) // stops at the oldest
	if h.Sel() != 0 {
		t.Errorf("Sel = %d, want the oldest", h.Sel())
	}
	h.Move(1)
	if v, ok := h.Pick(); !ok || v != "two" || h.IsOpen() {
		t.Errorf("Pick = %q, %v, open %v", v, ok, h.IsOpen())
	}
	h.Open("draft")
	h.SetQuery("")
	if d, closed := h.Move(1); !closed || d != "draft" || h.IsOpen() {
		t.Errorf("Down past the newest = %q, %v; want closed with the draft", d, closed)
	}
	h.Open("draft")
	h.Select(0)
	h.Select(9) // out of range: no change
	if h.Sel() != -1 {
		t.Errorf("Sel = %d; \"draft\" matches nothing", h.Sel())
	}
	h.SetQuery("")
	h.Select(0)
	if v, _ := h.Pick(); v != "one" {
		t.Errorf("picked %q after selecting the first", v)
	}
}
