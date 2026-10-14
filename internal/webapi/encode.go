// Package webapi is Kiln's core behind a page: a loop that owns the core,
// takes the page's actions as JSON, and hands back what changed each turn
// as one JSON batch. It's plain Go, tested natively; the wasm build
// connects it to the page.
package webapi

import (
	"bytes"
	"encoding/json"
	"reflect"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/str"
)

// Line is a line as the page gets it: its text as spans (see Span) and
// as plain text, and the day and time as people read them.
type Line struct {
	Seq      int
	Kind     string // server, echo, sys, day, history_end
	Day      string
	DayLabel string // the day as people read it; "" for none
	Time     time.Time
	Clock    string // the time of day; "" for none
	Out      bool   // something you sent
	Plain    string // the text, unstyled: the spans' texts joined
	Spans    []Span
	Tags     []classify.Tag
	Priority bool
	Quiet    bool
}

// lineOf is l for the page, its server line's tags drawn with hl (nil
// for none).
func lineOf(l app.Line, hl *rules.Highlighter) Line {
	out := Line{Seq: l.Seq, Kind: kindName(l.Kind), Day: l.Day, Time: l.Entry.Time, Out: l.Entry.Dir == logstore.Out,
		Tags: l.Tags, Priority: l.Priority, Quiet: l.Quiet}
	if l.Day != "" {
		out.DayLabel = app.DayLabel(l.Day)
	}
	if !l.Entry.Time.IsZero() {
		out.Clock = app.TimeOfDay(l.Entry.Time)
	}
	text, tags := l.Text(), l.Tags
	switch l.Kind {
	case app.Day:
		text = out.DayLabel
	case app.HistoryEnd:
		text = str.ScrollbackHistoryEnds(l.Entry.Time.Format(str.DateDayTime()))
	}
	if l.Kind != app.Server {
		hl, tags = nil, nil
	}
	out.Spans = spansOf(text, hl, tags)
	for _, s := range out.Spans {
		out.Plain += s.Text
	}
	return out
}

// kindName and opName are a line's kind and a Lines event's op as the
// page gets them: protocol vocabulary, never shown.
func kindName(k app.Kind) string {
	switch k {
	case app.Echo:
		return "echo" //str:ok
	case app.Sys:
		return "sys" //str:ok
	case app.Day:
		return "day" //str:ok
	case app.HistoryEnd:
		return "history_end" //str:ok
	}
	return "server" //str:ok
}

func opName(o feed.Op) string {
	switch o {
	case feed.Replace:
		return "replace" //str:ok
	case feed.Append:
		return "append" //str:ok
	case feed.Prepend:
		return "prepend" //str:ok
	}
	return "reset" //str:ok
}

func linesOf(ls []app.Line, hl *rules.Highlighter) []Line {
	out := make([]Line, len(ls))
	for i, l := range ls {
		out[i] = lineOf(l, hl)
	}
	return out
}

// Encode is evs as a JSON array of objects, each with "type" (its Go
// type's name) first. hl gives each character's highlighter for its
// lines' tags; it may be nil, or give nil, for no tag styles.
func Encode(evs []any, hl func(key string) *rules.Highlighter) ([]byte, error) {
	look := func(k string) *rules.Highlighter {
		if hl == nil {
			return nil
		}
		return hl(k)
	}
	var b bytes.Buffer
	b.WriteByte('[')
	for i, e := range evs {
		if i > 0 {
			b.WriteByte(',')
		}
		typ := reflect.TypeOf(e).Name()
		var extra map[string]string
		switch v := e.(type) {
		case feed.Lines:
			e = struct {
				Key   string
				Epoch int
				Op    string
				Lines []Line
			}{v.Key, v.Epoch, opName(v.Op), linesOf(v.Lines, look(v.Key))}
		case feed.LogLines:
			e = struct {
				Key   string
				Epoch int
				Op    string
				Lines []Line
			}{v.Key, v.Epoch, opName(v.Op), linesOf(v.Lines, look(v.Key))}
		case feed.Input:
			e = struct {
				Key, Text string
				Ack       int
				Prompt    []Span
				NeedPW    bool
			}{v.Key, v.Text, v.Ack, spansOf(v.Prompt, nil, nil), v.NeedPW}
		case feed.Effect:
			extra = map[string]string{"effect": reflect.TypeOf(v.Effect).Name()} //str:ok
			e = v.Effect
			if n, ok := v.Effect.(app.Notify); ok {
				n.Body = notify.Clean(n.Body)
				e = n
			}
		}
		body, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		b.WriteString(`{"type":`) //str:ok
		t, _ := json.Marshal(typ)
		b.Write(t)
		for k, v := range extra {
			kb, _ := json.Marshal(k)
			vb, _ := json.Marshal(v)
			b.WriteByte(',')
			b.Write(kb)
			b.WriteByte(':')
			b.Write(vb)
		}
		if len(body) > 2 { // not "{}"
			b.WriteByte(',')
			b.Write(body[1 : len(body)-1])
		}
		b.WriteByte('}')
	}
	b.WriteByte(']')
	return b.Bytes(), nil
}

// Saved says a settings action worked; Op is the action's do, and Key the
// character an add_character made ("world/id"), for the page to open.
// World and Char are the action's: the editor it answers.
type Saved struct{ Op, Key, World, Char string }

// Problems is what was wrong with a settings action: every problem, each
// naming its field ("" for one about no field). World and Char are the
// action's: the editor it answers. A settings read that failed (Op
// "settings") has none; the status line says why.
type Problems struct {
	Op, World, Char string
	Problems        []Problem
}

// Problem is one of Problems.
type Problem struct{ Field, Msg string }
