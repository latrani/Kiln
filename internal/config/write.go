package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/str"
)

// HighlightTag is the tag /highlight gives lines; the theme styles it.
const HighlightTag = "highlight" //str:ok: config vocabulary

// AppendHighlight adds a literal-text, case-insensitive classify rule,
// tagging matching lines "highlight", to the end of worlds/<world>.toml.
// The theme's [tags] say how highlight looks. Appending is safe because
// TOML table headers are absolute: "[[classify]]" always means the
// world's top-level list, even after a [characters.x] table. Existing
// content and comments are kept.
func AppendHighlight(dir, world, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New(str.ConfigNothingToHighlight())
	}
	if !idRE.MatchString(world) {
		return errors.New(str.ConfigBadWorldIdBare(world))
	}
	pattern, err := tomlString("(?i)" + regexp.QuoteMeta(text))
	if err != nil {
		return err
	}
	rule := fmt.Sprintf("\n# %s\n[[classify]]\ntags = [%q]\npattern = %s\n", //str:ok
		str.ConfigAddedByHighlight(), HighlightTag, pattern)
	path := filepath.Join(dir, "worlds", world+".toml")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(rule)
	return err
}

// tomlString quotes s as a TOML basic string. JSON string escapes
// (\" \\ \n \uXXXX) are all valid TOML escapes. JSON leaves DEL (0x7f)
// raw, but TOML forbids it in a basic string, so it is escaped here.
func tomlString(s string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	out := strings.TrimSuffix(b.String(), "\n")
	return strings.ReplaceAll(out, "\x7f", `\u007f`), nil
}

// NameChars reports whether name, possibly half-typed, uses only what
// Fuzzball allows in a player name: printable ASCII with no spaces, no
// =, & or |, and no leading !, *, # or $.
func NameChars(name string) error {
	if name != "" && strings.ContainsRune("!*#$", rune(name[0])) {
		return errors.New(str.ConfigNameBadStart(rune(name[0])))
	}
	for _, r := range name {
		switch {
		case r == ' ':
			return errors.New(str.ConfigNameHasSpaces())
		case r == '=' || r == '&' || r == '|':
			return errors.New(str.ConfigNameBadChar(r))
		case r <= ' ' || r > '~':
			return errors.New(str.ConfigNameNotAscii())
		}
	}
	return nil
}

// CheckName reports whether name is a whole, acceptable player name:
// NameChars, not empty, and none of the words Fuzzball reserves.
func CheckName(name string) error {
	if name == "" {
		return errors.New(str.ConfigNameRequiredBare())
	}
	switch strings.ToLower(name) {
	case "me", "here", "home", "nil":
		return errors.New(str.ConfigNameReserved(name))
	}
	return NameChars(name)
}

var idUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`) //str:ok

// CharID is the id a new character named name gets: the name, with
// anything an id can't use turned into _.
func CharID(name string) string { return idUnsafe.ReplaceAllString(name, "_") }

// AddCharacter appends a character named name to worlds/<world>.toml and
// returns its id. Like AppendHighlight, appending is safe because
// "[[characters]]" is absolute; existing content is kept.
func AddCharacter(dir, world, name string) (string, error) {
	if err := CheckName(name); err != nil {
		return "", err
	}
	if !idRE.MatchString(world) {
		return "", errors.New(str.ConfigBadWorldIdBare(world))
	}
	path := filepath.Join(dir, "worlds", world+".toml")
	var wf struct{ Characters []charFile }
	if _, err := toml.DecodeFile(path, &wf); err != nil {
		return "", err
	}
	id := CharID(name)
	for _, cf := range wf.Characters {
		cid := cf.ID
		if cid == "" {
			cid = cf.Name
		}
		if strings.EqualFold(cid, id) {
			return "", errors.New(str.ConfigCharacterIdTaken(world, cid))
		}
	}
	q, err := tomlString(name)
	if err != nil {
		return "", err
	}
	block := "\n[[characters]]\nname = " + q + "\n" //str:ok
	if id != name {
		block += fmt.Sprintf("id = %q\n", id) //str:ok
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.WriteString(block)
	return id, err
}

// AddWorld creates worlds/<id>.toml for a server. It uses the fuzzball
// pack when packs/fuzzball.toml exists; everything else is left to the
// file's author.
func AddWorld(dir, id, host string, port int, tls bool) error {
	if !idRE.MatchString(id) {
		return errors.New(str.ConfigWorldIdChars())
	}
	if host == "" || strings.ContainsFunc(host, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return errors.New(str.ConfigBadHost())
	}
	if port <= 0 || port > 65535 {
		return errors.New(str.ConfigPortRange())
	}
	existing, err := filepath.Glob(filepath.Join(dir, "worlds", "*.toml"))
	if err != nil {
		return err
	}
	for _, p := range existing {
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(p), ".toml"), id) {
			return errors.New(str.ConfigWorldExists(id))
		}
	}
	body := fmt.Sprintf("# %s\nhost = %q\nport = %d\ntls = %t\n", str.ConfigAddedByKiln(), host, port, tls) //str:ok
	if _, err := os.Stat(filepath.Join(dir, "packs", "fuzzball.toml")); err == nil {
		body += "use = [\"fuzzball\"]\n" //str:ok
	}
	if err := os.MkdirAll(filepath.Join(dir, "worlds"), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "worlds", id+".toml"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(body)
	return err
}
