package app

import (
	"slices"
	"testing"
)

func TestHistoryBrowsesAndBringsTheDraftBack(t *testing.T) {
	var h History
	if _, ok := h.Older("draft"); ok {
		t.Error("Older with no history moved")
	}
	h.Add("look")
	h.Add("look") // a repeat of the last isn't recorded again
	h.Add("")
	h.Add("say hi")
	if got := h.Lines(); !slices.Equal(got, []string{"look", "say hi"}) {
		t.Errorf("Lines = %q", got)
	}
	if v, ok := h.Older("half typed"); !ok || v != "say hi" {
		t.Errorf("Older = %q, %v", v, ok)
	}
	if v, _ := h.Older("say hi"); v != "look" || h.Pos() != 0 {
		t.Errorf("Older again = %q at %d", v, h.Pos())
	}
	if _, ok := h.Older("look"); ok {
		t.Error("Older past the oldest moved")
	}
	h.Newer()
	if v, ok := h.Newer(); !ok || v != "half typed" {
		t.Errorf("Newer past the newest = %q, %v; want the draft back", v, ok)
	}
	if _, ok := h.Newer(); ok {
		t.Error("Newer while not browsing moved")
	}
	h.Older("x")
	h.Stop()
	if h.Pos() != len(h.Lines()) {
		t.Error("Stop didn't stop browsing")
	}
}
