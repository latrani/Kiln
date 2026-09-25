package conn

import (
	"strings"
	"unicode/utf8"
)

// maxLine bounds a single unterminated line so a server that never sends
// a newline can't grow memory without limit.
const maxLine = 64 * 1024

// splitter turns a byte stream (telnet already removed) into lines.
type splitter struct {
	buf []byte
}

// push appends data and returns every completed line: CR stripped,
// decoded to UTF-8, MCP out-of-band lines removed.
func (s *splitter) push(data []byte) []string {
	var out []string
	for _, b := range data {
		if b == '\n' {
			out = appendLine(out, s.buf)
			s.buf = s.buf[:0]
			continue
		}
		s.buf = append(s.buf, b)
		if len(s.buf) >= maxLine {
			out = appendLine(out, s.buf)
			s.buf = s.buf[:0]
		}
	}
	return out
}

// flush returns any partial line left at end of stream.
func (s *splitter) flush() []string {
	if len(s.buf) == 0 {
		return nil
	}
	out := appendLine(nil, s.buf)
	s.buf = s.buf[:0]
	return out
}

func appendLine(out []string, raw []byte) []string {
	line := strings.ReplaceAll(decode(raw), "\r", "")
	// MCP 2.1: "#$#" lines are out-of-band messages (swallowed);
	// "#$\"" quotes a normal line that happens to start with "#$#".
	if strings.HasPrefix(line, "#$#") {
		return out
	}
	line = strings.TrimPrefix(line, "#$\"")
	return append(out, line)
}

// decode returns raw as a string if it is valid UTF-8, otherwise treats it
// as Latin-1 (what most older MUCKs actually send).
func decode(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	var b strings.Builder
	b.Grow(len(raw) * 2)
	for _, c := range raw {
		b.WriteRune(rune(c))
	}
	return b.String()
}
