package webapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/logstore"
)

func decode(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return out
}

func TestEncodeTagsEachEventWithItsType(t *testing.T) {
	when := time.Date(2026, 9, 24, 21, 0, 0, 0, time.UTC)
	l := app.Line{Kind: app.Server, Seq: 3, Day: "2026-09-24", Entry: logstore.Entry{Time: when, Dir: logstore.In, Text: "\x1b[1mhi\x1b[0m\x07"}}
	b, err := Encode([]any{
		feed.Active{Key: "fm/kit"},
		feed.Lines{Key: "fm/kit", Epoch: 2, Op: feed.Append, Lines: []app.Line{l}},
		feed.Effect{Effect: app.Copy{Text: "x"}},
		Problems{Op: "save_world", Problems: []Problem{{Field: "port", Msg: "bad"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	evs := decode(t, b)
	if evs[0]["type"] != "Active" || evs[0]["Key"] != "fm/kit" {
		t.Errorf("active %v", evs[0])
	}
	lines := evs[1]["Lines"].([]any)
	got := lines[0].(map[string]any)
	if evs[1]["type"] != "Lines" || got["Seq"] != 3.0 || got["Text"] != "\x1b[1mhi\x1b[0m" || got["Out"] != false {
		t.Errorf("lines %v", evs[1])
	}
	if evs[2]["type"] != "Effect" || evs[2]["effect"] != "Copy" || evs[2]["Text"] != "x" {
		t.Errorf("effect %v", evs[2])
	}
	if evs[3]["type"] != "Problems" {
		t.Errorf("problems %v", evs[3])
	}
}

func TestEncodeNamesKindsAndOps(t *testing.T) {
	kinds := map[app.Kind]string{app.Server: "server", app.Echo: "echo", app.Sys: "sys", app.Day: "day", app.HistoryEnd: "history_end"}
	ops := map[feed.Op]string{feed.Reset: "reset", feed.Replace: "replace", feed.Append: "append", feed.Prepend: "prepend"}
	for k, name := range kinds {
		for o, op := range ops {
			b, err := Encode([]any{feed.Lines{Key: "fm/kit", Op: o, Lines: []app.Line{{Kind: k}}}})
			if err != nil {
				t.Fatal(err)
			}
			ev := decode(t, b)[0]
			if ev["Op"] != op || ev["Lines"].([]any)[0].(map[string]any)["Kind"] != name {
				t.Errorf("%v/%v: %v", k, o, ev)
			}
		}
	}
}

func TestEffectFieldsDontCollide(t *testing.T) {
	for _, e := range []app.Effect{app.Notify{}, app.Copy{}, app.SaveFile{}, app.Do{}, app.Quit{}} {
		typ := reflect.TypeOf(e)
		for i := range typ.NumField() {
			if n := typ.Field(i).Name; strings.EqualFold(n, "type") || strings.EqualFold(n, "effect") {
				t.Errorf("%s.%s would collide with the envelope", typ.Name(), n)
			}
		}
	}
}

func TestZeroConnectedAtIsLeftOut(t *testing.T) {
	b, err := Encode([]any{feed.Char{Key: "fm/kit"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decode(t, b)[0]["ConnectedAt"]; ok {
		t.Error("a zero ConnectedAt went out")
	}
}
