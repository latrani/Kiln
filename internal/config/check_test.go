package config

import (
	"errors"
	"slices"
	"testing"
)

// fields lists the fields err's FieldErrors name, in order.
func fields(err error) []string {
	var out []string
	var walk func(error)
	walk = func(e error) {
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, x := range j.Unwrap() {
				walk(x)
			}
			return
		}
		var fe *FieldError
		if errors.As(e, &fe) {
			out = append(out, fe.Field)
		}
	}
	if err != nil {
		walk(err)
	}
	return out
}

func TestCheckWorldNamesEveryBadField(t *testing.T) {
	dir := editDir(t, uglyWorld)
	zero, bogus, sideways := 0, "bogus", "sideways"
	s := WorldSettings{Host: "a b", Port: 0, Use: []string{"fuzzball", "no-such", "bad id"},
		MaxLineBytes: &zero, NewlineMode: &sideways, Notify: &bogus}
	want := []string{"port", "host", "use", "use", "max_line_bytes", "newline_mode", "notify"}
	if got := fields(CheckWorld(dir, s)); !slices.Equal(got, want) {
		t.Errorf("fields %q, want %q", got, want)
	}
	good, _, _ := ReadWorld(dir, "fm")
	if err := CheckWorld(dir, good); err != nil {
		t.Errorf("a world as read is refused: %v", err)
	}
}

func TestCheckCharacter(t *testing.T) {
	bogus := "bogus"
	if got := fields(CheckCharacter(CharacterSettings{Aliases: []string{"Kitty", "#K"}, Notify: &bogus})); !slices.Equal(got, []string{"aliases", "notify"}) {
		t.Errorf("fields %q", got)
	}
	if err := CheckCharacter(CharacterSettings{Aliases: []string{"Kitty"}}); err != nil {
		t.Error(err)
	}
}

// A new world's problems come in the order the TUI always showed the
// first of: port, the id's characters, host, the id being taken, then
// the rest.
func TestCheckNewWorld(t *testing.T) {
	dir := editDir(t, uglyWorld)
	for _, c := range []struct {
		id   string
		s    WorldSettings
		want []string
	}{
		{"", WorldSettings{Port: 1}, []string{"id", "host"}},
		{"a b", WorldSettings{Port: 0, Host: "h"}, []string{"port", "id"}},
		{"FM", WorldSettings{Port: 1, Host: ""}, []string{"host", "id"}},
		{"FM", WorldSettings{Port: 1, Host: "h", Use: []string{"no-such"}}, []string{"id", "use"}},
	} {
		if got := fields(CheckNewWorld(dir, c.id, c.s)); !slices.Equal(got, c.want) {
			t.Errorf("%q %+v: fields %q, want %q", c.id, c.s, got, c.want)
		}
	}
	if err := CheckNewWorld(dir, "new", WorldSettings{Host: "h", Port: 1}); err != nil {
		t.Error(err)
	}
}

func TestWritesCheckFirst(t *testing.T) {
	dir := editDir(t, uglyWorld)
	s, _, _ := ReadWorld(dir, "fm")
	s.Host = "a b"
	if got := fields(WriteWorld(dir, "fm", s)); !slices.Equal(got, []string{"host"}) {
		t.Errorf("WriteWorld fields %q", got)
	}
	c, _, _ := ReadCharacter(dir, "fm", "Kit")
	c.Aliases = []string{"#K"}
	if got := fields(WriteCharacter(dir, "fm", "Kit", c)); !slices.Equal(got, []string{"aliases"}) {
		t.Errorf("WriteCharacter fields %q", got)
	}
	if readWorld(t, dir) != uglyWorld {
		t.Error("a refused save changed the file")
	}
	_, err := AddCharacter(dir, "fm", "me")
	if got := fields(err); !slices.Equal(got, []string{"name"}) {
		t.Errorf("AddCharacter fields %q", got)
	}
	if got := fields(AddWorld(dir, "fm", "h", 1, false)); !slices.Equal(got, []string{"id"}) {
		t.Errorf("AddWorld fields %q", got)
	}
}
