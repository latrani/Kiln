package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Editing hand-written world files. Only the lines being changed change:
// a value is replaced where it stands (keeping a comment after it), an
// unset key's line is removed, and a new key goes after the last key of
// its table. Everything else, comments and formatting included, is kept
// byte for byte. Each edit is checked by loading the result before the
// file is (atomically) replaced.

// tomlDoc is a world file's text and where its tables and keys are.
type tomlDoc struct {
	b      []byte
	tables []tomlTable // in file order; tables[0] is the top level
}

// tomlTable is one table: the top level, or a [header] / [[header]].
type tomlTable struct {
	key   []string // nil for the top level
	array bool     // [[...]]
	start int      // where its header line starts (0 for the top level)
	keys  []tomlKV // its own key/values, in order
}

type tomlKV struct {
	key        string
	start, end int // the "key = value" expression
}

// headerRE finds a table header line in the text between expressions,
// which holds only blanks, comments and headers.
var headerRE = regexp.MustCompile(`(?m)^[ \t]*\[`)

func parseDoc(b []byte) (*tomlDoc, error) {
	d := &tomlDoc{b: b, tables: []tomlTable{{}}}
	p := unstable.Parser{}
	p.Reset(b)
	prevEnd := 0
	var pending []int // tables whose header line isn't placed yet
	place := func(upTo int) {
		if len(pending) == 0 {
			return
		}
		locs := headerRE.FindAllIndex(b[prevEnd:upTo], -1)
		for i, t := range pending {
			if i < len(locs) {
				d.tables[t].start = prevEnd + locs[i][0]
			} else {
				d.tables[t].start = upTo
			}
		}
		pending = nil
	}
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			var key []string
			for it := e.Key(); it.Next(); {
				key = append(key, string(it.Node().Data))
			}
			d.tables = append(d.tables, tomlTable{key: key, array: e.Kind == unstable.ArrayTable})
			pending = append(pending, len(d.tables)-1)
		case unstable.KeyValue:
			start, end := int(e.Raw.Offset), int(e.Raw.Offset+e.Raw.Length)
			place(start)
			var parts []string
			for it := e.Key(); it.Next(); {
				parts = append(parts, string(it.Node().Data))
			}
			t := &d.tables[len(d.tables)-1]
			t.keys = append(t.keys, tomlKV{key: strings.Join(parts, "."), start: start, end: end})
			prevEnd = end
		}
	}
	if err := p.Error(); err != nil {
		return nil, err
	}
	place(len(b))
	return d, nil
}

// character finds the table of the n-th [[characters]] (0-based).
func (d *tomlDoc) character(n int) (int, bool) {
	for i, t := range d.tables {
		if t.array && slices.Equal(t.key, []string{"characters"}) {
			if n == 0 {
				return i, true
			}
			n--
		}
	}
	return 0, false
}

// span is a byte range to replace with text.
type span struct {
	start, end int
	text       string
}

// apply makes the replacements, which must not overlap.
func (d *tomlDoc) apply(spans []span) []byte {
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	out := slices.Clone(d.b)
	for _, s := range spans {
		out = append(out[:s.start], append([]byte(s.text), out[s.end:]...)...)
	}
	return out
}

// lineEnd is the index just past the end of the line holding i
// (including its newline, if any).
func (d *tomlDoc) lineEnd(i int) int {
	if j := bytes.IndexByte(d.b[i:], '\n'); j >= 0 {
		return i + j + 1
	}
	return len(d.b)
}

// lineStart is the index where the line holding i starts.
func (d *tomlDoc) lineStart(i int) int { return bytes.LastIndexByte(d.b[:i], '\n') + 1 }

// setKeys edits table t: each key in set gets that value (a TOML
// literal), and each key whose value is "" is removed.
func (d *tomlDoc) setKeys(t int, set map[string]string) []span {
	tb := d.tables[t]
	var spans []span
	var add []string
	for _, k := range sortedKeys(set) {
		v := set[k]
		i := slices.IndexFunc(tb.keys, func(kv tomlKV) bool { return kv.key == k })
		switch {
		case i >= 0 && v != "":
			spans = append(spans, span{tb.keys[i].start, tb.keys[i].end, k + " = " + v})
		case i >= 0: // remove the whole line, with any comment on it
			kv := tb.keys[i]
			start, end := kv.start, kv.end
			if strings.TrimSpace(string(d.b[d.lineStart(start):start])) == "" {
				start, end = d.lineStart(start), d.lineEnd(end)
			}
			spans = append(spans, span{start, end, ""})
		case v != "":
			add = append(add, k+" = "+v+"\n")
		}
	}
	if len(add) > 0 {
		at := 0 // an empty top level: the start of the file
		switch {
		case len(tb.keys) > 0:
			at = d.lineEnd(tb.keys[len(tb.keys)-1].end)
		case t > 0:
			at = d.lineEnd(tb.start)
		}
		text := strings.Join(add, "")
		if at > 0 && d.b[at-1] != '\n' {
			text = "\n" + text
		}
		spans = append(spans, span{at, at, text})
	}
	return spans
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Literals for setKeys.
func tomlQuote(s string) string {
	q, _ := tomlString(s) // only fails on invalid UTF-8, which json replaces
	return q
}

func tomlStrings(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = tomlQuote(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func tomlOptBool(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

func tomlOptInt(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func tomlOptString(s *string) string {
	if s == nil {
		return ""
	}
	return tomlQuote(*s)
}

// WorldSettings are what a world file itself sets: its server, and the
// settings it gives its characters. A nil pointer (or empty Use) is
// unset, so the world inherits it from config.toml.
type WorldSettings struct {
	Host         string
	Port         int
	TLS          bool
	TLSTrust     string // "" for the default, "pin"
	Use          []string
	Login        *string
	MaxLineBytes *int
	NewlineMode  *string
	Autoconnect  *bool
	LocalEcho    *bool
}

// CharacterSettings are what a character's entry sets besides its name.
// A nil pointer is unset: the character inherits it from its world.
type CharacterSettings struct {
	Aliases     []string
	Autoconnect *bool
	LocalEcho   *bool
}

// Inherited is what a world or character gets for settings it leaves
// unset.
type Inherited struct {
	Login        string
	MaxLineBytes int
	NewlineMode  string
	Autoconnect  bool
	LocalEcho    bool
}

func inherited(s settings) Inherited {
	return Inherited{Login: *s.Login, MaxLineBytes: *s.MaxLineBytes, NewlineMode: *s.NewlineMode,
		Autoconnect: *s.Autoconnect, LocalEcho: *s.LocalEcho}
}

// Defaults is what a world gets for settings it leaves unset.
func Defaults(dir string) (Inherited, error) {
	_, base, err := loadGlobal(dir)
	if err != nil {
		return Inherited{}, err
	}
	return inherited(base), nil
}

func worldPath(dir, world string) (string, error) {
	if !idRE.MatchString(world) {
		return "", fmt.Errorf("bad world id %q", world)
	}
	return filepath.Join(dir, "worlds", world+".toml"), nil
}

// ReadWorld returns what worlds/<world>.toml sets, and what its unset
// settings inherit from config.toml.
func ReadWorld(dir, world string) (WorldSettings, Inherited, error) {
	path, err := worldPath(dir, world)
	if err != nil {
		return WorldSettings{}, Inherited{}, err
	}
	var wf worldFile
	if err := decodeFile(path, &wf, false); err != nil {
		return WorldSettings{}, Inherited{}, err
	}
	_, base, err := loadGlobal(dir)
	if err != nil {
		return WorldSettings{}, Inherited{}, err
	}
	return WorldSettings{
		Host: wf.Host, Port: wf.Port, TLS: wf.TLS, TLSTrust: wf.TLSTrust, Use: wf.Use,
		Login: wf.Login, MaxLineBytes: wf.MaxLineBytes, NewlineMode: wf.NewlineMode,
		Autoconnect: wf.Autoconnect, LocalEcho: wf.LocalEcho,
	}, inherited(base), nil
}

// WriteWorld saves s to worlds/<world>.toml, changing only the keys
// whose values differ.
func WriteWorld(dir, world string, s WorldSettings) error {
	path, err := worldPath(dir, world)
	if err != nil {
		return err
	}
	old, _, err := ReadWorld(dir, world)
	if err != nil {
		return err
	}
	set := map[string]string{}
	change := func(key, was, now string) {
		if was != now {
			set[key] = now
		}
	}
	change("host", tomlQuote(old.Host), tomlQuote(s.Host))
	change("port", strconv.Itoa(old.Port), strconv.Itoa(s.Port))
	change("tls", strconv.FormatBool(old.TLS), strconv.FormatBool(s.TLS))
	quoteSet := func(v string) string {
		if v == "" {
			return ""
		}
		return tomlQuote(v)
	}
	change("tls_trust", quoteSet(old.TLSTrust), quoteSet(s.TLSTrust))
	change("use", tomlStrings(old.Use), tomlStrings(s.Use))
	change("login", tomlOptString(old.Login), tomlOptString(s.Login))
	change("max_line_bytes", tomlOptInt(old.MaxLineBytes), tomlOptInt(s.MaxLineBytes))
	change("newline_mode", tomlOptString(old.NewlineMode), tomlOptString(s.NewlineMode))
	change("autoconnect", tomlOptBool(old.Autoconnect), tomlOptBool(s.Autoconnect))
	change("local_echo", tomlOptBool(old.LocalEcho), tomlOptBool(s.LocalEcho))
	if len(set) == 0 {
		return nil
	}
	return editWorld(dir, path, func(d *tomlDoc) ([]span, error) { return d.setKeys(0, set), nil })
}

// characterIndex is the position of character id among world file wf's
// [[characters]].
func characterIndex(wf worldFile, world, id string) (int, error) {
	for i, cf := range wf.Characters {
		cid := cf.ID
		if cid == "" {
			cid = cf.Name
		}
		if cid == id {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%s has no character %q", world, id)
}

// ReadCharacter returns what character id's entry in worlds/<world>.toml
// sets, and what its unset settings inherit from the world.
func ReadCharacter(dir, world, id string) (CharacterSettings, Inherited, error) {
	path, err := worldPath(dir, world)
	if err != nil {
		return CharacterSettings{}, Inherited{}, err
	}
	var wf worldFile
	if err := decodeFile(path, &wf, false); err != nil {
		return CharacterSettings{}, Inherited{}, err
	}
	i, err := characterIndex(wf, world, id)
	if err != nil {
		return CharacterSettings{}, Inherited{}, err
	}
	_, base, err := loadGlobal(dir)
	if err != nil {
		return CharacterSettings{}, Inherited{}, err
	}
	base.overlay(wf.settings)
	cf := wf.Characters[i]
	return CharacterSettings{Aliases: cf.Aliases, Autoconnect: cf.Autoconnect, LocalEcho: cf.LocalEcho}, inherited(base), nil
}

// WriteCharacter saves s to character id's entry, changing only the
// keys whose values differ.
func WriteCharacter(dir, world, id string, s CharacterSettings) error {
	path, err := worldPath(dir, world)
	if err != nil {
		return err
	}
	old, _, err := ReadCharacter(dir, world, id)
	if err != nil {
		return err
	}
	set := map[string]string{}
	change := func(key, was, now string) {
		if was != now {
			set[key] = now
		}
	}
	change("aliases", tomlStrings(old.Aliases), tomlStrings(s.Aliases))
	change("autoconnect", tomlOptBool(old.Autoconnect), tomlOptBool(s.Autoconnect))
	change("local_echo", tomlOptBool(old.LocalEcho), tomlOptBool(s.LocalEcho))
	if len(set) == 0 {
		return nil
	}
	return editWorld(dir, path, func(d *tomlDoc) ([]span, error) {
		t, err := d.characterTable(path, world, id)
		if err != nil {
			return nil, err
		}
		return d.setKeys(t, set), nil
	})
}

// characterTable finds character id's [[characters]] table.
func (d *tomlDoc) characterTable(path, world, id string) (int, error) {
	var wf worldFile
	if err := decodeBytes(path, d.b, &wf); err != nil {
		return 0, err
	}
	n, err := characterIndex(wf, world, id)
	if err != nil {
		return 0, err
	}
	t, ok := d.character(n)
	if !ok {
		return 0, fmt.Errorf("%s: can't find character %q's table", filepath.Base(path), id)
	}
	return t, nil
}

// DeleteCharacter removes character id's entry from worlds/<world>.toml:
// its [[characters]] table and the tables under it, like its own rules.
// Comments just above the next table stay with it.
func DeleteCharacter(dir, world, id string) error {
	path, err := worldPath(dir, world)
	if err != nil {
		return err
	}
	return editWorld(dir, path, func(d *tomlDoc) ([]span, error) {
		t, err := d.characterTable(path, world, id)
		if err != nil {
			return nil, err
		}
		end := len(d.b)
		for _, next := range d.tables[t+1:] {
			if len(next.key) < 2 || next.key[0] != "characters" { // not one of this character's sub-tables
				end = next.start
				break
			}
		}
		start := d.tables[t].start
		for end > start { // leave the next table's comment lines above it
			prev := d.lineStart(end - 1)
			if !strings.HasPrefix(strings.TrimSpace(string(d.b[prev:end])), "#") {
				break
			}
			end = prev
		}
		return []span{{start, end, ""}}, nil
	})
}

// DeleteWorld removes worlds/<world>.toml, which must have no characters
// left. Logs, saved passwords and certificates aren't touched.
func DeleteWorld(dir, world string) error {
	path, err := worldPath(dir, world)
	if err != nil {
		return err
	}
	var wf worldFile
	if err := decodeFile(path, &wf, false); err != nil {
		return err
	}
	if n := len(wf.Characters); n > 0 {
		return fmt.Errorf("%s still has %d character(s); delete them first", world, n)
	}
	return os.Remove(path)
}

// editWorld applies an edit to the world file at path, checks that the
// result still loads, and replaces the file with it.
func editWorld(dir, path string, edit func(*tomlDoc) ([]span, error)) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	d, err := parseDoc(b)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	spans, err := edit(d)
	if err != nil {
		return err
	}
	out := d.apply(spans)
	_, base, err := loadGlobal(dir)
	if err != nil {
		return err
	}
	if _, err := loadWorldData(dir, path, out, base, map[string]Rules{}); err != nil {
		return err
	}
	return replaceFile(path, out)
}

// replaceFile writes b to path atomically, keeping the file's mode.
func replaceFile(path string, b []byte) error {
	mode := os.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".edit-*") // not *.toml, so never loaded
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
