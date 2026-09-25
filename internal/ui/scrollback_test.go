package ui

import (
	"fmt"
	"reflect"
	"testing"
)

func sb(width int, lines ...string) *Scrollback {
	s := &Scrollback{}
	s.SetWidth(width)
	for _, l := range lines {
		s.Append(l)
	}
	return s
}

func TestScrollbackViewPadsTop(t *testing.T) {
	got := sb(20, "one", "two").View(4)
	want := []string{"", "", "one", "two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("View = %q, want %q", got, want)
	}
}

func TestScrollbackViewShowsNewest(t *testing.T) {
	got := sb(20, "1", "2", "3", "4", "5").View(2)
	if !reflect.DeepEqual(got, []string{"4", "5"}) {
		t.Errorf("View = %q", got)
	}
}

func TestScrollbackWrapsLongLines(t *testing.T) {
	got := sb(10, "short", "the quick brown fox").View(3)
	want := []string{"short", "the quick", "brown fox"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("View = %q, want %q", got, want)
	}
}

func TestScrollbackScrollAndClamp(t *testing.T) {
	s := sb(20, "1", "2", "3", "4", "5")
	s.ScrollUp(2)
	if got := s.View(2); !reflect.DeepEqual(got, []string{"2", "3"}) {
		t.Errorf("after ScrollUp(2) = %q", got)
	}
	s.ScrollUp(100)
	if got := s.View(2); !reflect.DeepEqual(got, []string{"1", "2"}) {
		t.Errorf("clamped view = %q", got)
	}
	s.ScrollDown(100)
	if s.Scrolled() {
		t.Error("ScrollDown past bottom should return to live")
	}
}

func TestScrollbackStaysPutWhileScrolledAndCountsUnseen(t *testing.T) {
	s := sb(20, "1", "2", "3", "4")
	s.ScrollUp(1)
	before := s.View(2)
	s.Append("5")
	s.Append("6")
	if got := s.View(2); !reflect.DeepEqual(got, before) {
		t.Errorf("view moved from %q to %q", before, got)
	}
	if s.Unseen() != 2 {
		t.Errorf("Unseen = %d, want 2", s.Unseen())
	}
	s.ToBottom()
	if got := s.View(2); !reflect.DeepEqual(got, []string{"5", "6"}) || s.Unseen() != 0 {
		t.Errorf("ToBottom view = %q unseen=%d", got, s.Unseen())
	}
}

func TestScrollbackPrompt(t *testing.T) {
	s := sb(20, "Welcome")
	s.SetPrompt("Password: ")
	if got := s.View(2); !reflect.DeepEqual(got, []string{"Welcome", "Password: "}) {
		t.Errorf("View = %q", got)
	}
	s.Append("Hello")
	if got := s.View(2); !reflect.DeepEqual(got, []string{"Welcome", "Hello"}) {
		t.Errorf("prompt not cleared: %q", got)
	}
}

func TestScrollbackOnlyWrapsVisibleLines(t *testing.T) {
	s := &Scrollback{}
	s.SetWidth(40)
	for i := 0; i < 100000; i++ {
		s.Append(fmt.Sprintf("line %d", i))
	}
	s.View(10)
	wrapped := 0
	for i := range s.lines {
		if s.lines[i].rows != nil {
			wrapped++
		}
	}
	if wrapped > 10 {
		t.Errorf("wrapped %d lines to show 10 rows", wrapped)
	}
}

func TestScrollbackRewrapsOnWidthChange(t *testing.T) {
	s := sb(40, "the quick brown fox")
	if got := s.View(1); got[0] != "the quick brown fox" {
		t.Errorf("wide = %q", got)
	}
	s.SetWidth(10)
	if got := s.View(2); !reflect.DeepEqual(got, []string{"the quick", "brown fox"}) {
		t.Errorf("narrow = %q", got)
	}
}
