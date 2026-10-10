// Package ui is Kiln's terminal interface: a sidebar of worlds and
// characters, and a right pane with scrollback, input box and statusline.
package ui

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// HistoryLines is how many logged lines are preloaded per character.
const HistoryLines = app.HistoryLines

// Deps are the UI's connections to the outside world. Tests substitute
// fakes; cmd/kiln wires the real ones.
type Deps struct {
	ConfigDir  string
	LogRoot    string // where log_dir is relative to; "" (and no absolute log_dir): no history
	KnownHosts conn.KnownHosts
	Load       func(dir string) (*config.Config, error)
	Dial       func(ctx context.Context, ch config.Character) (session.LineConn, error)
	NewLog     func(l logstore.Layout) session.Appender // l from logLayout
	// Password and SavePassword use the password_store setting in store.
	Password       func(store, world, char string) (string, error)
	SavePassword   func(store, world, char, password string) error // nil: never offer
	DeletePassword func(store, world, char string) error           // nil: passwords can't be forgotten
	Changes        <-chan struct{}                                 // config changes; nil: no hot reload
	OpenURL        func(url string) error                          // opens a clicked link; nil: links do nothing
	SaveFile       func(name string, data []byte) error            // offers a file to download (web); nil: browse save writes under export_dir
	Backup         func() (name string, err error)                 // downloads a backup of config and pins (web); nil: no Back up
	Restore        func() (n int, err error)                       // asks for a backup and writes its n files back; 0, nil when cancelled; nil: no Restore
	Tmux           bool                                            // inside tmux: wrap notifications and clipboard writes for passthrough
	Remote         bool                                            // over ssh: links can't open on your screen, so a click copies them
	NoAutoconnect  bool                                            // start with nothing open, whatever the characters' autoconnect says
	Raw            func(seq string) tea.Cmd                        // writes straight to the terminal; default tea.Raw
	Now            func() time.Time
}

// Model is the Bubble Tea model.
type Model struct {
	d           Deps
	a           *app.App              // what Kiln decides; Model is its terminal front end
	picker      *picker               // non-nil while the open-connection picker is open
	idle        *Input                // the input box while nothing is open
	chars       map[string]*charState // the open characters' views, keyed like the core's
	width       int
	height      int
	resizeGen   int      // bumped per WindowSizeMsg; see resizeMsg
	sideTop     int      // first sidebar row shown when it overflows
	sideShown   string   // active character last scrolled into view
	quitKey     string   // "ctrl+c" or "ctrl+d" once pressed on an empty input; again quits
	quitGen     int      // bumped per arming; see quitExpiredMsg
	logStatus   int      // the status Gen of a message log mode put up; its next key or click clears that one
	statusTimed int      // the statusGen whose expiry is scheduled
	lastClick   struct { // for spotting a double-click in the sidebar
		char string
		at   time.Time
	}
	backingUp     bool               // a Back up is running; another is ignored until it's done
	shownPresence app.Presence       // what the presence chip shows; see Update
	themed        bool               // a theme has been loaded; see loadTheme
	standIn       bool               // the theme is the built-in standing in for a broken one; see loadTheme
	themeErr      error              // why the theme didn't load, to report once the update is done
	detected      theme.Appearance   // the terminal's last answer about its background; Dark until one comes
	ovTop         int                // first row of a world's overview shown; see overview
	ovKeys        []string           // the character each overview row shows, from the last draw; "" for a rule
	drafts        map[string]*editor // editors hidden by Ctrl+T, unsaved, by target; see hideEditor
	parked        map[string]*editor // editors left open on a sidebar item while another is active, by m.a.Active(); see parkEditor
}

type charState struct {
	*app.Char                    // the core's half; ui only reads it
	hl        *rules.Highlighter // the character's look: the theme with its own looks on top
	sb        Scrollback
	in        *Input
	browse    *browse         // non-nil while browse mode is open
	hidBrowse *browse         // browse mode as Ctrl+L left it, kept up to date; the next Ctrl+L brings it back
	filter    scene.Filter    // log mode's filter; outlasts a log-mode session
	collapsed map[string]bool // filter panel parents folded shut, by tag
	foldSeen  map[string]bool // parents the panel has already met; a new one starts folded
}

// browses is log mode, showing or hidden by Ctrl+L, for what keeps
// either up to date.
func (cs *charState) browses() []*browse {
	var bs []*browse
	for _, b := range []*browse{cs.browse, cs.hidBrowse} {
		if b != nil {
			bs = append(bs, b)
		}
	}
	return bs
}

// hideBrowse leaves log mode as it is, filter panel, cursor, marks and
// all, to come back on the next Ctrl+L. Esc closes it for good.
func (cs *charState) hideBrowse() {
	if cs.browse.stopSearch() {
		cs.browse.status = "" // not news by the time it's back
	}
	cs.browse, cs.hidBrowse = nil, cs.browse
}

func key(world, char string) string { return app.Key(world, char) }

// Messages.
type (
	reloadMsg struct{}
	tickMsg   time.Time
	// quitExpiredMsg fires quitWindow after a quit key is armed; it
	// carries that arming's generation, and only the latest disarms.
	quitExpiredMsg int
	// statusExpiredMsg fires statusTimeout after a status is set; it
	// carries that status's generation, and only the latest clears.
	statusExpiredMsg int
)

// New builds the model from an already-loaded config and opens the
// autoconnect characters, preloading their recent history. Nothing
// connects until Init.
func New(d Deps, cfg *config.Config) *Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Raw == nil {
		d.Raw = func(seq string) tea.Cmd { return tea.Raw(seq) }
	}
	m := &Model{d: d, chars: map[string]*charState{}, idle: NewInput()}
	m.a = app.New(app.Deps{
		ConfigDir: d.ConfigDir, KnownHosts: d.KnownHosts, SavePassword: d.SavePassword,
		LogRoot: d.LogRoot, Load: d.Load, Dial: d.Dial, NewLog: d.NewLog, Password: d.Password,
		Now: func() time.Time { return m.d.Now() }, // late-bound: tests swap the clock
	})
	m.idle.hist = m.a.IdleHistory()
	m.applyWith(func() (app.ConfigResult, bool) { return m.a.ApplyConfig(cfg), true })
	m.shownPresence = m.a.Presence()
	m.loadTheme() // at start a broken theme falls back to the built-in, and says so
	if m.themeErr != nil {
		m.setStatus(true, str.StatusThemeNotLoaded(m.themeErr))
		m.themeErr = nil
	}
	for _, ch := range m.a.AllChars() {
		if ch.Autoconnect && !d.NoAutoconnect {
			m.open(key(ch.World, ch.ID))
		}
	}
	return m
}

// Init connects autoconnect characters and starts the clock and watcher.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(m.d.Now()), m.watch(), m.askBackground()}
	for _, k := range m.a.Order() {
		if m.chars[k].Ch.Autoconnect && !m.d.NoAutoconnect {
			cmds = append(cmds, m.connect(m.chars[k]))
		}
	}
	return tea.Batch(cmds...)
}

// tick wakes the model at the next local midnight, when "Connected
// since" gains its day.
func tick(now time.Time) tea.Cmd {
	next := nextMidnight(now.Local())
	return tea.Tick(next.Sub(now), func(t time.Time) tea.Msg { return tickMsg(t) })
}

// nextMidnight is the start of the day after now's, in now's zone.
// Where daylight saving starts at midnight that instant doesn't exist
// and time.Date lands before now, so it steps on to the first hour that
// comes after.
func nextMidnight(now time.Time) time.Time {
	y, mo, d := now.Date()
	next := time.Date(y, mo, d+1, 0, 0, 0, 0, now.Location())
	for !next.After(now) {
		next = next.Add(time.Hour)
	}
	return next
}

func (m *Model) watch() tea.Cmd {
	if m.d.Changes == nil {
		return nil
	}
	ch := m.d.Changes
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return nil
		}
		return reloadMsg{}
	}
}

// reloadNow loads the config and theme and applies them, reporting whether
// the config could be.
func (m *Model) reloadNow() bool {
	if !m.applyWith(m.a.Reload) {
		return false // Reload said why
	}
	m.loadTheme() // a broken theme doesn't stop the config; see themeErr
	return true
}

// appearance is the palette to draw with: the setting, or with auto the
// terminal's last answer (dark until one comes).
func (m *Model) appearance() theme.Appearance {
	switch m.a.Config().Appearance {
	case "light":
		return theme.Light
	case "dark":
		return theme.Dark
	}
	return m.detected
}

// askBackground asks the terminal for its background, when the
// appearance is auto.
func (m *Model) askBackground() tea.Cmd {
	if m.a.Config().Appearance != "auto" {
		return nil
	}
	return tea.RequestBackgroundColor
}

// loadTheme reads the theme from the config folder and draws with it,
// restyling what's on screen. A broken theme changes nothing, except at
// start, when the built-in theme stands in; either way its error is
// reported when the update ends (see Update), after any message the
// caller shows for what it did.
func (m *Model) loadTheme() {
	th, err := theme.Load(m.d.ConfigDir, m.a.Config().Theme, m.appearance())
	m.themeErr = err
	if err != nil && m.themed && !m.standIn {
		return // keep the theme we have
	}
	m.themed, m.standIn = true, err != nil // a stand-in built-in still follows the appearance
	if th == theme.Active() || th.Equal(theme.Active()) {
		return // restyling would drop a selection for nothing
	}
	theme.SetActive(th)
	for _, k := range m.a.Order() { // in order, so which error shows is settled
		cs := m.chars[k]
		if err := cs.compileLook(); err != nil { // each highlighter holds the theme it was built on
			m.setStatus(true, str.StatusCharError(k, err))
		}
		cs.sb.Repaint(func(l app.Line) string { return paint(cs.hl, l) })
		for _, b := range cs.browses() {
			b.restyle(func(l app.Line) string { return paint(cs.hl, l) })
		}
	}
}

// applyWith hands a config to the core through apply, then updates the
// open characters' views: export settings, looks, and lines when what
// styles them changed. It reports false if apply changed nothing.
func (m *Model) applyWith(apply func() (app.ConfigResult, bool)) bool {
	keys := slices.Clone(m.a.Order())
	before := map[string]config.Character{}
	for _, k := range keys {
		before[k] = m.chars[k].Ch
	}
	prev := m.a.Active()
	res, ok := apply()
	if !ok {
		return false
	}
	for _, k := range res.Closed {
		m.closed(k, prev)
	}
	m.activated(prev)
	for _, k := range keys { // in order, so which error shows is settled
		cs := m.chars[k]
		if cs == nil {
			continue
		}
		for _, b := range cs.browses() {
			b.setExport(m.a.Config())
		}
		if cs.Orphan {
			continue
		}
		if err := res.Errs[k]; err != nil {
			m.setStatus(true, str.StatusCharError(k, err))
			continue
		}
		if err := cs.compileLook(); err != nil {
			m.setStatus(true, str.StatusCharError(k, err))
		}
		if !reflect.DeepEqual(styleInputs(before[k]), styleInputs(cs.Ch)) {
			cs.sb.Repaint(func(l app.Line) string { return paint(cs.hl, l) }) // the core has made them again if the rules changed
		}
	}
	if m.picker != nil {
		m.fixPick()
	}
	return true
}

// styleInputs is what a character's scrollback styling depends on: the
// rules, its looks, and the name and aliases that classify its own lines.
func styleInputs(ch config.Character) any {
	return struct {
		rules   config.Rules
		looks   []theme.Layer
		name    string
		aliases []string
	}{ch.Rules, ch.Looks, ch.Name, ch.Aliases}
}

// compileLook builds cs's highlighter from the active theme with the
// character's own looks on top. Looks that don't resolve (a color nobody
// defines) leave the theme's own tag styles in use; either way the error
// is returned for the caller to show.
func (cs *charState) compileLook() error {
	th, err := theme.Active().With(cs.Ch.Looks...)
	if err != nil {
		th = theme.Active()
	}
	cs.hl = rules.New(th)
	return err
}

// preload has the core read cs's history again and shows it: what Open
// does, kept for tests that write logs after the harness opens Kit.
func (m *Model) preload(cs *charState) {
	m.a.Preload(cs.Key)
	m.showLines(cs)
}

// showLines builds cs's view from the core's lines, painted.
func (m *Model) showLines(cs *charState) {
	for _, l := range cs.Lines {
		cs.sb.AppendLine(lineOf(paint(cs.hl, *l), l))
	}
	cs.sb.SetMore(cs.More)
}

// pageOlder has the core read the next older page of the current
// character's history, when its view has scrolled past the oldest line.
func (m *Model) pageOlder() tea.Cmd {
	cs := m.cur()
	if cs == nil || cs.browse != nil || !cs.sb.RequestOlder(m.layout().sbH) {
		return nil
	}
	effs := m.a.RequestOlder(cs.Key)
	if effs == nil { // the core has nothing more after all
		cs.sb.PrependLines(nil, cs.More)
	}
	return m.run(effs)
}

// echoes reports whether e belongs in the scrollback: everything but sent
// lines, which only show with local_echo on. They're logged either way.
func (cs *charState) echoes(e logstore.Entry) bool {
	return e.Dir != logstore.Out || cs.Ch.LocalEcho
}

// render makes e into a line and paints it.
func (cs *charState) render(e logstore.Entry) (string, app.Line) {
	l := cs.Rules.Line(e)
	return paint(cs.hl, l), l
}

// connect starts (or restarts) a character's session.
func (m *Model) connect(cs *charState) tea.Cmd { return m.run(m.a.Connect(cs.Key)) }

// run performs the core's effects: blocking work as commands, quitting
// as tea.Quit.
func (m *Model) run(effs []app.Effect) tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range effs {
		switch e := e.(type) {
		case app.Run:
			f := e.Func
			cmds = append(cmds, func() tea.Msg { return f() })
		case app.Quit:
			cmds = append(cmds, tea.Quit)
		case app.Do:
			cmds = append(cmds, m.do(e))
		case app.Notify:
			cmds = append(cmds, m.encode(notify.Message(e.Title, e.Body)))
		}
	}
	return tea.Batch(cmds...)
}

// resizeDebounce is how long the window size must hold still before it is
// reported to servers, so a drag-resize doesn't flood them with NAWS.
const resizeDebounce = 200 * time.Millisecond

// resizeMsg fires resizeDebounce after a WindowSizeMsg; it carries that
// message's generation, and only the latest generation reports.
type resizeMsg int

// StatusMsg puts Text on the statusline (as an error when Err is set):
// how a front end, like the web build, tells the user something.
type StatusMsg struct {
	Text string
	Err  bool
}

// backupDoneMsg is what the Backup hook returned.
type backupDoneMsg struct {
	name string
	err  error
}

// restoreDoneMsg is what the Restore hook returned.
type restoreDoneMsg struct {
	n   int
	err error
}

// paneSize is the right pane's size, where server text is shown: what
// NAWS reports. It is 0×0 before the first WindowSizeMsg.
func (m *Model) paneSize() (w, h int) {
	if m.width == 0 {
		return 0, 0
	}
	return m.layout().rw, m.height
}

// Update handles one message, then pages in older scrollback if the
// view has run past it.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	if _, blur := msg.(tea.BlurMsg); !blur {
		// Switching away leaves the chip be: nobody's looking, and in tmux
		// redrawing it would flag the window as active every time you left.
		// It catches up with whatever next changes.
		m.shownPresence = m.a.Presence()
	}
	m.takeLogStatus()
	if older := m.pageOlder(); older != nil {
		cmd = tea.Batch(cmd, older)
	}
	if m.themeErr != nil { // last, so it isn't covered by what the update said
		m.setStatus(true, str.StatusThemeNotLoaded(m.themeErr))
		m.themeErr = nil
	}
	if g := m.a.Status().Gen; g != m.statusTimed { // a new status: time it out
		m.statusTimed = g
		cmd = tea.Batch(cmd, tea.Tick(statusTimeout, func(time.Time) tea.Msg { return statusExpiredMsg(g) }))
	}
	return m, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.a.SetPane(m.paneSize())
		m.resizeGen++
		gen := m.resizeGen
		return m, tea.Tick(resizeDebounce, func(time.Time) tea.Msg { return resizeMsg(gen) })
	case resizeMsg:
		if int(msg) == m.resizeGen { // the drag has settled
			m.a.ReportSize()
		}
	case StatusMsg:
		m.setStatus(msg.Err, msg.Text)
	case backupDoneMsg:
		m.backingUp = false
		if msg.err != nil {
			m.setStatus(true, str.StatusBackupFailed(msg.err))
		} else {
			m.setStatus(false, str.StatusDownloaded(msg.name))
		}
	case restoreDoneMsg:
		switch {
		case msg.err != nil:
			m.setStatus(true, str.StatusRestoreFailed(msg.err))
		case msg.n > 0 && m.reloadNow(): // reloadNow says why it didn't
			m.setStatus(false, str.StatusRestored(msg.n))
		}
	case tickMsg:
		return m, tick(time.Time(msg))
	case quitExpiredMsg:
		if int(msg) == m.quitGen {
			m.disarmQuit()
		}
	case statusExpiredMsg:
		if int(msg) == m.a.Status().Gen {
			m.a.ClearStatus()
		}
	case reloadMsg:
		if m.reloadNow() {
			m.setStatus(false, str.StatusConfigReloaded())
		}
		return m, tea.Batch(m.watch(), m.askBackground())
	case app.SessionMsg:
		return m, m.handleEvent(msg)
	case tea.FocusMsg:
		m.a.Focus(true)
		return m, m.askBackground() // the terminal may have gone light or dark meanwhile
	case tea.BackgroundColorMsg:
		ap := theme.Light
		if msg.IsDark() {
			ap = theme.Dark
		}
		if ap != m.detected {
			m.detected = ap
			if m.a.Config().Appearance == "auto" {
				m.loadTheme()
			}
		}
	case tea.BlurMsg:
		m.a.Focus(false)
	case app.OlderMsg:
		if lines, ok := m.a.HandleOlder(msg); ok {
			cs := m.chars[msg.Key]
			batch := make([]sbLine, len(lines))
			for i, l := range lines {
				batch[i] = lineOf(paint(cs.hl, *l), l)
			}
			cs.sb.PrependLines(batch, cs.More)
		}
	case olderMsg:
		if cs := m.chars[msg.key]; cs != nil && slices.Contains(cs.browses(), msg.b) {
			return m, msg.b.receive(msg)
		}
	case tea.PasteMsg:
		m.a.Here() // only a focused window gets input, even if its focus-in was lost
		if m.picker != nil && m.picker.edit != nil {
			m.picker.edit.form.paste(msg.Content)
		} else if m.picker != nil {
			m.picker.form.paste(msg.Content)
			m.fixPick()
		} else if cs := m.cur(); cs != nil && cs.browse != nil {
			// Only a text prompt takes a paste; the chat draft must not.
			if p := cs.browse.prompt; p == promptFind || p == promptDate || p == promptFilename {
				cs.browse.pin.InsertText(oneLine(msg.Content))
			}
		} else if _, _, asking := m.a.PendingSave(); !asking && !m.overviewing() {
			m.input().InsertText(msg.Content)
			m.a.Unconfirm()
			m.pushInput()
		}
	case tea.KeyPressMsg:
		m.a.Here()
		before, gen := m.input().Value(), m.a.Status().Gen
		cmd := m.handleKey(msg)
		m.pushInput()
		if m.a.Status().Gen == gen && m.input().Value() != before {
			m.a.ClearStatus() // typing again dismisses what was said before
		}
		return m, cmd
	case tea.MouseWheelMsg:
		m.a.Scrolled()
		return m, m.handleWheel(msg)
	case tea.MouseClickMsg:
		was := m.shownPresence // as drawn, before Here clears an Away that a click on the chip toggles
		m.a.Here()
		return m, m.handleClick(msg, was)
	case tea.MouseMotionMsg:
		m.handleDrag(msg.Mouse())
	case tea.MouseReleaseMsg:
		return m, m.handleRelease()
	}
	return m, nil
}

func (m *Model) cur() *charState { return m.chars[m.a.Active()] }

// input is the active character's input box, or the idle one.
func (m *Model) input() *Input {
	if cs := m.cur(); cs != nil {
		return cs.in
	}
	return m.idle
}

// statusTimeout is how long a status message stays up. Typing clears it
// sooner.
const statusTimeout = 30 * time.Second

func (m *Model) setStatus(isErr bool, msg string) { m.a.SetStatus(isErr, msg) }

// takeLogStatus moves a message log mode just set into the bottom bar's
// status, so it times out like any other and the newest message wins.
func (m *Model) takeLogStatus() {
	if cs := m.cur(); cs != nil && cs.browse != nil && cs.browse.status != "" {
		m.setStatus(cs.browse.statusErr, cs.browse.status)
		m.logStatus = m.a.Status().Gen
		cs.browse.status = ""
	}
}

// clearLogStatus drops a message from log mode when you act in it again,
// as log mode always has. A newer message (log mode's or anyone's)
// stays: it's news.
func (m *Model) clearLogStatus() {
	if m.logStatus != 0 && m.a.Status().Gen == m.logStatus {
		m.a.ClearStatus()
	}
	m.logStatus = 0
}

func (m *Model) handleEvent(msg app.SessionMsg) tea.Cmd {
	prev := m.a.Active()
	ev, ok, effs := m.a.Handle(msg)
	if !ok {
		return nil // stale session, or it has shut down
	}
	if ev.Closed {
		m.closed(ev.Key, prev)
		m.activated(prev)
		return nil
	}
	cs := m.chars[ev.Key]
	m.pullInput(cs) // the password prompt may have started or ended
	next := m.run(effs)
	switch ev.Ev.Kind {
	case session.EventLine:
		if ev.Shown {
			text := paint(cs.hl, *ev.Line)
			if ev.Line.Quiet {
				cs.sb.append(lineOf(text, ev.Line))
			} else {
				cs.sb.AppendLine(lineOf(text, ev.Line))
			}
		}
		if ev.Key == m.a.Active() {
			l := m.layout()
			cs.sb.SetWidth(l.rw) // measure at the pane's width, even before a View
			cs.sb.Pause(l.sbH)
		}
		for _, b := range cs.browses() {
			b.appendLive(ev.Ev.Entry)
		}
	case session.EventPrompt:
		cs.sb.SetPrompt(cs.Prompt)
	}
	return next
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	if cs := m.cur(); cs != nil {
		cs.sb.ClearSelection()
	}
	m.input().ClearSelection()
	armed := m.quitKey
	if armed != "" {
		m.disarmQuit() // any key but the armed one again starts over
	}
	cs := m.cur()
	if _, _, asking := m.a.PendingSave(); asking {
		switch k.String() {
		case "enter", "y", "Y":
			m.a.AnswerSave(true)
		case "n", "N", "esc", "ctrl+c":
			m.a.AnswerSave(false)
		case openPickerKey:
			m.openPicker() // says why not
		}
		return nil
	}
	switch k.String() {
	case "ctrl+up":
		m.switchBy(-1)
		return nil
	case "ctrl+down":
		m.switchBy(1)
		return nil
	case "tab", "shift+tab":
		if m.picker == nil { // the picker's forms move between fields with Tab
			m.switchToUnread(map[string]int{"tab": 1, "shift+tab": -1}[k.String()])
			return nil
		}
	case openPickerKey:
		if m.picker == nil {
			m.openPicker() // says why not, in browse mode
			return nil
		}
	}
	if m.picker != nil {
		return m.pickerKey(k)
	}
	if cs != nil && cs.browse != nil {
		m.clearLogStatus()
		if k.String() == openBrowseKey { // the key that opened it hides it, prompt or not
			cs.hideBrowse()
			return nil
		}
		cmd, closed := cs.browse.key(k, m.browseBodyH())
		if closed {
			cs.browse = nil
		}
		return cmd
	}
	switch k.String() {
	case openBrowseKey:
		if cs != nil {
			m.openBrowse(cs)
		}
		return nil
	case openEditorKey:
		if cs != nil {
			m.editCommand(cs, "")
		} else if w, ok := m.a.ActiveWorld(); ok {
			m.editWorld(w)
		}
		return nil
	case "ctrl+c", "ctrl+d": // Ctrl+D with text deletes forward, below
		if m.input().Empty() || m.overviewing() {
			return m.armQuit(k.String(), armed)
		}
		if k.String() == "ctrl+c" {
			m.input().Reset()
		}
	case "pgup":
		if cs != nil {
			cs.sb.ScrollUp(max(1, m.layout().sbH-1))
		} else if m.overviewing() {
			m.scrollOverview(-max(1, m.layout().sbH-1))
		}
	case "pgdown":
		if cs != nil {
			cs.sb.ScrollDown(max(1, m.layout().sbH-1))
		} else if m.overviewing() {
			m.scrollOverview(max(1, m.layout().sbH-1))
		}
	case "esc":
		m.a.Unconfirm()
		if m.a.SkipLogin() {
			m.pullInput(cs)
		} else if cs != nil && cs.sb.Scrolled() {
			cs.sb.ToBottom() // back to live, from a pause or a scroll
		}
	case "enter":
		return m.submit()
	}
	if m.overviewing() {
		return nil // no input box to type in
	}
	in := m.input()
	switch k.String() {
	case "shift+enter", "alt+enter":
		in.Newline()
	case "up":
		in.Up()
	case "down":
		in.Down()
	default:
		if !editKey(in, k) {
			return nil
		}
	}
	m.a.Unconfirm()
	return nil
}

// quitWindow is how long a first Ctrl+C or Ctrl+D waits for its second.
const quitWindow = 2 * time.Second

// quitHint is the status shown while key is armed.
func quitHint(key string) string {
	return str.StatusQuitHint(strings.ToUpper(strings.TrimPrefix(key, "ctrl+"))) //str:ok
}

// armQuit quits if key was already armed (pressed just before), and
// otherwise arms it and says so for quitWindow.
func (m *Model) armQuit(key, armed string) tea.Cmd {
	if key == armed {
		return m.quit()
	}
	m.quitKey = key
	m.quitGen++
	m.setStatus(false, quitHint(key))
	gen := m.quitGen
	return tea.Tick(quitWindow, func(time.Time) tea.Msg { return quitExpiredMsg(gen) })
}

// disarmQuit forgets an armed quit key, and its hint if still shown.
func (m *Model) disarmQuit() {
	if m.quitKey != "" && m.a.Status().Text == quitHint(m.quitKey) {
		m.a.ClearStatus()
	}
	m.quitKey = ""
}

func (m *Model) quit() tea.Cmd { return m.run(m.a.Quit()) }

// switchBy moves through the sidebar's worlds and characters.
func (m *Model) switchBy(delta int) {
	if t := m.a.StepTarget(delta); t != "" {
		m.switchTo(t)
	}
}

// switchToUnread moves to the next character (dir 1) or previous one
// (dir -1) with unread lines; see app.UnreadTarget.
func (m *Model) switchToUnread(dir int) {
	if t := m.a.UnreadTarget(dir); t != "" {
		m.switchTo(t)
	}
}

// switchTo makes k active: a character's key, or a world's selection key
// (worldSel) for its overview.
func (m *Model) switchTo(k string) {
	if !m.a.CanSwitch(k) {
		return
	}
	prev := m.a.Active()
	if prev != k {
		m.parkEditor()
		if old := m.cur(); old != nil {
			old.sb.MarkSeen() // you saw it up to now
		}
	}
	m.a.Switch(k)
	if prev == k {
		m.showActive()
	} else {
		m.activated(prev)
	}
}

// showActive opens the active character's scrollback at the first line
// you haven't seen, measured at the pane's width; log mode keeps the pane
// to itself.
func (m *Model) showActive() {
	cs := m.cur()
	if cs == nil {
		return
	}
	l := m.layout()
	cs.sb.SetWidth(l.rw)
	cs.sb.Pause(l.sbH)
	if cs.browse != nil && m.picker != nil {
		m.leaveEditor()
		m.closePicker() // browse has the pane; the filter would be hidden
	}
}

// activated does the screen's part when the core has moved the active
// item away from prev: Enter asks again before an over-limit line, an
// overview starts at its top, the new character shows, and an editor left
// open on it comes back. An editor still open on prev's overview (its
// world's last character closed) keeps its edits as a draft.
func (m *Model) activated(prev string) {
	if m.a.Active() == prev {
		return
	}
	if strings.HasPrefix(prev, worldSel("")) {
		m.leaveEditor()
	}
	m.ovTop = 0
	m.showActive()
	m.unparkEditor()
}

// submit handles Enter: on a world's overview, or an empty input with
// nothing open, the picker; otherwise the core's Submit.
func (m *Model) submit() tea.Cmd {
	if m.overviewing() {
		m.openPicker() // as on an empty input with nothing open
		return nil
	}
	m.pushInput()
	if m.cur() == nil && m.a.Text() == "" {
		m.openPicker()
		return nil
	}
	prev := m.a.Active()
	res, effs := m.a.Submit()
	cmd := m.run(effs)
	m.afterCore(prev)
	if cs := m.cur(); cs != nil && res.Sent {
		cs.sb.ToBottomKeeping(res.Echoed) // what you send is where the pager picks up
	}
	return cmd
}

// afterCore catches the screen up with a core call that may have closed
// characters, moved the active item, added lines or changed texts.
func (m *Model) afterCore(prev string) {
	for k := range m.chars {
		if m.a.Char(k) == nil {
			m.closed(k, prev)
		}
	}
	m.activated(prev)
	if cs := m.cur(); cs != nil {
		m.syncLines(cs)
		m.pullInput(cs)
	}
	m.pullIdle()
}

// pushInput tells the core the active input's text, as the widget has it.
func (m *Model) pushInput() { m.a.SetInput(m.input().Value()) }

// pullInput puts cs's core text in its widget, if the core changed it.
func (m *Model) pullInput(cs *charState) {
	if cs != nil && cs.in.Value() != cs.Text {
		cs.in.Reset()
		cs.in.SetValue(cs.Text)
	}
}

// pullIdle is pullInput for the input used while nothing is open.
func (m *Model) pullIdle() {
	if m.cur() == nil && m.idle.Value() != m.a.Text() {
		m.idle.Reset()
		m.idle.SetValue(m.a.Text())
	}
}

// syncLines shows lines the core has added to cs's scrollback since the
// view last caught up (what you sent, echoed).
func (m *Model) syncLines(cs *charState) {
	for _, l := range cs.Lines[cs.sb.Len():] {
		cs.sb.AppendLine(lineOf(paint(cs.hl, *l), l))
	}
}

// do carries out a command the core hands back.
func (m *Model) do(d app.Do) tea.Cmd {
	cs := m.chars[d.Key]
	switch d.Cmd {
	case "/backup", "/restore":
		if (d.Cmd == "/backup" && m.d.Backup == nil) || (d.Cmd == "/restore" && m.d.Restore == nil) {
			m.setStatus(true, str.StatusUnknownCommand(d.Cmd)) // web-only, so unknown here
			return nil
		}
		if d.Cmd == "/backup" {
			return m.backupCmd()
		}
		return m.restoreCmd()
	case "/open":
		m.openPicker() // says why not
	case "/log":
		m.openBrowse(cs)
	case "/edit":
		m.editCommand(cs, strings.Join(d.Args, " "))
	}
	return nil
}

// backupCmd runs the Backup hook off the UI goroutine. While one runs,
// another does nothing, so a double-click downloads one zip.
func (m *Model) backupCmd() tea.Cmd {
	if m.backingUp {
		return nil
	}
	m.backingUp = true
	backup := m.d.Backup
	return func() tea.Msg { name, err := backup(); return backupDoneMsg{name, err} }
}

// restoreCmd runs the Restore hook, which waits for the user to pick a
// file, off the UI goroutine.
func (m *Model) restoreCmd() tea.Cmd {
	restore := m.d.Restore
	return func() tea.Msg { n, err := restore(); return restoreDoneMsg{n, err} }
}

// openBrowse opens browse mode for cs.
func (m *Model) openBrowse(cs *charState) {
	if cs.hidBrowse != nil { // back as Ctrl+L left it
		cs.browse, cs.hidBrowse = cs.hidBrowse, nil
		m.a.ClearStatus()
		return
	}
	l, ok := m.a.LogLayout(cs.Ch)
	cs.browse = newBrowse(cs, l, ok)
	cs.browse.copy = m.copyCmd
	cs.browse.saveFile = m.d.SaveFile
	cs.browse.setExport(m.a.Config())
	m.a.ClearStatus()
}

// browseBodyH is the number of line rows in browse mode: the screen less
// the top bar and its rule, the action bar with its rules, and the bottom bar.
func (m *Model) browseBodyH() int { return max(1, m.height-topH-4) }

func (m *Model) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	l := m.layout()
	cs := m.cur()
	if cs != nil && cs.browse != nil && msg.X > l.sw {
		switch msg.Button {
		case tea.MouseWheelUp:
			return cs.browse.scrollBy(-m.scrollLines())
		case tea.MouseWheelDown:
			return cs.browse.scrollBy(m.scrollLines())
		}
		return nil
	}
	if msg.X < l.sw {
		if cs != nil && cs.browse != nil && cs.browse.panel != nil {
			switch msg.Button {
			case tea.MouseWheelUp:
				cs.browse.panelScroll(-3, m.height)
			case tea.MouseWheelDown:
				cs.browse.panelScroll(3, m.height)
			}
			return nil
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrollSidebar(-3)
		case tea.MouseWheelDown:
			m.scrollSidebar(3)
		}
		return nil
	}
	if m.overviewing() && msg.X > l.sw && msg.Y >= l.top && msg.Y-l.top < l.sbH {
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrollOverview(-m.scrollLines())
		case tea.MouseWheelDown:
			m.scrollOverview(m.scrollLines())
		}
		return nil
	}
	if cs == nil || msg.X <= l.sw || msg.Y < l.top || msg.Y-l.top >= l.sbH {
		return nil
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		cs.sb.ScrollUp(m.scrollLines())
	case tea.MouseWheelDown:
		cs.sb.ScrollDown(m.scrollLines())
	}
	return nil
}

// scrollLines is the scroll_lines setting: how far a wheel notch scrolls.
func (m *Model) scrollLines() int {
	if m.a.Config() != nil {
		return m.a.Config().ScrollLines
	}
	return config.DefaultScrollLines
}

// doubleClick is the longest gap between the clicks of a double-click.
const doubleClick = 400 * time.Millisecond

func (m *Model) handleClick(msg tea.MouseClickMsg, was app.Presence) tea.Cmd {
	if msg.Button != tea.MouseLeft {
		return nil
	}
	l := m.layout()
	if msg.X < l.sw {
		if cs := m.cur(); cs != nil && cs.browse != nil && cs.browse.panel != nil {
			cs.browse.panelClick(msg.X, msg.Y, m.height)
			return nil
		}
		if f := m.footerH(); f > 0 && msg.Y >= m.height-f {
			if msg.Y == m.height-f {
				return m.backupCmd()
			}
			return m.restoreCmd()
		}
		sv := m.sidebarView()
		r, hint := sv.at(msg.Y)
		switch {
		case hint != 0:
			m.scrollSidebar(hint * max(1, sv.avail-1))
		case r == nil:
		case m.listing(): // the rows are the picker's
			m.leaveEditor()
			if k := selKey(*r); k != "" {
				m.picker.sel = k
				return m.choose(k)
			}
		case r.kind == rowAdd:
			m.parkEditor()
			m.openPicker()
		case r.kind == rowWorld:
			m.switchTo(worldSel(r.world))
			m.sideShown = m.a.Active() // clicked, so already in view
		case r.kind == rowGap: // gaps do nothing
		case msg.X == badgeX && closable(m.chars[r.char]):
			m.close(r.char)
		default:
			m.switchTo(r.char)
			m.sideShown = r.char // clicked, so already in view
			now := m.d.Now()
			double := m.lastClick.char == r.char && now.Sub(m.lastClick.at) <= doubleClick
			m.lastClick.char, m.lastClick.at = r.char, now
			if cs := m.chars[r.char]; double && cs.State != session.Connected && cs.State != session.Connecting {
				m.lastClick.char = "" // a third click starts over
				return m.connect(cs)
			}
		}
		return nil
	}
	if msg.Y < l.top {
		if msg.Y == 0 {
			m.topClick(msg.X-l.sw-1, was)
		}
		return nil
	}
	msg.Y -= l.top // body rows from here on
	if m.overviewing() {
		if msg.Y < len(m.ovKeys) && m.ovKeys[msg.Y] != "" {
			m.switchTo(m.ovKeys[msg.Y])
		}
		return nil
	}
	if cs := m.cur(); cs != nil && cs.browse != nil {
		m.clearLogStatus()
		cs.browse.click(msg.X-l.sw-1, msg.Y, msg.Mod&tea.ModShift != 0)
		return nil
	}
	cs := m.cur()
	if cs != nil {
		cs.sb.ClearSelection()
	}
	m.input().ClearSelection()
	if cs != nil && cs.sb.Scrolled() && msg.Y == l.sbH-1 && msg.X >= m.width-l.pillW {
		cs.sb.ToBottom()
		return nil
	}
	if cs != nil && msg.Y < l.sbH && msg.X > l.sw {
		if p, ok := cs.sb.At(msg.Y, msg.X-l.sw-1); ok {
			cs.sb.StartSelect(p) // a drag selects; a click opens a link
		}
		return nil
	}
	if y := msg.Y - l.sbH - 1; !l.prompt && y >= 0 && y < len(l.inRows) && msg.X > l.sw {
		m.input().StartSelect(msg.X-l.sw-1-gutterWidth, l.inTop+y) // past the separator and gutter
	}
	return nil
}

// handleDrag follows a drag that started in the scrollback or the input,
// clamping the pointer to where it started. With no button down, it
// tracks what the pointer is over, so links light up.
func (m *Model) handleDrag(msg tea.Mouse) {
	l := m.layout()
	msg.Y -= l.top // body rows
	x := max(0, msg.X-l.sw-1)
	if cs := m.cur(); cs != nil && msg.Button == tea.MouseNone {
		cs.sb.Hover(nil)
		if msg.X > l.sw && msg.Y < l.sbH && cs.browse == nil {
			if p, ok := cs.sb.At(msg.Y, x); ok {
				cs.sb.Hover(&p)
			}
		}
		return
	}
	if cs := m.cur(); cs != nil && cs.sb.Dragging() {
		for y := min(max(0, msg.Y), l.sbH-1); y >= 0; y-- { // the nearest text row at or above
			if p, ok := cs.sb.At(y, x); ok {
				cs.sb.DragTo(p)
				return
			}
		}
		return
	}
	if in := m.input(); in.Dragging() && len(l.inRows) > 0 {
		y := min(max(0, msg.Y-l.sbH-1), len(l.inRows)-1)
		in.DragTo(max(0, x-gutterWidth), l.inTop+y)
	}
}

// openLink opens u, if it's a link and links can be opened. Over ssh an
// opener would run on the wrong machine, and where it fails, the link
// goes to the clipboard instead, which reaches your screen either way.
func (m *Model) openLink(u string) tea.Cmd {
	if u == "" || m.d.OpenURL == nil {
		return nil
	}
	if m.d.Remote {
		m.setStatus(false, str.StatusLinkCopied(u))
		return m.copyCmd(u)
	}
	if err := m.d.OpenURL(u); err != nil {
		m.setStatus(true, str.StatusLinkFailed(err))
		return m.copyCmd(u)
	}
	m.setStatus(false, str.StatusLinkOpened(u))
	return nil
}

// copyCmd puts text on the clipboard of the terminal Kiln draws on (OSC
// 52). tmux only passes it on when it's told to, so there it's sent
// twice: plain, for set-clipboard on, and wrapped, for allow-passthrough.
func (m *Model) copyCmd(text string) tea.Cmd {
	if !m.d.Tmux {
		return tea.SetClipboard(text)
	}
	return tea.Batch(tea.SetClipboard(text), m.d.Raw(notify.Clipboard(text, true)))
}

// handleRelease ends a drag: a selection is copied to the clipboard, and
// a click on a link in the scrollback opens it.
func (m *Model) handleRelease() tea.Cmd {
	text := ""
	if cs := m.cur(); cs != nil && cs.sb.Dragging() {
		var click sbPos
		var clicked bool
		text, click, clicked = cs.sb.EndSelect()
		if clicked {
			return m.openLink(cs.sb.URLAt(click))
		}
	} else if in := m.input(); in.Dragging() {
		text = in.EndSelect()
	}
	if text == "" {
		return nil
	}
	m.setStatus(false, str.StatusCopied())
	return m.copyCmd(text)
}
