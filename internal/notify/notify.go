// Package notify builds desktop notifications as terminal escape
// sequences: OSC 9 (iTerm2, kitty, Ghostty, WezTerm, foot, Blink) and the
// bell, optionally wrapped for tmux passthrough.
package notify

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/str"
)

// Level is how much a character notifies while you're away.
type Level string

const (
	All       Level = "all"       // every incoming, non-quiet line
	First     Level = "first"     // the first line since you left, then attention lines
	Attention Level = "attention" // lines an attention rule matched
	None      Level = "none"
)

// ParseLevel checks a level name.
func ParseLevel(s string) (Level, error) {
	switch l := Level(s); l {
	case All, First, Attention, None:
		return l, nil
	}
	return "", errors.New(str.NotifyBadLevel())
}

// Method is how a notification reaches the terminal.
type Method string

const (
	OSC  Method = "osc"  // OSC 9 with the message
	Bell Method = "bell" // a bare BEL; survives mosh, which strips OSC 9
	Both Method = "both"
)

// ParseMethod checks a method name.
func ParseMethod(s string) (Method, error) {
	switch m := Method(s); m {
	case OSC, Bell, Both:
		return m, nil
	}
	return "", errors.New(str.NotifyBadMethod())
}

// MaxLen is the longest message, in characters.
const MaxLen = 200

// Message is "name: line" with the line made safe to put inside an OSC:
// ANSI sequences and every other control character are removed, and the
// result is cut to MaxLen characters.
func Message(name, line string) string {
	line = ansi.Strip(strings.ToValidUTF8(line, ""))
	line = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, line)
	msg := []rune(name + ": " + strings.TrimSpace(line))
	if len(msg) > MaxLen {
		msg = append(msg[:MaxLen-1], '…')
	}
	return string(msg)
}

// Encode returns the bytes to write for msg. Inside tmux the OSC is
// wrapped in a DCS passthrough (which needs allow-passthrough on); the
// bell needs no wrapping.
func Encode(msg string, m Method, tmux bool) string {
	var b strings.Builder
	if m == OSC || m == Both {
		b.WriteString(passthrough("\x1b]9;"+msg+"\a", tmux))
	}
	if m == Bell || m == Both {
		b.WriteByte('\a')
	}
	return b.String()
}

// Clipboard returns the bytes that put text on the clipboard of the
// terminal you're looking at, over ssh too: OSC 52. Inside tmux it's
// wrapped like a notification is.
func Clipboard(text string, tmux bool) string {
	return passthrough("\x1b]52;c;"+base64.StdEncoding.EncodeToString([]byte(text))+"\a", tmux)
}

// passthrough wraps seq in tmux's DCS passthrough when inside tmux.
func passthrough(seq string, tmux bool) string {
	if !tmux {
		return seq
	}
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\" //str:ok
}
