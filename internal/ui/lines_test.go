package ui

import (
	"testing"

	"github.com/latrani/Kiln/internal/app"
)

// seqAfter and logSeqAfter number a line a test adds after ls the way
// the core numbers a new line: one above the last, 0 for the first.
func seqAfter(ls []*app.Line) int {
	if len(ls) == 0 {
		return 0
	}
	return ls[len(ls)-1].Seq + 1
}

func logSeqAfter(ls []*app.LogLine) int {
	if len(ls) == 0 {
		return 0
	}
	return ls[len(ls)-1].Seq + 1
}

// Lines tests add count on from the lines already there, as the core's do.
func TestTestLinesAreNumbered(t *testing.T) {
	h := newHarness(t, map[string]string{"fm": fmWorld})
	cs := h.m.chars["fm/kit"]
	base := seqAfter(cs.Lines)
	for range 3 {
		addLine(cs, "x", &app.Line{Kind: app.Server})
	}
	for i, l := range cs.Lines[len(cs.Lines)-3:] {
		if l.Seq != base+i {
			t.Errorf("line %d numbered %d, want %d", i, l.Seq, base+i)
		}
	}
	if got := logSeqAfter([]*app.LogLine{{Line: app.Line{Seq: -1}}}); got != 0 {
		t.Errorf("after -1: %d", got)
	}
}
