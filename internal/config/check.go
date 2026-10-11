package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/str"
)

// FieldError is a setting that can't be saved. Field is its config key
// ("id" for a new world's id, "name" for a new character's name), so a
// form can put Msg by the field.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Msg }

func fieldErr(field, msg string) error { return &FieldError{Field: field, Msg: msg} }

// checkHost and checkPort are a world's server; see CheckWorld.
func checkHost(host string) error {
	if host == "" || strings.ContainsFunc(host, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return fieldErr("host", str.ConfigBadHost()) //str:ok
	}
	return nil
}

func checkPort(port int) error {
	if port < 1 || port > 65535 {
		return fieldErr("port", str.ConfigPortRange()) //str:ok
	}
	return nil
}

func checkNotify(n *string) error {
	if n == nil {
		return nil
	}
	if _, err := notify.ParseLevel(*n); err != nil {
		return fieldErr("notify", err.Error()) //str:ok
	}
	return nil
}

// CheckWorld reports every problem with s as a world's settings, each a
// *FieldError, joined: the port, the host, each pack in use (a pack id,
// and one in dir/packs), max_line_bytes, newline_mode and notify. nil if
// there are none.
func CheckWorld(dir string, s WorldSettings) error {
	errs := []error{checkPort(s.Port), checkHost(s.Host)}
	for _, p := range s.Use {
		if !idRE.MatchString(p) {
			errs = append(errs, fieldErr("use", str.ConfigBadPackId(p))) //str:ok
		} else if _, err := os.Stat(filepath.Join(dir, "packs", p+".toml")); err != nil {
			errs = append(errs, fieldErr("use", str.ConfigUnknownPack(p))) //str:ok
		}
	}
	if s.MaxLineBytes != nil && *s.MaxLineBytes <= 0 {
		errs = append(errs, fieldErr("max_line_bytes", str.ConfigBadMaxLineBytes())) //str:ok
	}
	if s.NewlineMode != nil && *s.NewlineMode != "batch" && *s.NewlineMode != "flatten" { //str:ok
		errs = append(errs, fieldErr("newline_mode", str.ConfigBadNewlineMode())) //str:ok
	}
	errs = append(errs, checkNotify(s.Notify))
	return errors.Join(errs...)
}

// CheckCharacter reports every problem with s as a character's settings:
// its aliases, and notify.
func CheckCharacter(s CharacterSettings) error {
	var errs []error
	for _, a := range s.Aliases {
		if err := NameChars(a); err != nil {
			errs = append(errs, fieldErr("aliases", err.Error())) //str:ok
			break
		}
	}
	errs = append(errs, checkNotify(s.Notify))
	return errors.Join(errs...)
}

// CheckNewWorld reports whether id can be a new world's id in dir: an id
// that isn't taken, ignoring case.
func CheckNewWorld(dir, id string) error {
	if !idRE.MatchString(id) {
		return fieldErr("id", str.ConfigWorldIdChars()) //str:ok
	}
	existing, err := filepath.Glob(filepath.Join(dir, "worlds", "*.toml"))
	if err != nil {
		return err
	}
	for _, p := range existing {
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(p), ".toml"), id) {
			return fieldErr("id", str.ConfigWorldExists(id)) //str:ok
		}
	}
	return nil
}
