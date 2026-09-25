package conn

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
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
			cut := runeCut(s.buf)
			out = appendLine(out, s.buf[:cut])
			s.buf = append(s.buf[:0], s.buf[cut:]...)
		}
	}
	return out
}

// runeCut returns where to force-cut buf: its length, or the start of a
// trailing UTF-8 sequence that is still incomplete, so the cut never
// splits a character and both halves still decode as UTF-8.
func runeCut(buf []byte) int {
	for i := len(buf) - 1; i >= max(0, len(buf)-utf8.UTFMax); i-- {
		if utf8.RuneStart(buf[i]) {
			if i > 0 && !utf8.FullRune(buf[i:]) {
				return i
			}
			break
		}
	}
	return len(buf)
}

// partial returns the decoded unterminated line currently buffered, or ""
// if there is none (or it is an MCP message). The buffer is not consumed:
// when the rest of the line arrives, push returns the whole line.
func (s *splitter) partial() string {
	if len(s.buf) == 0 {
		return ""
	}
	out := appendLine(nil, s.buf)
	if len(out) == 0 {
		return ""
	}
	return out[0]
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
// as Windows-1252: Latin-1 plus smart quotes and dashes in 0x80–0x9F, which
// is what older MUCKs actually relay from Windows clients.
func decode(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	out, err := charmap.Windows1252.NewDecoder().Bytes(raw)
	if err != nil { // cannot happen: every byte maps to a rune
		return strings.ToValidUTF8(string(raw), "\uFFFD")
	}
	return string(out)
}
