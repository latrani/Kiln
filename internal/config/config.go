// Package config loads Kiln's config directory:
//
//	<dir>/config.toml        [defaults] + global prefs
//	<dir>/worlds/<id>.toml   one world and its characters
//	<dir>/packs/<id>.toml    reusable classify/highlight rule sets
//
// and resolves inheritance: defaults → packs (in `use` order) → world →
// character. Rule lists append along that chain; scalars override.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Style is how a highlighted line is drawn. Empty colors mean "unchanged".
type Style struct {
	FG        string `toml:"fg"`
	BG        string `toml:"bg"`
	Bold      bool   `toml:"bold"`
	Italic    bool   `toml:"italic"`
	Underline bool   `toml:"underline"`
}

// ClassifyRule tags lines whose plain text matches Pattern.
type ClassifyRule struct {
	Tag     string `toml:"tag"`
	Pattern string `toml:"pattern"`
}

// Match selects lines for a highlight rule. A line matches when it has at
// least one of Tags (if any are given) AND matches Pattern (if given).
type Match struct {
	Tags    []string `toml:"tags"`
	Pattern string   `toml:"pattern"`
}

// HighlightRule styles matching lines and optionally flags them for attention.
type HighlightRule struct {
	Match     Match `toml:"match"`
	Style     Style `toml:"style"`
	Attention bool  `toml:"attention"`
}

// Rules is the rule set carried by packs, worlds, and characters.
type Rules struct {
	Classify  []ClassifyRule  `toml:"classify"`
	Highlight []HighlightRule `toml:"highlight"`
}

// Character is a fully resolved character: everything a session needs.
type Character struct {
	World        string // world id (filename without .toml)
	ID           string // key under [characters]
	Name         string // canonical in-game name
	Aliases      []string
	Host         string
	Port         int
	TLS          bool
	TLSTrust     string // "pin" or "ca"
	Login        string // template with {name} and {password}; "" = no auto-login
	MaxLineBytes int
	NewlineMode  string // "batch" or "flatten"
	Autoconnect  bool   // connect when Kiln starts
	Rules        Rules
}

// World groups resolved characters under their world id.
type World struct {
	ID         string
	Characters []Character // in file order
}

// Config is the resolved configuration.
type Config struct {
	Worlds        []World // sorted by ID
	ExportDir     string  // where browse-mode exports go; "~" already expanded
	PasswordStore string  // "keychain", "file" or "none"
}

// Find returns the resolved character, or false.
func (c *Config) Find(world, char string) (Character, bool) {
	for _, w := range c.Worlds {
		if w.ID != world {
			continue
		}
		for _, ch := range w.Characters {
			if ch.ID == char {
				return ch, true
			}
		}
	}
	return Character{}, false
}

// settings are the inheritable scalars. Pointers distinguish "unset" from
// zero values so later levels only override what they actually set.
type settings struct {
	MaxLineBytes *int    `toml:"max_line_bytes"`
	NewlineMode  *string `toml:"newline_mode"`
	Login        *string `toml:"login"`
	Autoconnect  *bool   `toml:"autoconnect"`
}

func (s *settings) overlay(o settings) {
	if o.MaxLineBytes != nil {
		s.MaxLineBytes = o.MaxLineBytes
	}
	if o.NewlineMode != nil {
		s.NewlineMode = o.NewlineMode
	}
	if o.Login != nil {
		s.Login = o.Login
	}
	if o.Autoconnect != nil {
		s.Autoconnect = o.Autoconnect
	}
}

type globalFile struct {
	ExportDir     string   `toml:"export_dir"`
	PasswordStore string   `toml:"password_store"`
	Defaults      settings `toml:"defaults"`
}

type charFile struct {
	settings
	Rules
	ID      string   `toml:"id"` // defaults to Name
	Name    string   `toml:"name"`
	Aliases []string `toml:"aliases"`
}

type worldFile struct {
	settings
	Rules
	Host       string     `toml:"host"`
	Port       int        `toml:"port"`
	TLS        bool       `toml:"tls"`
	TLSTrust   string     `toml:"tls_trust"`
	Use        []string   `toml:"use"`
	Characters []charFile `toml:"characters"`
}

// Built-in defaults, applied beneath config.toml's [defaults].
const (
	DefaultMaxLineBytes = 2047 // Fuzzball MAX_COMMAND_LEN (2048) minus the NUL
	DefaultNewlineMode  = "batch"
	// DefaultPasswordStore is used when config.toml sets no password_store.
	DefaultPasswordStore = "keychain"
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Load reads and resolves the config directory. config.toml and the
// worlds/ and packs/ directories are all optional.
func Load(dir string) (*Config, error) {
	var g globalFile
	if err := decodeFile(filepath.Join(dir, "config.toml"), &g, true); err != nil {
		return nil, err
	}
	base := settings{MaxLineBytes: ptr(DefaultMaxLineBytes), NewlineMode: ptr(DefaultNewlineMode), Login: ptr(""), Autoconnect: ptr(false)}
	base.overlay(g.Defaults)

	worldPaths, err := filepath.Glob(filepath.Join(dir, "worlds", "*.toml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(worldPaths)

	packs := map[string]Rules{}
	exportDir, err := ExpandHome(g.ExportDir)
	if err != nil {
		return nil, err
	}
	store := g.PasswordStore
	if store == "" {
		store = DefaultPasswordStore
	}
	if store != "keychain" && store != "file" && store != "none" {
		return nil, errors.New(`config.toml: password_store must be "keychain", "file" or "none"`)
	}
	cfg := &Config{ExportDir: exportDir, PasswordStore: store}
	for _, wp := range worldPaths {
		w, err := loadWorld(dir, wp, base, packs)
		if err != nil {
			return nil, err
		}
		cfg.Worlds = append(cfg.Worlds, w)
	}
	return cfg, nil
}

func loadWorld(dir, path string, base settings, packs map[string]Rules) (World, error) {
	id := strings.TrimSuffix(filepath.Base(path), ".toml")
	rel := filepath.Join("worlds", filepath.Base(path))
	if !idRE.MatchString(id) {
		return World{}, fmt.Errorf("%s: world id %q may only use letters, digits, _ and -", rel, id)
	}
	var wf worldFile
	if err := decodeFile(path, &wf, false); err != nil {
		return World{}, err
	}
	if wf.Host == "" {
		return World{}, fmt.Errorf("%s: host is required", rel)
	}
	if wf.Port <= 0 || wf.Port > 65535 {
		return World{}, fmt.Errorf("%s: port must be 1-65535", rel)
	}
	switch wf.TLSTrust {
	case "":
		wf.TLSTrust = "pin"
	case "pin", "ca":
	default:
		return World{}, fmt.Errorf("%s: tls_trust must be \"pin\" or \"ca\"", rel)
	}

	var worldRules Rules
	for _, p := range wf.Use {
		r, err := loadPack(dir, p, packs)
		if err != nil {
			return World{}, fmt.Errorf("%s: %w", rel, err)
		}
		worldRules = appendRules(worldRules, r)
	}
	worldRules = appendRules(worldRules, wf.Rules)
	ws := base
	ws.overlay(wf.settings)

	w := World{ID: id}
	seen := map[string]bool{}
	for i, cf := range wf.Characters {
		where := fmt.Sprintf("%s: characters #%d", rel, i+1)
		if strings.TrimSpace(cf.Name) == "" {
			return World{}, fmt.Errorf("%s: name is required", where)
		}
		cid := cf.ID
		if cid == "" {
			cid = cf.Name
		}
		where = fmt.Sprintf("%s: characters %q", rel, cid)
		if !idRE.MatchString(cid) {
			return World{}, fmt.Errorf("%s: id may only use letters, digits, _ and - (set id = \"...\" if the name has others)", where)
		}
		// Case-insensitively, since the id names log folders.
		if seen[strings.ToLower(cid)] {
			return World{}, fmt.Errorf("%s: id is used by another character in this world", where)
		}
		seen[strings.ToLower(cid)] = true
		cs := ws
		cs.overlay(cf.settings)
		ch := Character{
			World: id, ID: cid, Name: cf.Name, Aliases: cf.Aliases,
			Host: wf.Host, Port: wf.Port, TLS: wf.TLS, TLSTrust: wf.TLSTrust,
			Login: *cs.Login, MaxLineBytes: *cs.MaxLineBytes, NewlineMode: *cs.NewlineMode, Autoconnect: *cs.Autoconnect,
			Rules: appendRules(worldRules, cf.Rules),
		}
		if err := validate(ch); err != nil {
			return World{}, fmt.Errorf("%s: %w", where, err)
		}
		w.Characters = append(w.Characters, ch)
	}
	return w, nil
}

func loadPack(dir, id string, cache map[string]Rules) (Rules, error) {
	if r, ok := cache[id]; ok {
		return r, nil
	}
	if !idRE.MatchString(id) {
		return Rules{}, fmt.Errorf("pack id %q may only use letters, digits, _ and -", id)
	}
	var r Rules
	path := filepath.Join(dir, "packs", id+".toml")
	if _, err := os.Stat(path); err != nil {
		return Rules{}, fmt.Errorf("unknown pack %q (no packs/%s.toml)", id, id)
	}
	if err := decodeFile(path, &r, false); err != nil {
		return Rules{}, err
	}
	cache[id] = r
	return r, nil
}

func validate(ch Character) error {
	if ch.MaxLineBytes <= 0 {
		return errors.New("max_line_bytes must be positive")
	}
	if ch.NewlineMode != "batch" && ch.NewlineMode != "flatten" {
		return errors.New(`newline_mode must be "batch" or "flatten"`)
	}
	for i, r := range ch.Rules.Classify {
		if r.Tag == "" {
			return fmt.Errorf("classify rule %d: tag is required", i+1)
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("classify rule %d (%s): %w", i+1, r.Tag, err)
		}
	}
	for i, r := range ch.Rules.Highlight {
		if len(r.Match.Tags) == 0 && r.Match.Pattern == "" {
			return fmt.Errorf("highlight rule %d: match needs tags or pattern", i+1)
		}
		if _, err := regexp.Compile(r.Match.Pattern); err != nil {
			return fmt.Errorf("highlight rule %d: %w", i+1, err)
		}
	}
	return nil
}

// decodeFile decodes TOML into v, rejecting unknown keys so typos surface.
func decodeFile(path string, v any, optional bool) error {
	b, err := os.ReadFile(path)
	if optional && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	md, err := toml.Decode(string(b), v)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return fmt.Errorf("%s: unknown key %q", filepath.Base(path), und[0].String())
	}
	return nil
}

// DefaultExportDir is used when config.toml sets no export_dir.
const DefaultExportDir = "~/Documents/Kiln Scenes"

// ExpandHome expands a leading "~/" (and defaults an empty path to
// DefaultExportDir).
func ExpandHome(p string) (string, error) {
	if p == "" {
		p = DefaultExportDir
	}
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}

func appendRules(a, b Rules) Rules {
	return Rules{
		Classify:  append(append([]ClassifyRule(nil), a.Classify...), b.Classify...),
		Highlight: append(append([]HighlightRule(nil), a.Highlight...), b.Highlight...),
	}
}

func ptr[T any](v T) *T { return &v }
