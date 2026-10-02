package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/str"
)

// The editors open over the picker, in the input area: adding a world or
// a character, and editing or deleting one.

type editKind int

const (
	addWorld editKind = iota
	addChar
	editWorld
	editChar
)

// editor is a form for a world or character.
type editor struct {
	form     *form
	kind     editKind
	world    string // the world; "" while adding one
	char     string // the character's id, for editChar
	armed    string // a delete button pressed once; Enter again does it
	closeAll bool   // opened by /edit: closing the editor closes the picker
}

// Field and button labels. Fields are found by label, so each is read
// from the catalog once.
var (
	extraLabel     = str.EditorExtra()
	saveLabel      = str.EditorSave()
	forgetPWLabel  = str.EditorForgetPassword()
	delCharLabel   = str.EditorDeleteCharacter()
	delWorldLabel  = str.EditorDeleteWorld()
	worldLabel     = str.EditorWorld()
	hostLabel      = str.EditorHost()
	portLabel      = str.EditorPort()
	tlsLabel       = str.EditorTls()
	trustLabel     = str.EditorCertTrust()
	packsLabel     = str.EditorPacks()
	loginLabel     = str.EditorLogin()
	maxBytesLabel  = str.EditorMaxBytes()
	newlinesLabel  = str.EditorNewlines()
	autoconnLabel  = str.EditorAutoconnect()
	reconnectLabel = str.EditorReconnect()
	notifyLabel    = str.EditorNotify()
	echoLabel      = str.EditorLocalEcho()
	aliasesLabel   = str.EditorAliases()
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

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// shown is how a choice's value reads: on and off in words, config values
// (batch, pin, …) as they're written in the file.
func shown(v string) string {
	switch v {
	case "on":
		return str.EditorOn()
	case "off":
		return str.EditorOff()
	}
	return v
}

// inheritChoice is a choice between inheriting (shown with what that
// gives) and the given options.
func inheritChoice(label, inherits string, options ...string) field {
	labels := []string{str.EditorDefault(shown(inherits))}
	for _, o := range options {
		labels = append(labels, shown(o))
	}
	return choiceField(label, append([]string{""}, options...), labels)
}

// setBool puts an optional on/off setting into a choice field.
func (f *form) setBool(label string, b *bool) {
	if b != nil {
		f.choose(f.field(label), onOff(*b))
	}
}

// optBool reads an inheritable on/off choice.
func (f *form) optBool(label string) *bool {
	switch f.chosen(f.field(label)) {
	case "on":
		return new(true)
	case "off":
		return new(false)
	}
	return nil
}

// list splits a comma- or space-separated field into its items.
func list(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

// worldForm builds the form for a new world (add) or an existing one.
func (m *Model) worldForm(add bool, s config.WorldSettings, inh config.Inherited) *form {
	var fields []field
	if add {
		world := textField(worldLabel)
		world.accept = onlyMatching(worldIDChar, str.EditorWorldIdChars())
		fields = append(fields, world)
	}
	host, port := textField(hostLabel), textField(portLabel)
	host.accept = func(s string) error {
		if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r > '~' }) {
			return errors.New(str.EditorHostSpaces())
		}
		return nil
	}
	port.accept = onlyMatching(portChar, str.EditorPortNumber())
	packs := withHint(textField(packsLabel), str.EditorNone())
	packs.accept = onlyMatching(packsChar, str.EditorPacksList())
	login := str.EditorNone()
	if inh.Login != "" {
		login = inh.Login
	}
	maxBytes := withHint(textField(maxBytesLabel), str.EditorDefaultHint(inh.MaxLineBytes))
	maxBytes.accept = onlyMatching(numChar, str.EditorBytesNumber())
	fields = append(fields, host, port, toggleField(tlsLabel), sectionField(extraLabel),
		asExtra(inheritChoice(trustLabel, "pin", "pin", "ca")),
		asExtra(packs),
		asExtra(withHint(textField(loginLabel), str.EditorDefaultHint(login))),
		asExtra(maxBytes),
		asExtra(inheritChoice(newlinesLabel, inh.NewlineMode, "batch", "flatten")),
		asExtra(inheritChoice(autoconnLabel, onOff(inh.Autoconnect), "on", "off")),
		asExtra(inheritChoice(reconnectLabel, onOff(inh.Reconnect), "on", "off")),
		asExtra(inheritChoice(notifyLabel, inh.Notify, "all", "first", "attention", "none")),
		asExtra(inheritChoice(echoLabel, onOff(inh.LocalEcho), "on", "off")),
		primaryButton(saveLabel))
	if !add {
		fields = append(fields, buttonField(delWorldLabel))
	}
	f := newForm(str.EditorHint(), fields...)
	f.fields[f.field(hostLabel)].in.SetValue(s.Host)
	if s.Port > 0 {
		f.fields[f.field(portLabel)].in.SetValue(strconv.Itoa(s.Port))
	}
	f.fields[f.field(tlsLabel)].on = s.TLS
	f.choose(f.field(trustLabel), s.TLSTrust)
	f.fields[f.field(packsLabel)].in.SetValue(strings.Join(s.Use, ", "))
	if s.Login != nil {
		f.fields[f.field(loginLabel)].in.SetValue(*s.Login)
	}
	if s.MaxLineBytes != nil {
		f.fields[f.field(maxBytesLabel)].in.SetValue(strconv.Itoa(*s.MaxLineBytes))
	}
	if s.NewlineMode != nil {
		f.choose(f.field(newlinesLabel), *s.NewlineMode)
	}
	f.setBool(autoconnLabel, s.Autoconnect)
	f.setBool(reconnectLabel, s.Reconnect)
	if s.Notify != nil {
		f.choose(f.field(notifyLabel), *s.Notify)
	}
	f.setBool(echoLabel, s.LocalEcho)
	return f
}

// worldSettings reads a world form.
func worldSettings(f *form) (config.WorldSettings, error) {
	port, err := strconv.Atoi(f.value(f.field(portLabel)))
	if err != nil || port < 1 || port > 65535 {
		return config.WorldSettings{}, errors.New(str.EditorPortRange())
	}
	s := config.WorldSettings{
		Host: f.value(f.field(hostLabel)), Port: port, TLS: f.on(f.field(tlsLabel)),
		TLSTrust: f.chosen(f.field(trustLabel)), Use: list(f.value(f.field(packsLabel))),
		Autoconnect: f.optBool(autoconnLabel), Reconnect: f.optBool(reconnectLabel),
		LocalEcho: f.optBool(echoLabel),
	}
	if v := f.value(f.field(loginLabel)); v != "" {
		s.Login = &v
	}
	if v := f.value(f.field(maxBytesLabel)); v != "" {
		n, _ := strconv.Atoi(v)
		s.MaxLineBytes = &n
	}
	if v := f.chosen(f.field(newlinesLabel)); v != "" {
		s.NewlineMode = &v
	}
	if v := f.chosen(f.field(notifyLabel)); v != "" {
		s.Notify = &v
	}
	return s, nil
}

// charForm builds the form for editing a character.
func (m *Model) charForm(name string, s config.CharacterSettings, inh config.Inherited) *form {
	aliases := withHint(textField(aliasesLabel), str.EditorNone())
	aliases.accept = func(v string) error {
		for _, a := range list(v) {
			if err := config.NameChars(a); err != nil {
				return err
			}
		}
		return nil
	}
	// Every character setting is an extra one, so none hide behind a
	// section header.
	f := newForm(str.EditorHint(), aliases,
		inheritChoice(autoconnLabel, onOff(inh.Autoconnect), "on", "off"),
		inheritChoice(reconnectLabel, onOff(inh.Reconnect), "on", "off"),
		inheritChoice(notifyLabel, inh.Notify, "all", "first", "attention", "none"),
		inheritChoice(echoLabel, onOff(inh.LocalEcho), "on", "off"),
		primaryButton(saveLabel), buttonField(forgetPWLabel), buttonField(delCharLabel))
	f.title = str.EditorEditing(name)
	f.fields[f.field(aliasesLabel)].in.SetValue(strings.Join(s.Aliases, ", "))
	f.setBool(autoconnLabel, s.Autoconnect)
	f.setBool(reconnectLabel, s.Reconnect)
	if s.Notify != nil {
		f.choose(f.field(notifyLabel), *s.Notify)
	}
	f.setBool(echoLabel, s.LocalEcho)
	return f
}

// openWorldEditor opens the editor for world, or for a new world if world
// is "".
func (m *Model) openWorldEditor(world string) {
	if world == "" {
		var s config.WorldSettings
		if _, err := os.Stat(filepath.Join(m.d.ConfigDir, "packs", "fuzzball.toml")); err == nil {
			s.Use = []string{"fuzzball"} // as AddWorld does
		}
		inh, err := config.Defaults(m.d.ConfigDir)
		if err != nil {
			m.setStatus(true, err.Error())
			return
		}
		f := m.worldForm(true, s, inh)
		f.title = str.EditorNewWorld()
		m.setEditor(&editor{form: f, kind: addWorld})
		return
	}
	s, inh, err := config.ReadWorld(m.d.ConfigDir, world)
	if err != nil {
		m.setStatus(true, err.Error())
		return
	}
	f := m.worldForm(false, s, inh)
	f.title = str.EditorEditing(world)
	m.setEditor(&editor{form: f, kind: editWorld, world: world})
}

// openCharEditor opens the editor for character k.
func (m *Model) openCharEditor(k string) {
	ch, ok := m.find(k)
	if !ok {
		return
	}
	s, inh, err := config.ReadCharacter(m.d.ConfigDir, ch.World, ch.ID)
	if err != nil {
		m.setStatus(true, err.Error())
		return
	}
	m.setEditor(&editor{form: m.charForm(ch.World+"/"+ch.Name, s, inh), kind: editChar, world: ch.World, char: ch.ID})
}

// editCommand opens an editor from /edit: the active character's, or
// with "world" its world's. The picker opens under it and closes with it.
func (m *Model) editCommand(cs *charState, arg string) {
	if m.picker == nil {
		m.openPicker()
		if m.picker == nil {
			return // openPicker said why
		}
	}
	if arg == "world" {
		m.openWorldEditor(cs.ch.World)
	} else {
		m.openCharEditor(cs.key)
	}
	if m.picker.edit != nil {
		m.picker.edit.closeAll = true
	}
}

// editWorld opens world's editor from its overview, as editCommand does
// for a character: the picker opens under it and closes with it.
func (m *Model) editWorld(world string) {
	if m.picker == nil {
		m.openPicker()
		if m.picker == nil {
			return // openPicker said why
		}
	}
	m.openWorldEditor(world)
	if m.picker.edit != nil {
		m.picker.edit.closeAll = true
	}
}

// target says what e adds or edits; a draft comes back to the same one.
func (e *editor) target() string { return fmt.Sprint(e.kind, "/", e.world, "/", e.char) } //str:ok

// setEditor opens e over the picker, or in its place the draft that
// Ctrl+T hid for the same target.
func (m *Model) setEditor(e *editor) {
	if d := m.drafts[e.target()]; d != nil {
		delete(m.drafts, e.target())
		d.armed, d.closeAll = "", false // the caller sets closeAll for where it's opened from now
		e = d
	}
	m.picker.edit = e
}

// hideEditor closes the editor but keeps its edits, to come back the
// next time the same world or character's editor opens.
func (m *Model) hideEditor() {
	if m.drafts == nil {
		m.drafts = map[string]*editor{}
	}
	m.drafts[m.picker.edit.target()] = m.picker.edit
	m.closeEditor()
	m.setStatus(false, str.StatusDraftKept())
}

// closeEditor goes back to the picker, or out of it for /edit.
func (m *Model) closeEditor() {
	if m.picker.edit.closeAll {
		m.closePicker()
		return
	}
	m.picker.edit = nil
}

// editorKey handles a key while an editor is open over the picker.
func (m *Model) editorKey(k tea.KeyPressMsg) tea.Cmd {
	e := m.picker.edit
	if k.String() != "enter" {
		e.armed = ""
	}
	switch k.String() {
	case "esc", "ctrl+c":
		m.closeEditor()
	case openEditorKey: // the key that opened it hides it, edits and all
		m.hideEditor()
	case "enter":
		if e.form.key(k) { // moved to the next field, or opened the extras
			e.armed = ""
			return nil
		}
		return m.editorEnter(e)
	default:
		e.form.key(k)
	}
	return nil
}

// editorEnter handles Enter where the form doesn't: on a button, or in
// the one-field character name form.
func (m *Model) editorEnter(e *editor) tea.Cmd {
	f := e.form
	switch btn := f.pressedLabel(); {
	case e.kind == addChar:
		return m.saveCharacter()
	case btn == saveLabel && e.kind == addWorld:
		m.saveNewWorld()
	case btn == saveLabel && e.kind == editWorld:
		m.saveWorld()
	case btn == saveLabel && e.kind == editChar:
		m.saveCharSettings()
	case btn == forgetPWLabel:
		m.forgetPassword(e)
	case btn == delCharLabel || btn == delWorldLabel:
		if e.armed != btn {
			e.armed = btn
			f.reject = m.deleteWarning(e)
			return nil
		}
		m.deleteEdited(e)
	}
	return nil
}

// deleteWarning says what a delete will do, asking for Enter again.
func (m *Model) deleteWarning(e *editor) string {
	if e.kind == editWorld {
		return str.EditorDeleteWorldWarning(e.world)
	}
	return str.EditorDeleteCharacterWarning(e.world + "/" + e.char)
}

// saveNewWorld writes the new world, then highlights its row for adding a
// character. A problem is shown in the editor, which stays.
func (m *Model) saveNewWorld() {
	f := m.picker.edit.form
	s, err := worldSettings(f)
	if err != nil {
		f.reject = err.Error()
		return
	}
	id := f.value(f.field(worldLabel))
	if err := config.AddWorld(m.d.ConfigDir, id, s.Host, s.Port, s.TLS); err != nil {
		f.reject = err.Error()
		return
	}
	if err := config.WriteWorld(m.d.ConfigDir, id, s); err != nil {
		config.DeleteWorld(m.d.ConfigDir, id) // take back the half-made world
		f.reject = err.Error()
		return
	}
	m.picker.edit = nil
	m.picker.form.fields[0].in.SetValue("") // so the new world is listed
	m.reloadNow()
	m.picker.sel = addCharSel(id)
	m.fixPick()
}

// saveWorld writes an edited world.
func (m *Model) saveWorld() {
	e := m.picker.edit
	s, err := worldSettings(e.form)
	if err == nil {
		err = config.WriteWorld(m.d.ConfigDir, e.world, s)
	}
	if err != nil {
		e.form.reject = err.Error()
		return
	}
	m.closeEditor()
	if m.reloadNow() {
		m.setStatus(false, str.EditorSaved(e.world))
	}
}

// saveCharSettings writes an edited character.
func (m *Model) saveCharSettings() {
	e := m.picker.edit
	f := e.form
	s := config.CharacterSettings{Aliases: list(f.value(f.field(aliasesLabel))),
		Autoconnect: f.optBool(autoconnLabel), Reconnect: f.optBool(reconnectLabel),
		LocalEcho: f.optBool(echoLabel)}
	if v := f.chosen(f.field(notifyLabel)); v != "" {
		s.Notify = &v
	}
	if err := config.WriteCharacter(m.d.ConfigDir, e.world, e.char, s); err != nil {
		f.reject = err.Error()
		return
	}
	m.closeEditor()
	if m.reloadNow() {
		m.setStatus(false, str.EditorSaved(e.world+"/"+e.char))
	}
}

// forgetPassword deletes the edited character's saved password.
func (m *Model) forgetPassword(e *editor) {
	if m.d.DeletePassword == nil {
		return
	}
	if err := m.d.DeletePassword(m.passwordStore(), e.world, e.char); err != nil {
		e.form.reject = err.Error()
		return
	}
	m.setStatus(false, str.EditorForgotPassword(e.world, e.char))
	m.closeEditor()
}

// deleteEdited deletes the edited world or character: a character's
// entry and saved password (it closes first if it's open), or a world
// with no characters. Logs are never touched.
func (m *Model) deleteEdited(e *editor) {
	var err error
	what := e.world
	if e.kind == editChar {
		what = e.world + "/" + e.char
		err = config.DeleteCharacter(m.d.ConfigDir, e.world, e.char)
		if err == nil {
			m.close(key(e.world, e.char))
			if m.d.DeletePassword != nil {
				if perr := m.d.DeletePassword(m.passwordStore(), e.world, e.char); perr != nil {
					m.setStatus(true, str.EditorDeletedPasswordKept(what, perr))
				}
			}
		}
	} else {
		err = config.DeleteWorld(m.d.ConfigDir, e.world)
		if err == nil {
			for t, d := range m.drafts { // drafts for the world are moot; one would reappear for a new one by its name
				if d.world == e.world {
					delete(m.drafts, t)
				}
			}
		}
	}
	if err != nil {
		e.armed = ""
		e.form.reject = err.Error()
		return
	}
	m.closeEditor()
	status, isErr := m.status, m.statusErr
	if m.reloadNow() && !isErr {
		m.setStatus(false, str.EditorDeleted(what))
	} else if isErr {
		m.setStatus(true, status)
	}
	if m.picker != nil {
		m.fixPick()
	}
}
