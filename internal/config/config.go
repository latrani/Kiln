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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/pathfmt"
	"github.com/latrani/Kiln/internal/str"
)

// Style is how a highlighted line is drawn. Empty colors mean "unchanged".
type Style struct {
	FG        string `toml:"fg"`
	BG        string `toml:"bg"`
	Bold      bool   `toml:"bold"`
	Italic    bool   `toml:"italic"`
	Underline bool   `toml:"underline"`
}

// ClassifyRule tags lines whose plain text matches Pattern with Tag and
// every one of Tags. By convention a "/" nests a tag under another, as in
// tags = ["page", "page/in"], so a line can be filtered broadly (page)
// and styled narrowly (page/in).
type ClassifyRule struct {
	Tag     string   `toml:"tag"`
	Tags    []string `toml:"tags"`
	Pattern string   `toml:"pattern"`
}

// AllTags is Tag and Tags together, without duplicates or blanks.
func (r ClassifyRule) AllTags() []string {
	var out []string
	for _, t := range append([]string{r.Tag}, r.Tags...) {
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// Match selects lines for a highlight rule. A line matches when it has at
// least one of Tags (if any are given) AND matches Pattern (if given).
type Match struct {
	Tags    []string `toml:"tags"`
	Pattern string   `toml:"pattern"`
}

// HighlightRule styles matching lines and optionally flags them for
// attention, or the opposite, quiet: they don't count as unread and never
// ask for attention, even when another rule does. Scope "match" styles
// only the matched text (see rules.Apply); "" or "line" styles the whole
// line.
type HighlightRule struct {
	Match     Match  `toml:"match"`
	Style     Style  `toml:"style"`
	Attention bool   `toml:"attention"`
	Quiet     bool   `toml:"quiet"`
	Scope     string `toml:"scope"`
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
	NewlineMode  string       // "batch" or "flatten"
	Autoconnect  bool         // connect when Kiln starts
	Reconnect    bool         // retry after a drop or failed connect
	Notify       notify.Level // what notifies while you're away
	LocalEcho    bool         // show sent lines in the scrollback
	Rules        Rules
}

// World groups resolved characters under their world id.
type World struct {
	ID         string
	Characters []Character // in file order
}

// Config is the resolved configuration.
type Config struct {
	Worlds        []World       // sorted by ID
	ExportDir     string        // where browse-mode exports go; "~" already expanded
	ExportName    string        // export file name template; see ExportNameVars
	ExportFormat  string        // "plain", "ansi" or "html" preselected for exports; "" asks
	LogDir        string        // log folder template (strftime, {world} {char} {name}); "~" expanded, "" for the default
	LogName       string        // log file name template, without ".log"; "" for the default
	PasswordStore string        // "keychain", "file" or "none"
	NotifyIdle    time.Duration // no input for this long counts as away; 0: only blur does
	NotifyMethod  notify.Method
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
	Reconnect    *bool   `toml:"reconnect"`
	Notify       *string `toml:"notify"`
	LocalEcho    *bool   `toml:"local_echo"`
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
	if o.Reconnect != nil {
		s.Reconnect = o.Reconnect
	}
	if o.Notify != nil {
		s.Notify = o.Notify
	}
	if o.LocalEcho != nil {
		s.LocalEcho = o.LocalEcho
	}
}

type globalFile struct {
	ExportDir     string   `toml:"export_dir"`
	ExportName    string   `toml:"export_name"`
	ExportFormat  string   `toml:"export_format"`
	LogDir        string   `toml:"log_dir"`
	LogName       string   `toml:"log_name"`
	PasswordStore string   `toml:"password_store"`
	NotifyIdle    any      `toml:"notify_idle"` // a duration string; any so a bare number gets a friendly error
	NotifyMethod  string   `toml:"notify_method"`
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
	// DefaultNotifyIdle is used when config.toml sets no notify_idle.
	DefaultNotifyIdle = 5 * time.Minute
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`) //str:ok

// Load reads and resolves the config directory. config.toml and the
// worlds/ and packs/ directories are all optional.
func Load(dir string) (*Config, error) {
	g, base, err := loadGlobal(dir)
	if err != nil {
		return nil, err
	}

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
		return nil, errors.New(str.ConfigBadPasswordStore())
	}
	exportName := g.ExportName
	if exportName == "" {
		exportName = DefaultExportName
	}
	if err := checkVars("export_name", exportName, ExportNameVars); err != nil {
		return nil, err
	}
	switch g.ExportFormat {
	case "", "plain", "ansi", "html":
	default:
		return nil, errors.New(str.ConfigBadExportFormat())
	}
	idle := DefaultNotifyIdle
	if g.NotifyIdle != nil {
		v, ok := g.NotifyIdle.(string)
		if ok {
			idle, err = time.ParseDuration(v)
		}
		if !ok || err != nil || idle < 0 {
			return nil, errors.New(str.ConfigBadNotifyIdle())
		}
	}
	method := notify.OSC
	if g.NotifyMethod != "" {
		if method, err = notify.ParseMethod(g.NotifyMethod); err != nil {
			return nil, fmt.Errorf("config.toml: %w", err) //str:ok
		}
	}
	logDir := g.LogDir
	if logDir != "" {
		if logDir, err = ExpandHome(logDir); err != nil {
			return nil, err
		}
	}
	if err := checkVars("log_dir", logDir, LogVars); err != nil {
		return nil, err
	}
	if err := checkVars("log_name", g.LogName, LogVars); err != nil {
		return nil, err
	}
	if strings.ContainsAny(g.LogName, `/\`) {
		return nil, errors.New(str.ConfigLogNameHasFolders())
	}
	cfg := &Config{ExportDir: exportDir, ExportName: exportName, ExportFormat: g.ExportFormat, LogDir: logDir, LogName: g.LogName, PasswordStore: store,
		NotifyIdle: idle, NotifyMethod: method}
	for _, wp := range worldPaths {
		w, err := loadWorld(dir, wp, base, packs)
		if err != nil {
			return nil, err
		}
		cfg.Worlds = append(cfg.Worlds, w)
	}
	return cfg, nil
}

// loadGlobal reads config.toml, and the settings every world starts
// from: the built-in defaults under its [defaults].
func loadGlobal(dir string) (globalFile, settings, error) {
	var g globalFile
	if err := decodeFile(filepath.Join(dir, "config.toml"), &g, true); err != nil {
		return g, settings{}, err
	}
	base := settings{MaxLineBytes: ptr(DefaultMaxLineBytes), NewlineMode: ptr(DefaultNewlineMode), Login: ptr(""), Autoconnect: ptr(false), Reconnect: ptr(true), Notify: ptr(string(notify.First)), LocalEcho: ptr(false)}
	base.overlay(g.Defaults)
	return g, base, nil
}

func loadWorld(dir, path string, base settings, packs map[string]Rules) (World, error) {
	return loadWorldData(dir, path, nil, base, packs)
}

// loadWorldData is loadWorld with the file's contents given as data,
// or read from path when data is nil.
func loadWorldData(dir, path string, data []byte, base settings, packs map[string]Rules) (World, error) {
	id := strings.TrimSuffix(filepath.Base(path), ".toml")
	rel := filepath.Join("worlds", filepath.Base(path))
	if !idRE.MatchString(id) {
		return World{}, errors.New(str.ConfigBadWorldId(rel, id))
	}
	var wf worldFile
	if data == nil {
		if err := decodeFile(path, &wf, false); err != nil {
			return World{}, err
		}
	} else if err := decodeBytes(path, data, &wf); err != nil {
		return World{}, err
	}
	if wf.Host == "" {
		return World{}, errors.New(str.ConfigHostRequired(rel))
	}
	if wf.Port <= 0 || wf.Port > 65535 {
		return World{}, errors.New(str.ConfigBadPort(rel))
	}
	switch wf.TLSTrust {
	case "":
		wf.TLSTrust = "pin"
	case "pin", "ca":
	default:
		return World{}, errors.New(str.ConfigBadTlsTrust(rel))
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
		where := str.ConfigCharacterNumber(rel, i+1)
		if strings.TrimSpace(cf.Name) == "" {
			return World{}, errors.New(str.ConfigNameRequired(where))
		}
		cid := cf.ID
		if cid == "" {
			cid = cf.Name
		}
		where = str.ConfigCharacterId(rel, cid)
		if !idRE.MatchString(cid) {
			return World{}, errors.New(str.ConfigBadCharacterId(where))
		}
		// Case-insensitively, since the id names log folders.
		if seen[strings.ToLower(cid)] {
			return World{}, errors.New(str.ConfigDuplicateCharacterId(where))
		}
		seen[strings.ToLower(cid)] = true
		cs := ws
		cs.overlay(cf.settings)
		ch := Character{
			World: id, ID: cid, Name: cf.Name, Aliases: cf.Aliases,
			Host: wf.Host, Port: wf.Port, TLS: wf.TLS, TLSTrust: wf.TLSTrust,
			Login: *cs.Login, MaxLineBytes: *cs.MaxLineBytes, NewlineMode: *cs.NewlineMode, Autoconnect: *cs.Autoconnect,
			Reconnect: *cs.Reconnect, Notify: notify.Level(*cs.Notify), LocalEcho: *cs.LocalEcho,
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
		return Rules{}, errors.New(str.ConfigBadPackId(id))
	}
	var r Rules
	path := filepath.Join(dir, "packs", id+".toml")
	if _, err := os.Stat(path); err != nil {
		return Rules{}, errors.New(str.ConfigUnknownPack(id))
	}
	if err := decodeFile(path, &r, false); err != nil {
		return Rules{}, err
	}
	cache[id] = r
	return r, nil
}

func validate(ch Character) error {
	if ch.MaxLineBytes <= 0 {
		return errors.New(str.ConfigBadMaxLineBytes())
	}
	if ch.NewlineMode != "batch" && ch.NewlineMode != "flatten" {
		return errors.New(str.ConfigBadNewlineMode())
	}
	if _, err := notify.ParseLevel(string(ch.Notify)); err != nil {
		return err
	}
	for i, r := range ch.Rules.Classify {
		tags := r.AllTags()
		if len(tags) == 0 {
			return errors.New(str.ConfigClassifyNeedsTag(i + 1))
		}
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return str.Wrap(str.ConfigClassifyBadPattern(i+1, strings.Join(tags, ", "), err), err)
		}
	}
	for i, r := range ch.Rules.Highlight {
		if len(r.Match.Tags) == 0 && r.Match.Pattern == "" {
			return errors.New(str.ConfigHighlightNeedsMatch(i + 1))
		}
		if _, err := regexp.Compile(r.Match.Pattern); err != nil {
			return str.Wrap(str.ConfigHighlightBadPattern(i+1, err), err)
		}
		if r.Scope != "" && r.Scope != "line" && r.Scope != "match" {
			return errors.New(str.ConfigBadHighlightScope(i + 1))
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
	return decodeBytes(path, b, v)
}

// decodeBytes is decodeFile for contents already read from path.
func decodeBytes(path string, b []byte, v any) error {
	md, err := toml.Decode(string(b), v)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return errors.New(str.ConfigUnknownKey(filepath.Base(path), und[0].String()))
	}
	return nil
}

// DefaultExportName is used when config.toml sets no export_name.
const DefaultExportName = "{date} {time} {world} {name}" //str:ok

// LogVars are the placeholders log_dir and log_name may use (with
// strftime codes for the session's start): the world id, the character's
// id and its name.
var LogVars = []string{"world", "char", "name"}

// ExportNameVars are the placeholders export_name may use (with strftime
// codes for the scene's first line): its date (YYYY-MM-DD) and time
// (HHMM), the world id and the character's name.
var ExportNameVars = []string{"date", "time", "world", "name"}

// checkVars rejects {placeholders} in a template other than vars, and
// unknown strftime codes.
func checkVars(setting, template string, vars []string) error {
	if err := pathfmt.Check(template, vars); err != nil {
		return fmt.Errorf("config.toml: %s: %w", setting, err) //str:ok
	}
	return nil
}

// DefaultExportDir is used when config.toml sets no export_dir.
const DefaultExportDir = "~/Documents/Kiln Scenes" //str:ok

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
