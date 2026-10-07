// Package ui is Kiln's terminal interface: a sidebar of worlds and
// characters, and a right pane with scrollback, input box and statusline.
package ui

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/notify"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/style"
	"github.com/latrani/Kiln/internal/theme"
)

// HistoryLines is how many logged lines are preloaded per character.
const HistoryLines = 200

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

type mode int

const (
	modeNormal mode = iota
	modeSavePassword
)

// Model is the Bubble Tea model.
type Model struct {
	d           Deps
	cfg         *config.Config // the loaded config; the picker lists from it
	picker      *picker        // non-nil while the open-connection picker is open
	idle        *Input         // the input box while nothing is open
	chars       map[string]*charState
	order       []string // sidebar order of character keys
	active      string
	width       int
	height      int
	status      string
	statusErr   bool
	mode        mode
	pendingPW   string                 // entered password awaiting the save y/n answer
	pendingCh   [2]string              // world and character id the pending password belongs to
	confirm     bool                   // next Enter sends an over-limit line anyway
	resizeGen   int                    // bumped per WindowSizeMsg; see resizeMsg
	sideTop     int                    // first sidebar row shown when it overflows
	sideShown   string                 // active character last scrolled into view
	pwStore     atomic.Pointer[string] // password_store; sessions read it off the UI goroutine
	recent      []string               // open characters by when last active, most recent first; not the active one
	quitKey     string                 // "ctrl+c" or "ctrl+d" once pressed on an empty input; again quits
	quitGen     int                    // bumped per arming; see quitExpiredMsg
	statusGen   int                    // bumped by each setStatus; see statusExpiredMsg
	statusOfLog bool                   // status came from log mode; its next key or click clears it
	statusTimed int                    // the statusGen whose expiry is scheduled
	lastClick   struct {               // for spotting a double-click in the sidebar
		char string
		at   time.Time
	}
	focused         bool                    // the terminal has focus, as far as we know
	focusSeen       bool                    // the terminal has sent a focus-in or focus-out, so it reports focus
	lastHere        time.Time               // latest focus-in or input; see here
	awayNow         bool                    // set by /away until the next input; see away
	shownPresence   presenceState           // what the presence chip shows; see Update
	themed          bool                    // a theme has been loaded; see loadTheme
	standIn         bool                    // the theme is the built-in standing in for a broken one; see loadTheme
	themeErr        error                   // why the theme didn't load, to report once the update is done
	detected        theme.Appearance        // the terminal's last answer about its background; Dark until one comes
	hereGen         int                     // bumped by each here; re-arms "first"
	notifyOverrides map[string]notify.Level // from /notify, by character key, until Kiln quits
	ovTop           int                     // first row of a world's overview shown; see overview
	ovKeys          []string                // the character each overview row shows, from the last draw; "" for a rule
	drafts          map[string]*editor      // editors hidden by Ctrl+T, unsaved, by target; see hideEditor
	parked          map[string]*editor      // editors left open on a sidebar item while another is active, by m.active; see parkEditor
}

type charState struct {
	key         string
	ch          config.Character
	sess        *session.Session
	cancel      context.CancelFunc
	state       session.State
	cls         *classify.Classifier
	hl          *rules.Highlighter
	sb          Scrollback
	in          *Input
	unread      int
	attention   bool
	pin         *conn.PinMismatchError
	needPW      bool
	pwDraft     string           // input stashed while the password prompt is up
	orphan      bool             // removed from the config; dropped when it disconnects
	browse      *browse          // non-nil while browse mode is open
	hidBrowse   *browse          // browse mode as Ctrl+L left it, kept up to date; the next Ctrl+L brings it back
	filter      scene.Filter     // log mode's filter; outlasts a log-mode session
	collapsed   map[string]bool  // filter panel parents folded shut, by tag
	foldSeen    map[string]bool  // parents the panel has already met; a new one starts folded
	hist        *history.Reader  // pages older log days into sb; only an in-flight sbOlderMsg read touches it
	leftover    []logstore.Entry // the preload's unshown start of its oldest day
	sentGen     int              // hereGen when the last notification went out; -1: none yet
	connectedAt time.Time        // when the current connection came up
	lastSent    time.Time        // when the last notification went out
}

// sbOlderMsg carries older scrollback lines, read and rendered off the UI
// goroutine by pageOlder.
type sbOlderMsg struct {
	key   string
	hist  *history.Reader // the reader that was asked; stale if it has changed
	theme *theme.Theme    // the theme active when the read started
	lines []sbLine
	more  bool
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

// startPassword shows the masked password prompt, stashing any draft so
// it neither becomes part of the password nor is lost.
func (cs *charState) startPassword() {
	if cs.needPW {
		return
	}
	cs.needPW = true
	cs.pwDraft = cs.in.Value()
	cs.in.Reset()
}

// endPassword leaves the password prompt (submitted, skipped, or the
// connection dropped), clearing anything typed and restoring the draft.
func (cs *charState) endPassword() {
	cs.needPW = false
	cs.in.Reset()
	cs.in.SetValue(cs.pwDraft)
	cs.pwDraft = ""
}

func key(world, char string) string { return world + "/" + char }

// Messages.
type (
	eventMsg struct {
		key  string
		sess *session.Session
		ev   session.Event
		ok   bool
	}
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
	m.focused, m.lastHere = true, d.Now()
	m.notifyOverrides = map[string]notify.Level{}
	m.applyConfig(cfg)
	m.shownPresence = m.presence()
	m.loadTheme() // at start a broken theme falls back to the built-in, and says so
	if m.themeErr != nil {
		m.setStatus(true, str.StatusThemeNotLoaded(m.themeErr))
		m.themeErr = nil
	}
	for _, ch := range m.allChars() {
		if ch.Autoconnect && !d.NoAutoconnect {
			m.open(key(ch.World, ch.ID))
		}
	}
	return m
}

// Init connects autoconnect characters and starts the clock and watcher.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(m.d.Now()), m.watch(), m.askBackground()}
	for _, k := range m.order {
		if m.chars[k].ch.Autoconnect && !m.d.NoAutoconnect {
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

func waitEvent(k string, s *session.Session) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-s.Events()
		return eventMsg{key: k, sess: s, ev: ev, ok: ok}
	}
}

// reloadNow loads the config and theme and applies them, reporting whether
// the config could be.
func (m *Model) reloadNow() bool {
	cfg, err := m.d.Load(m.d.ConfigDir)
	if err != nil {
		m.setStatus(true, str.StatusConfigNotReloaded(err))
		return false
	}
	m.applyConfig(cfg)
	m.loadTheme() // a broken theme doesn't stop the config; see themeErr
	return true
}

// appearance is the palette to draw with: the setting, or with auto the
// terminal's last answer (dark until one comes).
func (m *Model) appearance() theme.Appearance {
	switch m.cfg.Appearance {
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
	if m.cfg.Appearance != "auto" {
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
	th, err := theme.Load(m.d.ConfigDir, m.cfg.Theme, m.appearance())
	m.themeErr = err
	if err != nil && m.themed && !m.standIn {
		return // keep the theme we have
	}
	m.themed, m.standIn = true, err != nil // a stand-in built-in still follows the appearance
	if th == theme.Active() || th.Equal(theme.Active()) {
		return // restyling would drop a selection for nothing
	}
	theme.SetActive(th)
	for _, k := range m.order { // in order, so which error shows is settled
		cs := m.chars[k]
		if _, err := cs.compile(); err != nil { // each highlighter holds the theme it was built on
			m.setStatus(true, str.StatusCharError(k, err))
		}
		render := func(e logstore.Entry) string { text, _ := cs.render(e); return text }
		cs.sb.Rerender(render)
		for _, b := range cs.browses() {
			b.restyle(render)
		}
	}
}

// applyConfig keeps cfg for the picker and updates open characters to
// match it. An open character that vanished from the config stays as an
// orphan while it's connected, until it next disconnects; otherwise it
// closes.
func (m *Model) applyConfig(cfg *config.Config) {
	m.cfg = cfg
	store := cfg.PasswordStore
	m.pwStore.Store(&store)
	for _, k := range slices.Clone(m.order) {
		cs := m.chars[k]
		for _, b := range cs.browses() {
			b.setExport(cfg)
		}
		ch, ok := m.find(k)
		if !ok {
			if cs.sess != nil && cs.state != session.Disconnected && cs.state != session.Failed {
				cs.orphan = true
			} else {
				m.close(k)
			}
			continue
		}
		restyle := !reflect.DeepEqual(styleInputs(cs.ch), styleInputs(ch))
		cs.ch, cs.orphan = ch, false
		installed, err := cs.compile()
		if err != nil {
			m.setStatus(true, str.StatusCharError(k, err))
		}
		if installed && restyle {
			cs.sb.Rerender(func(e logstore.Entry) string { text, _ := cs.render(e); return text })
		}
		if cs.sess != nil {
			cs.sess.SetChar(ch)
		}
	}
	m.sortOrder() // names may have changed
	if m.picker != nil {
		m.fixPick()
	}
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

// compile builds cs's classifier, and its highlighter from the active
// theme with the character's own looks on top, reporting whether it
// installed them. Looks that don't resolve (a color nobody defines)
// leave the theme's own tag styles in use; either way the error is
// returned for the caller to show.
func (cs *charState) compile() (installed bool, err error) {
	cls, err := classify.New(cs.ch.Rules.Classify, cs.ch.Name, cs.ch.Aliases)
	if err != nil {
		return false, err
	}
	th, lookErr := theme.Active().With(cs.ch.Looks...)
	if lookErr != nil {
		th = theme.Active()
	}
	cs.cls, cs.hl = cls, rules.New(th, cs.ch.Rules.Attention, cs.ch.Rules.Quiet)
	return true, lookErr
}

// logLayout is where ch's logs go; ok is false when there's nowhere.
func (m *Model) logLayout(ch config.Character) (l logstore.Layout, ok bool) {
	l = logstore.Layout{Root: m.d.LogRoot, Dir: m.cfg.LogDir, Name: m.cfg.LogName,
		World: ch.World, Char: ch.ID, CharName: ch.Name}
	return l, m.d.LogRoot != "" || filepath.IsAbs(m.cfg.LogDir)
}

// mustLayout is logLayout for the log writer, which always has a place
// to write in the real program (LogRoot is set).
func (m *Model) mustLayout(ch config.Character) logstore.Layout {
	l, _ := m.logLayout(ch)
	return l
}

// preload fills the scrollback with the tail of the most recent log days
// and hands everything older to the scrollback to page in on demand, so
// scrollback is unlimited.
func (m *Model) preload(cs *charState) {
	l, ok := m.logLayout(cs.ch)
	if !ok {
		return
	}
	hist, err := history.NewReader(l)
	if err != nil {
		return
	}
	var entries []logstore.Entry
	for len(entries) < HistoryLines {
		es, _, ok, err := hist.LoadOlder()
		if !ok || err != nil {
			break
		}
		entries = append(es, entries...)
	}
	if len(entries) == 0 {
		return
	}
	// entries holds whole days. Keep the newest HistoryLines on screen; the
	// rest (the start of the oldest day, plus any fuller days before it)
	// is the first batch paged in when scrolling up.
	var leftover []logstore.Entry
	if len(entries) > HistoryLines {
		leftover = entries[:len(entries)-HistoryLines]
		entries = entries[len(entries)-HistoryLines:]
	}
	// The preload starts a day only if nothing of that day was left over.
	startsDay := len(leftover) == 0 || leftover[len(leftover)-1].Time.Local().Format("2006-01-02") != entries[0].Time.Local().Format("2006-01-02")
	for _, l := range cs.renderDays(entries, startsDay) {
		cs.sb.AppendLine(l)
	}
	cs.sb.AppendLine(chromeLine(theme.ScrollbackHistoryEnd, str.ScrollbackHistoryEnds(entries[len(entries)-1].Time.Format(str.DateDayTime()))))
	cs.hist, cs.leftover = hist, leftover
	cs.sb.SetMore(leftover != nil || !hist.Exhausted())
}

// pageOlder starts reading the next older batch into the current
// character's scrollback in a tea.Cmd, when its view has scrolled past
// the oldest line. The first batch is the preload's leftover; after that,
// one log day per read.
func (m *Model) pageOlder() tea.Cmd {
	cs := m.cur()
	if cs == nil || cs.browse != nil || cs.hist == nil || !cs.sb.RequestOlder(m.layout().sbH) {
		return nil
	}
	key, h, cls, hl, echo, leftover := cs.key, cs.hist, cs.cls, cs.hl, cs.ch.LocalEcho, cs.leftover
	cs.leftover = nil
	th := theme.Active()
	return func() tea.Msg {
		msg := sbOlderMsg{key: key, hist: h, theme: th}
		if leftover != nil {
			msg.lines, msg.more = renderDays(cls, hl, echo, leftover, true), !h.Exhausted()
			return msg
		}
		if es, _, ok, err := h.LoadOlder(); ok && err == nil {
			msg.lines, msg.more = renderDays(cls, hl, echo, es, true), !h.Exhausted()
		}
		return msg
	}
}

// renderDays renders log entries with a dim divider before the first line
// of each day. The very first entry gets one only if startsDay, i.e. it
// really is the first line of its day. Sent lines are left out unless the
// character has local_echo on.
func (cs *charState) renderDays(entries []logstore.Entry, startsDay bool) []sbLine {
	return renderDays(cs.cls, cs.hl, cs.ch.LocalEcho, entries, startsDay)
}

// renderDays is charState.renderDays with the given rules; like
// renderLine it is safe off the UI goroutine.
func renderDays(cls *classify.Classifier, hl *rules.Highlighter, echo bool, entries []logstore.Entry, startsDay bool) []sbLine {
	out := make([]sbLine, 0, len(entries)+2)
	prev := ""
	for i, e := range entries {
		if e.Dir == logstore.Out && !echo {
			continue
		}
		day := e.Time.Local().Format("2006-01-02")
		if day != prev && (i > 0 || startsDay) {
			out = append(out, chromeLine(theme.ScrollbackDay, "── "+dayLabel(day)+" ──"))
		}
		prev = day
		text, _ := renderLine(cls, hl, e)
		out = append(out, entryLine(text, e))
	}
	return out
}

// echoes reports whether e belongs in the scrollback: everything but sent
// lines, which only show with local_echo on. They're logged either way.
func (cs *charState) echoes(e logstore.Entry) bool {
	return e.Dir != logstore.Out || cs.ch.LocalEcho
}

// render turns a log entry into a drawable line, with what the highlight
// rules made of it (attention, quiet).
func (cs *charState) render(e logstore.Entry) (string, rules.Result) {
	return renderLine(cs.cls, cs.hl, e)
}

// renderLine styles e with the given rules. It only reads cls and hl
// (compiled once, never mutated), so it is safe off the UI goroutine.
func renderLine(cls *classify.Classifier, hl *rules.Highlighter, e logstore.Entry) (string, rules.Result) {
	text := ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return theme.Paint(theme.ScrollbackEcho, gutterMark+text), rules.Result{}
	case logstore.Sys:
		return theme.Paint(theme.ScrollbackSys, "* "+text), rules.Result{}
	}
	plain := ansi.Strip(text)
	res := hl.Apply(plain, cls.Tags(plain))
	return style.Highlight(text, res.Runs), res
}

// connect starts (or restarts) a character's session.
func (m *Model) connect(cs *charState) tea.Cmd {
	if cs.sess != nil {
		cs.sess.Reconnect()
		return nil
	}
	var s *session.Session
	s = session.New(session.Options{
		Char: cs.ch,
		Log:  m.d.NewLog(m.mustLayout(cs.ch)),
		Dial: func(ctx context.Context) (session.LineConn, error) { return m.d.Dial(ctx, s.Char()) },
		Password: func() (string, error) {
			ch := s.Char()
			return m.d.Password(m.passwordStore(), ch.World, ch.ID)
		},
	})
	if w, h := m.paneSize(); w > 0 {
		s.Resize(w, h)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cs.sess, cs.cancel = s, cancel
	cs.state = session.Connecting // until the session says otherwise; no × flash
	go s.Run(ctx)
	return waitEvent(cs.key, s)
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

// reportSize tells every session the pane size. Sessions only send it to
// servers that negotiated NAWS.
func (m *Model) reportSize() {
	w, h := m.paneSize()
	for _, cs := range m.chars {
		if cs.sess != nil {
			cs.sess.Resize(w, h)
		}
	}
}

// Update handles one message, then pages in older scrollback if the
// view has run past it.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	if _, blur := msg.(tea.BlurMsg); !blur {
		// Switching away leaves the chip be: nobody's looking, and in tmux
		// redrawing it would flag the window as active every time you left.
		// It catches up with whatever next changes.
		m.shownPresence = m.presence()
	}
	m.takeLogStatus()
	if older := m.pageOlder(); older != nil {
		cmd = tea.Batch(cmd, older)
	}
	if m.themeErr != nil { // last, so it isn't covered by what the update said
		m.setStatus(true, str.StatusThemeNotLoaded(m.themeErr))
		m.themeErr = nil
	}
	if m.statusGen != m.statusTimed { // a new status: time it out
		m.statusTimed = m.statusGen
		gen := m.statusGen
		cmd = tea.Batch(cmd, tea.Tick(statusTimeout, func(time.Time) tea.Msg { return statusExpiredMsg(gen) }))
	}
	return m, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeGen++
		gen := m.resizeGen
		return m, tea.Tick(resizeDebounce, func(time.Time) tea.Msg { return resizeMsg(gen) })
	case resizeMsg:
		if int(msg) == m.resizeGen { // the drag has settled
			m.reportSize()
		}
	case StatusMsg:
		m.setStatus(msg.Err, msg.Text)
	case backupDoneMsg:
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
		if int(msg) == m.statusGen {
			m.status = ""
		}
	case reloadMsg:
		if m.reloadNow() {
			m.setStatus(false, str.StatusConfigReloaded())
		}
		return m, tea.Batch(m.watch(), m.askBackground())
	case eventMsg:
		return m, m.handleEvent(msg)
	case tea.FocusMsg:
		m.focused, m.focusSeen = true, true
		m.here()
		return m, m.askBackground() // the terminal may have gone light or dark meanwhile
	case tea.BackgroundColorMsg:
		ap := theme.Light
		if msg.IsDark() {
			ap = theme.Dark
		}
		if ap != m.detected {
			m.detected = ap
			if m.cfg.Appearance == "auto" {
				m.loadTheme()
			}
		}
	case tea.BlurMsg:
		m.focused, m.focusSeen = false, true
	case sbOlderMsg:
		if cs := m.chars[msg.key]; cs != nil && cs.hist == msg.hist {
			if msg.theme != theme.Active() { // rendered in a theme since replaced
				render := func(e logstore.Entry) string { text, _ := cs.render(e); return text }
				for i, l := range msg.lines {
					msg.lines[i] = l.restyled(render)
				}
			}
			cs.sb.PrependLines(msg.lines, msg.more)
		}
	case olderMsg:
		if cs := m.chars[msg.key]; cs != nil && slices.Contains(cs.browses(), msg.b) {
			return m, msg.b.receive(msg)
		}
	case tea.PasteMsg:
		m.focused = true // only a focused window gets input, even if its focus-in was lost
		m.here()
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
		} else if m.mode == modeNormal && !m.overviewing() {
			m.input().InsertText(msg.Content)
			m.confirm = false
		}
	case tea.KeyPressMsg:
		m.focused = true
		m.here()
		before, gen := m.input().Value(), m.statusGen
		cmd := m.handleKey(msg)
		if m.statusGen == gen && m.input().Value() != before {
			m.status = "" // typing again dismisses what was said before
		}
		return m, cmd
	case tea.MouseWheelMsg:
		if m.focused { // macOS scrolls windows in the background
			m.here()
		}
		return m, m.handleWheel(msg)
	case tea.MouseClickMsg:
		was := m.shownPresence // as drawn, before here() clears an Away that a click on the chip toggles
		m.focused = true
		m.here()
		return m, m.handleClick(msg, was)
	case tea.MouseMotionMsg:
		m.handleDrag(msg.Mouse())
	case tea.MouseReleaseMsg:
		return m, m.handleRelease()
	}
	return m, nil
}

func (m *Model) cur() *charState { return m.chars[m.active] }

// input is the active character's input box, or the idle one.
func (m *Model) input() *Input {
	if cs := m.cur(); cs != nil {
		return cs.in
	}
	return m.idle
}

// passwordStore is the password_store setting.
func (m *Model) passwordStore() string {
	if p := m.pwStore.Load(); p != nil && *p != "" {
		return *p
	}
	return config.DefaultPasswordStore
}

// statusTimeout is how long a status message stays up. Typing clears it
// sooner.
const statusTimeout = 30 * time.Second

func (m *Model) setStatus(isErr bool, msg string) {
	m.status, m.statusErr = msg, isErr
	m.statusGen++
	m.statusOfLog = false
}

// takeLogStatus moves a message log mode just set into the bottom bar's
// status, so it times out like any other and the newest message wins.
func (m *Model) takeLogStatus() {
	if cs := m.cur(); cs != nil && cs.browse != nil && cs.browse.status != "" {
		m.setStatus(cs.browse.statusErr, cs.browse.status)
		m.statusOfLog = true
		cs.browse.status = ""
	}
}

// clearLogStatus drops a message from log mode when you act in it again,
// as log mode always has.
func (m *Model) clearLogStatus() {
	if m.statusOfLog {
		m.status, m.statusOfLog = "", false
	}
}

func (m *Model) handleEvent(msg eventMsg) tea.Cmd {
	cs, ok := m.chars[msg.key]
	if !ok || cs.sess != msg.sess || !msg.ok {
		return nil // stale session, or it has shut down
	}
	ev := msg.ev
	switch ev.Kind {
	case session.EventLine:
		text, res := cs.render(ev.Entry)
		switch {
		case !cs.echoes(ev.Entry):
		case res.Quiet:
			cs.sb.append(entryLine(text, ev.Entry))
		default:
			cs.sb.AppendLine(entryLine(text, ev.Entry))
		}
		if msg.key == m.active {
			l := m.layout()
			cs.sb.SetWidth(l.rw) // measure at the pane's width, even before a View
			cs.sb.Pause(l.sbH)
		}
		for _, b := range cs.browses() {
			b.appendLive(ev.Entry)
		}
		if msg.key != m.active && ev.Entry.Dir == logstore.In && !res.Quiet {
			cs.unread++
			cs.attention = cs.attention || res.Attention
		}
		if n := m.notifyCmd(cs, ev.Entry, res); n != nil {
			return tea.Batch(n, waitEvent(msg.key, msg.sess))
		}
	case session.EventPrompt:
		cs.sb.SetPrompt(ansi.Sanitize(ev.Entry.Text))
	case session.EventState:
		cs.state = ev.State
		if ev.State == session.Connected {
			cs.connectedAt = m.d.Now()
		}
		if cs.orphan && (ev.State == session.Disconnected || ev.State == session.Failed) {
			m.close(msg.key) // don't reconnect (and log in) a deleted character
			return nil
		}
		var pin *conn.PinMismatchError
		if ev.State == session.Failed && errors.As(ev.Err, &pin) {
			cs.pin = pin
			m.setStatus(true, str.StatusCertChanged(cs.ch.Name))
		}
		if ev.State == session.Connected {
			cs.pin = nil
		}
		if ev.State != session.Connected && cs.needPW {
			cs.endPassword()
		}
	case session.EventLogError:
		m.setStatus(true, str.StatusLogWriteFailed(cs.ch.Name, ev.Err))
	case session.EventNeedPassword:
		cs.startPassword()
	}
	return waitEvent(msg.key, msg.sess)
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
	if m.mode == modeSavePassword {
		switch k.String() {
		case "enter", "y", "Y":
			// Save for the character that was asked about, even if another
			// one is active now.
			store := m.passwordStore()
			switch err := m.d.SavePassword(store, m.pendingCh[0], m.pendingCh[1], m.pendingPW); {
			case err != nil && store == "keychain":
				m.setStatus(true, str.StatusKeychainFailed(err))
			case err != nil:
				m.setStatus(true, str.StatusPasswordNotSavedErr(err))
			default:
				m.setStatus(false, str.StatusPasswordSaved())
			}
		case "n", "N", "esc", "ctrl+c":
			m.setStatus(false, str.StatusPasswordNotSaved())
		case openPickerKey:
			m.openPicker() // says why not
			return nil
		default:
			return nil
		}
		m.mode, m.pendingPW, m.pendingCh = modeNormal, "", [2]string{}
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
		} else if w, ok := m.activeWorld(); ok {
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
		m.confirm = false
		if cs != nil && cs.needPW {
			cs.endPassword()
			m.setStatus(false, str.StatusSkippedLogin())
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
	m.confirm = false
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
	if m.quitKey != "" && m.status == quitHint(m.quitKey) {
		m.status = ""
	}
	m.quitKey = ""
}

func (m *Model) quit() tea.Cmd {
	for _, cs := range m.chars {
		if cs.cancel != nil {
			cs.cancel()
		}
	}
	return tea.Quit
}

// switchBy moves through the sidebar's worlds and characters.
func (m *Model) switchBy(delta int) {
	stops := m.stops()
	if len(stops) == 0 {
		return
	}
	i := max(0, slices.Index(stops, m.active))
	i = (i + delta + len(stops)) % len(stops)
	m.switchTo(stops[i])
}

// switchToUnread moves to the next character (dir 1) or previous one
// (dir -1) in sidebar order that has unseen lines, wrapping around. With
// nothing unread it goes back to the character active before this one,
// so Tab flips between the last two, like switching apps.
func (m *Model) switchToUnread(dir int) {
	n := len(m.order)
	i := slices.Index(m.order, m.active) // -1 when nothing is open
	for step := 1; step <= n; step++ {
		k := m.order[((i+dir*step)%n+n)%n]
		if k != m.active && m.chars[k].unread > 0 {
			m.switchTo(k)
			return
		}
	}
	if len(m.recent) > 0 {
		m.switchTo(m.recent[0])
	} else if n > 1 { // others are open but none has been active yet
		m.switchTo(m.order[((i+dir)%n+n)%n])
	}
}

// switchTo makes k active: a character's key, or a world's selection key
// (worldSel) for its overview. Tab's history holds only characters.
func (m *Model) switchTo(k string) {
	cs, ok := m.chars[k]
	w, isWorld := strings.CutPrefix(k, worldSel(""))
	if !ok && (!isWorld || !m.worldOpen(w)) {
		return
	}
	if m.active != k {
		m.parkEditor()
		defer m.unparkEditor()
	}
	if old := m.cur(); old != nil && m.active != k {
		old.sb.MarkSeen() // you saw it up to now
		m.recent = slices.DeleteFunc(m.recent, func(r string) bool { return r == m.active })
		m.recent = append([]string{m.active}, m.recent...)
	}
	if m.active != k {
		m.ovTop = 0 // an overview starts at its top
	}
	m.active, m.confirm = k, false
	if !ok {
		return
	}
	m.recent = slices.DeleteFunc(m.recent, func(r string) bool { return r == k })
	l := m.layout()
	cs.sb.SetWidth(l.rw)
	cs.sb.Pause(l.sbH) // open at the first line you haven't seen
	if cs.browse != nil && m.picker != nil {
		m.leaveEditor()
		m.closePicker() // browse has the pane; the filter would be hidden
	}
	cs.unread, cs.attention = 0, false
}

// submit handles Enter: a command, a password, or lines for the server.
func (m *Model) submit() tea.Cmd {
	if m.overviewing() {
		m.openPicker() // as on an empty input with nothing open
		return nil
	}
	cs := m.cur()
	if cs == nil {
		switch text := m.idle.Value(); {
		case text == "":
			m.openPicker()
		case strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//"):
			m.idle.Commit()
			return m.command(nil, text)
		default:
			m.setStatus(true, str.StatusNothingOpen())
		}
		return nil
	}
	if cs.needPW {
		pw := cs.in.CommitSecret()
		e, err := cs.sess.Login(pw)
		if err != nil && !isLogErr(err) {
			m.setStatus(true, str.StatusLoginNotSent(err))
			return nil
		}
		cs.endPassword()
		if cs.echoes(e) {
			cs.sb.AppendLine(chromeLine(theme.ScrollbackEcho, gutterMark+e.Text))
		}
		if m.d.SavePassword != nil && pw != "" && m.passwordStore() != "none" {
			m.mode, m.pendingPW = modeSavePassword, pw
			m.pendingCh = [2]string{cs.ch.World, cs.ch.ID}
		}
		return nil
	}
	m.status = ""
	if cs.in.Empty() && cs.state != session.Connected {
		if cs.state == session.Connecting || cs.pin != nil {
			return nil // nothing Enter can do; the prompt says why
		}
		return m.connect(cs)
	}
	text := cs.in.Value()
	if strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//") {
		cs.in.Commit()
		return m.command(cs, text)
	}
	text = strings.TrimPrefix(text, "/") // "//foo" sends "/foo"
	if cs.sess == nil || cs.state != session.Connected {
		m.setStatus(true, str.StatusNotConnected(cs.ch.Name))
		return nil
	}
	if cs.in.OverLimit(cs.ch.MaxLineBytes, cs.ch.NewlineMode == "flatten") && !m.confirm {
		m.confirm = true
		m.setStatus(true, str.StatusOverLimit(cs.ch.MaxLineBytes))
		return nil
	}
	m.confirm = false
	lines := strings.Split(text, "\n")
	if cs.ch.NewlineMode == "flatten" {
		lines = []string{strings.Join(lines, " ")}
	}
	secret, echoed := false, false
	for _, line := range lines {
		e, err := cs.sess.Send(line)
		if err != nil && !isLogErr(err) {
			m.setStatus(true, str.StatusNotSent(err))
			break
		}
		if err != nil {
			m.setStatus(true, err.Error())
		}
		secret = secret || e.Text != line // the session redacted a typed password
		echoed = cs.echoes(e)
		if echoed {
			cs.sb.AppendLine(chromeLine(theme.ScrollbackEcho, gutterMark+ansi.Sanitize(e.Text)))
		}
	}
	if secret {
		cs.in.CommitSecret()
	} else {
		cs.in.Commit()
	}
	cs.sb.ToBottomKeeping(echoed) // what you send is where the pager picks up
	return nil
}

func isLogErr(err error) bool {
	var le *session.LogError
	return errors.As(err, &le)
}

// command runs a slash command. text is the whole input line, so commands
// that take free text (/highlight) can keep its spacing.
func (m *Model) command(cs *charState, text string) tea.Cmd {
	args := strings.Fields(text)
	if (args[0] == "/backup" && m.d.Backup == nil) || (args[0] == "/restore" && m.d.Restore == nil) {
		m.setStatus(true, str.StatusUnknownCommand(args[0])) // web-only, so unknown here
		return nil
	}
	if args[0] == "/away" {
		m.awayNow = true
		m.setStatus(false, str.StatusAway())
		return nil
	}
	if cs == nil && args[0] != "/quit" && args[0] != "/open" && args[0] != "/backup" && args[0] != "/restore" {
		m.setStatus(true, str.StatusNeedsCharacter(args[0]))
		return nil
	}
	switch args[0] {
	case "/connect":
		if cs.sess != nil && cs.state == session.Connected {
			m.setStatus(false, str.StatusAlreadyConnected(cs.ch.Name))
			return nil
		}
		return m.connect(cs)
	case "/reconnect":
		return m.connect(cs)
	case "/disconnect":
		if cs.sess != nil {
			cs.sess.Disconnect()
		}
	case "/trust":
		if cs.pin == nil {
			m.setStatus(true, str.StatusNoChangedCert())
			return nil
		}
		if err := m.d.KnownHosts.Trust(cs.pin.HostPort, cs.pin.Got); err != nil {
			m.setStatus(true, str.StatusTrustFailed(err))
			return nil
		}
		m.setStatus(false, str.StatusTrusted(cs.pin.HostPort))
		cs.pin = nil
		return m.connect(cs)
	case "/close":
		m.close(cs.key)
	case "/quit":
		return m.quit()
	case "/open":
		m.openPicker() // says why not
	case "/log":
		m.openBrowse(cs)
	case "/edit":
		m.editCommand(cs, strings.Join(args[1:], " "))
	case "/highlight":
		text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), args[0]))
		if err := config.AppendHighlight(m.d.ConfigDir, cs.ch.World, text); err != nil {
			m.setStatus(true, str.StatusHighlightFailed(err))
			return nil
		}
		m.setStatus(false, str.StatusHighlightAdded(text))
	case "/notify":
		m.notifyCommand(cs, args[1:])
	case "/backup":
		return m.backupCmd()
	case "/restore":
		return m.restoreCmd()
	default:
		m.setStatus(true, str.StatusUnknownCommand(args[0]))
	}
	return nil
}

// backupCmd runs the Backup hook off the UI goroutine.
func (m *Model) backupCmd() tea.Cmd {
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
		m.status = ""
		return
	}
	l, ok := m.logLayout(cs.ch)
	cs.browse = newBrowse(cs, l, ok)
	cs.browse.copy = m.copyCmd
	cs.browse.saveFile = m.d.SaveFile
	cs.browse.setExport(m.cfg)
	m.status = ""
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
	if m.cfg != nil {
		return m.cfg.ScrollLines
	}
	return config.DefaultScrollLines
}

// doubleClick is the longest gap between the clicks of a double-click.
const doubleClick = 400 * time.Millisecond

func (m *Model) handleClick(msg tea.MouseClickMsg, was presenceState) tea.Cmd {
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
			m.sideShown = m.active // clicked, so already in view
		case r.kind == rowGap: // gaps do nothing
		case msg.X == badgeX && closable(m.chars[r.char]):
			m.close(r.char)
		default:
			m.switchTo(r.char)
			m.sideShown = r.char // clicked, so already in view
			now := m.d.Now()
			double := m.lastClick.char == r.char && now.Sub(m.lastClick.at) <= doubleClick
			m.lastClick.char, m.lastClick.at = r.char, now
			if cs := m.chars[r.char]; double && cs.state != session.Connected && cs.state != session.Connecting {
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
