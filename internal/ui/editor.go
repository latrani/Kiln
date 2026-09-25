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

// Field and button labels.
const (
	extraLabel    = "Additional settings"
	saveLabel     = "Save"
	forgetPWLabel = "Forget saved password"
	delCharLabel  = "Delete character"
	delWorldLabel = "Delete world"
	editorHint    = "Esc to cancel"
)

var (
	worldIDChar = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)
	portChar    = regexp.MustCompile(`^[0-9]{0,5}$`)
	numChar     = regexp.MustCompile(`^[0-9]{0,9}$`)
	packsChar   = regexp.MustCompile(`^[A-Za-z0-9_, -]*$`)
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

// inheritChoice is a choice between inheriting (shown with what that
// gives) and the given options.
func inheritChoice(label, inherits string, options ...string) field {
	return choiceField(label, append([]string{""}, options...), append([]string{"default (" + inherits + ")"}, options...))
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
		world := textField("World")
		world.accept = onlyMatching(worldIDChar, "a world id can only use letters, digits, _ and -")
		fields = append(fields, world)
	}
	host, port := textField("Host"), textField("Port")
	host.accept = func(s string) error {
		if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r > '~' }) {
			return errors.New("a host can't have spaces")
		}
		return nil
	}
	port.accept = onlyMatching(portChar, "a port is a number")
	packs := withHint(textField("Packs"), "none")
	packs.accept = onlyMatching(packsChar, "pack ids, separated by commas")
	login := "none"
	if inh.Login != "" {
		login = inh.Login
	}
	maxBytes := withHint(textField("Max bytes"), "default: "+strconv.Itoa(inh.MaxLineBytes))
	maxBytes.accept = onlyMatching(numChar, "a number of bytes")
	fields = append(fields, host, port, toggleField("TLS"), sectionField(extraLabel),
		asExtra(inheritChoice("Cert trust", "pin", "pin", "ca")),
		asExtra(packs),
		asExtra(withHint(textField("Login"), "default: "+login)),
		asExtra(maxBytes),
		asExtra(inheritChoice("Newlines", inh.NewlineMode, "batch", "flatten")),
		asExtra(inheritChoice("Autoconnect", onOff(inh.Autoconnect), "on", "off")),
		asExtra(inheritChoice("Local echo", onOff(inh.LocalEcho), "on", "off")),
		buttonField(saveLabel))
	if !add {
		fields = append(fields, buttonField(delWorldLabel))
	}
	f := newForm(editorHint, fields...)
	f.fields[f.field("Host")].in.SetValue(s.Host)
	if s.Port > 0 {
		f.fields[f.field("Port")].in.SetValue(strconv.Itoa(s.Port))
	}
	f.fields[f.field("TLS")].on = s.TLS
	f.choose(f.field("Cert trust"), s.TLSTrust)
	f.fields[f.field("Packs")].in.SetValue(strings.Join(s.Use, ", "))
	if s.Login != nil {
		f.fields[f.field("Login")].in.SetValue(*s.Login)
	}
	if s.MaxLineBytes != nil {
		f.fields[f.field("Max bytes")].in.SetValue(strconv.Itoa(*s.MaxLineBytes))
	}
	if s.NewlineMode != nil {
		f.choose(f.field("Newlines"), *s.NewlineMode)
	}
	f.setBool("Autoconnect", s.Autoconnect)
	f.setBool("Local echo", s.LocalEcho)
	return f
}

// worldSettings reads a world form.
func worldSettings(f *form) (config.WorldSettings, error) {
	port, err := strconv.Atoi(f.value(f.field("Port")))
	if err != nil || port < 1 || port > 65535 {
		return config.WorldSettings{}, errors.New("port must be 1-65535")
	}
	s := config.WorldSettings{
		Host: f.value(f.field("Host")), Port: port, TLS: f.on(f.field("TLS")),
		TLSTrust: f.chosen(f.field("Cert trust")), Use: list(f.value(f.field("Packs"))),
		Autoconnect: f.optBool("Autoconnect"), LocalEcho: f.optBool("Local echo"),
	}
	if v := f.value(f.field("Login")); v != "" {
		s.Login = &v
	}
	if v := f.value(f.field("Max bytes")); v != "" {
		n, _ := strconv.Atoi(v)
		s.MaxLineBytes = &n
	}
	if v := f.chosen(f.field("Newlines")); v != "" {
		s.NewlineMode = &v
	}
	return s, nil
}

// charForm builds the form for editing a character.
func (m *Model) charForm(name string, s config.CharacterSettings, inh config.Inherited) *form {
	aliases := withHint(textField("Aliases"), "none")
	aliases.accept = func(v string) error {
		for _, a := range list(v) {
			if err := config.NameChars(a); err != nil {
				return err
			}
		}
		return nil
	}
	f := newForm(editorHint, sectionField(extraLabel),
		asExtra(aliases),
		asExtra(inheritChoice("Autoconnect", onOff(inh.Autoconnect), "on", "off")),
		asExtra(inheritChoice("Local echo", onOff(inh.LocalEcho), "on", "off")),
		buttonField(saveLabel), buttonField(forgetPWLabel), buttonField(delCharLabel))
	f.title = "Editing " + name
	f.fields[f.field("Aliases")].in.SetValue(strings.Join(s.Aliases, ", "))
	f.setBool("Autoconnect", s.Autoconnect)
	f.setBool("Local echo", s.LocalEcho)
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
			m.setStatus(true, "%v", err)
			return
		}
		f := m.worldForm(true, s, inh)
		f.title = "New world"
		m.picker.edit = &editor{form: f, kind: addWorld}
		return
	}
	s, inh, err := config.ReadWorld(m.d.ConfigDir, world)
	if err != nil {
		m.setStatus(true, "%v", err)
		return
	}
	f := m.worldForm(false, s, inh)
	f.title = "Editing " + world
	m.picker.edit = &editor{form: f, kind: editWorld, world: world}
}

// openCharEditor opens the editor for character k.
func (m *Model) openCharEditor(k string) {
	ch, ok := m.find(k)
	if !ok {
		return
	}
	s, inh, err := config.ReadCharacter(m.d.ConfigDir, ch.World, ch.ID)
	if err != nil {
		m.setStatus(true, "%v", err)
		return
	}
	m.picker.edit = &editor{form: m.charForm(ch.World+"/"+ch.Name, s, inh), kind: editChar, world: ch.World, char: ch.ID}
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
		return fmt.Sprintf("Enter again to delete %s (logs are kept)", e.world)
	}
	return "Enter again to delete it and its saved password (logs are kept)"
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
	id := f.value(f.field("World"))
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
		m.setStatus(false, "saved %s", e.world)
	}
}

// saveCharSettings writes an edited character.
func (m *Model) saveCharSettings() {
	e := m.picker.edit
	f := e.form
	s := config.CharacterSettings{Aliases: list(f.value(f.field("Aliases"))),
		Autoconnect: f.optBool("Autoconnect"), LocalEcho: f.optBool("Local echo")}
	if err := config.WriteCharacter(m.d.ConfigDir, e.world, e.char, s); err != nil {
		f.reject = err.Error()
		return
	}
	m.closeEditor()
	if m.reloadNow() {
		m.setStatus(false, "saved %s/%s", e.world, e.char)
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
	m.setStatus(false, "forgot %s/%s's saved password", e.world, e.char)
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
					m.setStatus(true, "deleted %s, but its password wasn't: %v", what, perr)
				}
			}
		}
	} else {
		err = config.DeleteWorld(m.d.ConfigDir, e.world)
	}
	if err != nil {
		e.armed = ""
		e.form.reject = err.Error()
		return
	}
	m.closeEditor()
	status, isErr := m.status, m.statusErr
	if m.reloadNow() && !isErr {
		m.setStatus(false, "deleted %s (logs kept)", what)
	} else if isErr {
		m.setStatus(true, "%s", status)
	}
	if m.picker != nil {
		m.fixPick()
	}
}
