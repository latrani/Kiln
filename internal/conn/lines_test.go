package conn

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitterLines(t *testing.T) {
	var s splitter
	got := s.push([]byte("one\r\ntwo\nthr"))
	if want := []string{"one", "two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	got = s.push([]byte("ee\r\n"))
	if want := []string{"three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSplitterUTF8SplitAcrossPushes(t *testing.T) {
	var s splitter
	fox := []byte("🦊 hi\n")
	var got []string
	for _, b := range fox {
		got = append(got, s.push([]byte{b})...)
	}
	if len(got) != 1 || got[0] != "🦊 hi" {
		t.Errorf("got %q", got)
	}
}

func TestSplitterLatin1Fallback(t *testing.T) {
	var s splitter
	got := s.push([]byte{'c', 'a', 'f', 0xe9, '\n'}) // "café" in Latin-1
	if len(got) != 1 || got[0] != "café" {
		t.Errorf("got %q", got)
	}
}

func TestSplitterWindows1252SmartQuotes(t *testing.T) {
	var s splitter
	// "don’t — “ok”" as a Windows client would send it.
	got := s.push([]byte("don\x92t \x97 \x93ok\x94\n"))
	if len(got) != 1 || got[0] != "don’t — “ok”" {
		t.Errorf("got %q", got)
	}
}

func TestSplitterMCP(t *testing.T) {
	var s splitter
	got := s.push([]byte("#$#mcp version: 2.1 to: 2.1\n#$\"#$#not mcp\nnormal\n"))
	if want := []string{"#$#not mcp", "normal"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSplitterFlushAndBlankLines(t *testing.T) {
	var s splitter
	got := s.push([]byte("\r\n\npartial"))
	if want := []string{"", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
	if got := s.flush(); !reflect.DeepEqual(got, []string{"partial"}) {
		t.Errorf("flush = %q", got)
	}
	if got := s.flush(); got != nil {
		t.Errorf("second flush = %q", got)
	}
}

func TestSplitterBoundsRunawayLine(t *testing.T) {
	var s splitter
	got := s.push([]byte(strings.Repeat("x", maxLine+10)))
	if len(got) != 1 || len(got[0]) != maxLine {
		t.Errorf("got %d lines", len(got))
	}
}

func TestSplitterRunawayCutKeepsRunesWhole(t *testing.T) {
	var s splitter
	// "é" is 2 bytes; the prefix puts its first byte at the last slot.
	in := strings.Repeat("x", maxLine-1) + "é" + strings.Repeat("y", 3) + "\n"
	got := s.push([]byte(in))
	want := []string{strings.Repeat("x", maxLine-1), "éyyy"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %d lines, first len %d", len(got), len(got[0]))
	}
}

func TestSplitterPartialDoesNotConsume(t *testing.T) {
	var s splitter
	s.push([]byte("Password: "))
	if got := s.partial(); got != "Password: " {
		t.Errorf("partial = %q", got)
	}
	if got := s.push([]byte("\r\n")); !reflect.DeepEqual(got, []string{"Password: "}) {
		t.Errorf("completed line = %q", got)
	}
	if got := s.partial(); got != "" {
		t.Errorf("partial after completion = %q", got)
	}
}

func TestSplitterPartialHidesMCP(t *testing.T) {
	var s splitter
	s.push([]byte("#$#mcp-negotiate"))
	if got := s.partial(); got != "" {
		t.Errorf("partial = %q, want MCP hidden", got)
	}
}
