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
)

// Line is a line as the page gets it: Text sanitized, the server's SGR
// kept for the page to style.
type Line struct {
	Seq       int
	Kind      app.Kind
	Day       string
	Time      time.Time
	Out       bool // something you sent
	Text      string
	Tags      []classify.Tag
	Attention bool
	Quiet     bool
}

func lineOf(l app.Line) Line {
	return Line{Seq: l.Seq, Kind: l.Kind, Day: l.Day, Time: l.Entry.Time, Out: l.Entry.Dir == logstore.Out,
		Text: l.Text(), Tags: l.Tags, Attention: l.Attention, Quiet: l.Quiet}
}

func linesOf(ls []app.Line) []Line {
	out := make([]Line, len(ls))
	for i, l := range ls {
		out[i] = lineOf(l)
	}
	return out
}

// Encode is evs as a JSON array of objects, each with "type" (its Go
// type's name) first.
func Encode(evs []any) ([]byte, error) {
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
				Op    feed.Op
				Lines []Line
			}{v.Key, v.Epoch, v.Op, linesOf(v.Lines)}
		case feed.LogLines:
			e = struct {
				Key   string
				Epoch int
				Op    feed.Op
				Lines []Line
			}{v.Key, v.Epoch, v.Op, linesOf(v.Lines)}
		case feed.Effect:
			extra = map[string]string{"effect": reflect.TypeOf(v.Effect).Name()} //str:ok
			e = v.Effect
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

// Saved says a settings action worked; Op is the action's do.
type Saved struct{ Op string }

// Problems is what was wrong with a settings action: every problem, each
// naming its field ("" for one about no field).
type Problems struct {
	Op       string
	Problems []Problem
}

// Problem is one of Problems.
type Problem struct{ Field, Msg string }
