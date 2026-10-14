package webapi

import (
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
)

// Settings is a world's or character's settings, for the page's editor:
// what its file sets, and what it inherits for what it leaves unset.
// World "" is a new world; Char "" the world itself. Name is the
// character's, for the form's title.
type Settings struct {
	World, Char, Name string
	WorldSettings     *config.WorldSettings     `json:",omitempty"`
	CharSettings      *config.CharacterSettings `json:",omitempty"`
	Inherited         config.Inherited
}

// settings reads what the editor for world's char starts from. One that
// can't be read is said on the status line, as the TUI's, and there's
// nothing to send.
func (l *Loop) settings(world, char string) []any {
	a := l.a
	out := Settings{World: world, Char: char}
	var err error
	switch {
	case world == "":
		var s config.WorldSettings
		s, out.Inherited, err = a.NewWorldSettings()
		out.WorldSettings = &s
	case char == "":
		var s config.WorldSettings
		s, out.Inherited, err = a.WorldSettings(world)
		out.WorldSettings = &s
	default:
		var s config.CharacterSettings
		s, out.Inherited, err = a.CharacterSettings(world, char)
		out.CharSettings = &s
		if ch, ok := a.Find(app.Key(world, char)); ok {
			out.Name = ch.Name
		}
	}
	if err != nil {
		a.SetStatus(true, err.Error())
		return nil
	}
	return []any{out}
}
