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
)

// HighlightStyle is the style /highlight gives new rules.
var HighlightStyle = Style{FG: "#ffd166", Bold: true}

// AppendHighlight adds a literal-text, case-insensitive highlight rule to
// the end of worlds/<world>.toml. Appending is safe because TOML table
// headers are absolute: "[[highlight]]" always means the world's top-level
// list, even after a [characters.x] table. The rule uses inline tables so
// nothing after the header can be mistaken for a sub-table. Existing
// content and comments are kept.
func AppendHighlight(dir, world, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("nothing to highlight")
	}
	if !idRE.MatchString(world) {
		return fmt.Errorf("bad world id %q", world)
	}
	pattern, err := tomlString("(?i)" + regexp.QuoteMeta(text))
	if err != nil {
		return err
	}
	rule := fmt.Sprintf("\n# added by /highlight\n[[highlight]]\nmatch = { pattern = %s }\nstyle = { fg = %q, bold = %t }\n",
		pattern, HighlightStyle.FG, HighlightStyle.Bold)
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
		return fmt.Errorf("a name can't start with %c", name[0])
	}
	for _, r := range name {
		switch {
		case r == ' ':
			return errors.New("a name can't have spaces")
		case r == '=' || r == '&' || r == '|':
			return fmt.Errorf("a name can't have %c", r)
		case r <= ' ' || r > '~':
			return errors.New("a name can only use plain ASCII")
		}
	}
	return nil
}

// CheckName reports whether name is a whole, acceptable player name:
// NameChars, not empty, and none of the words Fuzzball reserves.
func CheckName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	switch strings.ToLower(name) {
	case "me", "here", "home", "nil":
		return fmt.Errorf("%q isn't allowed as a name", name)
	}
	return NameChars(name)
}

var idUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

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
		return "", fmt.Errorf("bad world id %q", world)
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
			return "", fmt.Errorf("%s already has a character with id %q", world, cid)
		}
	}
	q, err := tomlString(name)
	if err != nil {
		return "", err
	}
	block := "\n[[characters]]\nname = " + q + "\n"
	if id != name {
		block += fmt.Sprintf("id = %q\n", id)
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
		return errors.New("world id may only use letters, digits, _ and -")
	}
	if host == "" || strings.ContainsFunc(host, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return errors.New("host must be a hostname or address")
	}
	if port <= 0 || port > 65535 {
		return errors.New("port must be 1-65535")
	}
	existing, err := filepath.Glob(filepath.Join(dir, "worlds", "*.toml"))
	if err != nil {
		return err
	}
	for _, p := range existing {
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(p), ".toml"), id) {
			return fmt.Errorf("world %q already exists", id)
		}
	}
	body := fmt.Sprintf("# added by Kiln\nhost = %q\nport = %d\ntls = %t\n", host, port, tls)
	if _, err := os.Stat(filepath.Join(dir, "packs", "fuzzball.toml")); err == nil {
		body += "use = [\"fuzzball\"]\n"
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
