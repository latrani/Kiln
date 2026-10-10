package app

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

var (
	worldIDChar = regexp.MustCompile(`^[A-Za-z0-9_-]*$`) //str:ok
	portChar    = regexp.MustCompile(`^[0-9]{0,5}$`)
	numChar     = regexp.MustCompile(`^[0-9]{0,9}$`)
	packsChar   = regexp.MustCompile(`^[A-Za-z0-9_, -]*$`) //str:ok
)

// onlyMatching accepts what re matches, and otherwise says why.
func onlyMatching(re *regexp.Regexp, why string) func(string) error {
	return func(s string) error {
		if !re.MatchString(s) {
			return errors.New(why)
		}
		return nil
	}
}

// What a form field takes as it's typed: each is checked on every
// keystroke, so every start of a good value passes.
var (
	AcceptWorldID   = onlyMatching(worldIDChar, str.EditorWorldIdChars())
	AcceptPort      = onlyMatching(portChar, str.EditorPortNumber())
	AcceptPacks     = onlyMatching(packsChar, str.EditorPacksList())
	AcceptByteCount = onlyMatching(numChar, str.EditorBytesNumber())
)

// AcceptHost takes a host name: no spaces or characters outside ASCII.
func AcceptHost(s string) error {
	if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return errors.New(str.EditorHostSpaces())
	}
	return nil
}

// AcceptAliases takes a list of names a character also goes by.
func AcceptAliases(s string) error {
	for _, a := range List(s) {
		if err := config.NameChars(a); err != nil {
			return err
		}
	}
	return nil
}

// List splits a comma- or space-separated field into its items.
func List(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// ParsePort reads a port number, 1 to 65535.
func ParsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New(str.EditorPortRange())
	}
	return port, nil
}

// DeleteWarning says what deleting world (char "") or its character char
// will do.
func DeleteWarning(world, char string) string {
	if char == "" {
		return str.EditorDeleteWorldWarning(world)
	}
	return str.EditorDeleteCharacterWarning(world + "/" + char)
}

// AddWorld writes a new world with settings s. If its settings can't be
// written, the half-made world is taken back. Nothing reloads: the front
// end does, as after every edit.
func (a *App) AddWorld(id string, s config.WorldSettings) error {
	if err := config.AddWorld(a.d.ConfigDir, id, s.Host, s.Port, s.TLS); err != nil {
		return err
	}
	if err := config.WriteWorld(a.d.ConfigDir, id, s); err != nil {
		config.DeleteWorld(a.d.ConfigDir, id)
		return err
	}
	return nil
}

// SaveWorld writes world's settings.
func (a *App) SaveWorld(world string, s config.WorldSettings) error {
	return config.WriteWorld(a.d.ConfigDir, world, s)
}

// SaveCharacter writes the settings of world's character char.
func (a *App) SaveCharacter(world, char string, s config.CharacterSettings) error {
	return config.WriteCharacter(a.d.ConfigDir, world, char, s)
}

// AddCharacter writes a new character called name to world and returns
// its id.
func (a *App) AddCharacter(world, name string) (string, error) {
	return config.AddCharacter(a.d.ConfigDir, world, name)
}

// DeleteWorld deletes a world with no characters. Logs are never touched.
func (a *App) DeleteWorld(world string) error {
	return config.DeleteWorld(a.d.ConfigDir, world)
}

// DeleteCharacter deletes world's character char from the config, closing
// it first if it's open, then its saved password. Logs are never touched.
// err is the config's; pwErr is a password that couldn't be deleted, for
// the front end to say after it reloads.
func (a *App) DeleteCharacter(world, char string) (pwErr, err error) {
	if err := config.DeleteCharacter(a.d.ConfigDir, world, char); err != nil {
		return nil, err
	}
	a.Close(Key(world, char))
	if a.d.DeletePassword != nil {
		pwErr = a.d.DeletePassword(a.PasswordStore(), world, char)
	}
	return pwErr, nil
}

// CanForgetPasswords reports whether saved passwords can be deleted here.
func (a *App) CanForgetPasswords() bool { return a.d.DeletePassword != nil }

// ForgetPassword deletes world's character char's saved password.
func (a *App) ForgetPassword(world, char string) error {
	return a.d.DeletePassword(a.PasswordStore(), world, char)
}
