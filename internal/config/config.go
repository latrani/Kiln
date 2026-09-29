// Package config loads Kiln's config directory:
//
//	<dir>/config.toml        [defaults] + global prefs
//	<dir>/worlds/<id>.toml   one world and its characters
//	<dir>/packs/<id>.toml    reusable classify rules and tag lists
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
	"github.com/latrani/Kiln/internal/theme"
)

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

// Rules is what packs, worlds and characters say about lines: how to tag
// them, and which tags ask for attention or are quiet. Along the chain
// the classify rules append, and the tag lists add up.
type Rules struct {
	Classify  []ClassifyRule `toml:"classify"`
	Attention []string       `toml:"attention"`
	Quiet     []string       `toml:"quiet"`
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
	Looks        []theme.Layer // the world's and then the character's own [palette] and [tags]
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
	ExportDir     string       `toml:"export_dir"`
	ExportName    string       `toml:"export_name"`
	ExportFormat  string       `toml:"export_format"`
	LogDir        string       `toml:"log_dir"`
	LogName       string       `toml:"log_name"`
	PasswordStore string       `toml:"password_store"`
	NotifyIdle    any          `toml:"notify_idle"` // a duration string; any so a bare number gets a friendly error
	NotifyMethod  string       `toml:"notify_method"`
	Defaults      defaultsFile `toml:"defaults"`
}

// defaultsFile is config.toml's [defaults]: the inheritable settings, and
// tag lists every world starts from.
type defaultsFile struct {
	settings
	Attention []string `toml:"attention"`
	Quiet     []string `toml:"quiet"`
}

type charFile struct {
	settings
	Rules
	Palette map[string]any `toml:"palette"`
	Tags    map[string]any `toml:"tags"`
	ID      string         `toml:"id"` // defaults to Name
	Name    string         `toml:"name"`
	Aliases []string       `toml:"aliases"`
}

type worldFile struct {
	settings
	Rules
	Palette    map[string]any `toml:"palette"`
	Tags       map[string]any `toml:"tags"`
	Host       string         `toml:"host"`
	Port       int            `toml:"port"`
	TLS        bool           `toml:"tls"`
	TLSTrust   string         `toml:"tls_trust"`
	Use        []string       `toml:"use"`
	Characters []charFile     `toml:"characters"`
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
	g, base, baseRules, err := loadGlobal(dir)
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
		w, err := loadWorld(dir, wp, base, baseRules, packs)
		if err != nil {
			return nil, err
		}
		cfg.Worlds = append(cfg.Worlds, w)
	}
	return cfg, nil
}

// loadGlobal reads config.toml, and the settings and tag lists every
// world starts from: the built-in defaults under its [defaults].
func loadGlobal(dir string) (globalFile, settings, Rules, error) {
	var g globalFile
	if err := decodeFile(filepath.Join(dir, "config.toml"), &g, true); err != nil {
		return g, settings{}, Rules{}, err
	}
	base := settings{MaxLineBytes: ptr(DefaultMaxLineBytes), NewlineMode: ptr(DefaultNewlineMode), Login: ptr(""), Autoconnect: ptr(false), Reconnect: ptr(true), Notify: ptr(string(notify.First)), LocalEcho: ptr(false)}
	base.overlay(g.Defaults.settings)
	return g, base, Rules{Attention: g.Defaults.Attention, Quiet: g.Defaults.Quiet}, nil
}

func loadWorld(dir, path string, base settings, baseRules Rules, packs map[string]Rules) (World, error) {
	return loadWorldData(dir, path, nil, base, baseRules, packs)
}

// loadWorldData is loadWorld with the file's contents given as data,
// or read from path when data is nil.
func loadWorldData(dir, path string, data []byte, base settings, baseRules Rules, packs map[string]Rules) (World, error) {
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

	var worldLooks []theme.Layer
	if wf.Palette != nil || wf.Tags != nil {
		l, err := theme.ParseLayer(rel, wf.Palette, wf.Tags)
		if err != nil {
			return World{}, err
		}
		worldLooks = append(worldLooks, l)
	}

	worldRules := baseRules
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
		looks := slices.Clone(worldLooks)
		if cf.Palette != nil || cf.Tags != nil {
			l, err := theme.ParseLayer(where, cf.Palette, cf.Tags)
			if err != nil {
				return World{}, err
			}
			looks = append(looks, l)
		}
		cs := ws
		cs.overlay(cf.settings)
		ch := Character{
			World: id, ID: cid, Name: cf.Name, Aliases: cf.Aliases,
			Host: wf.Host, Port: wf.Port, TLS: wf.TLS, TLSTrust: wf.TLSTrust,
			Login: *cs.Login, MaxLineBytes: *cs.MaxLineBytes, NewlineMode: *cs.NewlineMode, Autoconnect: *cs.Autoconnect,
			Reconnect: *cs.Reconnect, Notify: notify.Level(*cs.Notify), LocalEcho: *cs.LocalEcho,
			Rules: appendRules(worldRules, cf.Rules), Looks: looks,
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
	_, world := v.(*worldFile) // only worlds and their characters have looks
	for _, k := range md.Undecoded() {
		if !world || !inLooks(k) {
			return errors.New(str.ConfigUnknownKey(filepath.Base(path), k.String()))
		}
	}
	return nil
}

// inLooks reports whether k is inside a world's or character's [tags]
// or [palette]. Those decode into maps of any, which the TOML decoder
// counts as undecoded below their first level; the theme checks them.
func inLooks(k toml.Key) bool {
	if len(k) > 1 && k[0] == "characters" {
		k = k[1:]
	}
	return len(k) > 1 && (k[0] == "tags" || k[0] == "palette")
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
		Attention: union(a.Attention, b.Attention),
		Quiet:     union(a.Quiet, b.Quiet),
	}
}

// union is a then b's new entries, in order.
func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, s := range b {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func ptr[T any](v T) *T { return &v }
