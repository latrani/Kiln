// Package ui is Kiln's terminal interface: a sidebar of worlds and
// characters, and a right pane with scrollback, input box and statusline.
package ui

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/style"
)

// HistoryLines is how many logged lines are preloaded per character.
const HistoryLines = 200

// Deps are the UI's connections to the outside world. Tests substitute
// fakes; cmd/kiln wires the real ones.
type Deps struct {
	ConfigDir  string
	LogRoot    string
	KnownHosts conn.KnownHosts
	Load       func(dir string) (*config.Config, error)
	Dial       func(ctx context.Context, ch config.Character) (session.LineConn, error)
	NewLog     func(world, char string) session.Appender
	// Password and SavePassword use the password_store setting in store.
	Password     func(store, world, char string) (string, error)
	SavePassword func(store, world, char, password string) error // nil: never offer
	Changes      <-chan struct{}                                 // config changes; nil: no hot reload
	Now          func() time.Time
}

type mode int

const (
	modeNormal mode = iota
	modeSavePassword
)

// Model is the Bubble Tea model.
type Model struct {
	d         Deps
	cfg       *config.Config // the loaded config; the picker lists from it
	picker    *picker        // non-nil while the add-connection picker is open
	chars     map[string]*charState
	order     []string // sidebar order of character keys
	collapsed map[string]bool
	active    string
	width     int
	height    int
	status    string
	statusErr bool
	mode      mode
	pendingPW string    // entered password awaiting the save y/n answer
	pendingCh [2]string // world and character id the pending password belongs to
	exportDir string
	confirm   bool                   // next Enter sends an over-limit line anyway
	resizeGen int                    // bumped per WindowSizeMsg; see resizeMsg
	sideTop   int                    // first sidebar row shown when it overflows
	sideShown string                 // active character last scrolled into view
	pwStore   atomic.Pointer[string] // password_store; sessions read it off the UI goroutine
	lastClick struct {               // for spotting a double-click in the sidebar
		char string
		at   time.Time
	}
}

type charState struct {
	key       string
	ch        config.Character
	sess      *session.Session
	cancel    context.CancelFunc
	state     session.State
	cls       *classify.Classifier
	hl        *rules.Highlighter
	sb        Scrollback
	in        *Input
	unread    int
	attention bool
	pin       *conn.PinMismatchError
	needPW    bool
	pwDraft   string           // input stashed while the password prompt is up
	orphan    bool             // removed from the config; dropped when it disconnects
	browse    *browse          // non-nil while browse mode is open
	hist      *history.Reader  // pages older log days into sb; only an in-flight sbOlderMsg read touches it
	leftover  []logstore.Entry // the preload's unshown start of its oldest day
}

// sbOlderMsg carries older scrollback lines, read and rendered off the UI
// goroutine by pageOlder.
type sbOlderMsg struct {
	key   string
	hist  *history.Reader // the reader that was asked; stale if it has changed
	lines []string
	more  bool
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
)

// New builds the model from an already-loaded config and opens the
// autoconnect characters, preloading their recent history. Nothing
// connects until Init.
func New(d Deps, cfg *config.Config) *Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	m := &Model{d: d, chars: map[string]*charState{}, collapsed: map[string]bool{}}
	m.applyConfig(cfg)
	for _, ch := range m.allChars() {
		if ch.Autoconnect {
			m.open(key(ch.World, ch.ID))
		}
	}
	return m
}

// Init connects autoconnect characters and starts the clock and watcher.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(m.d.Now()), m.watch()}
	for _, k := range m.order {
		if m.chars[k].ch.Autoconnect {
			cmds = append(cmds, m.connect(m.chars[k]))
		}
	}
	return tea.Batch(cmds...)
}

func tick(now time.Time) tea.Cmd {
	next := now.Truncate(time.Minute).Add(time.Minute)
	return tea.Tick(next.Sub(now), func(t time.Time) tea.Msg { return tickMsg(t) })
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

// applyConfig keeps cfg for the picker and updates open characters to
// match it. An open character that vanished from the config stays as an
// orphan while it's connected, until it next disconnects; otherwise it
// closes.
func (m *Model) applyConfig(cfg *config.Config) {
	m.cfg = cfg
	m.exportDir = cfg.ExportDir
	store := cfg.PasswordStore
	m.pwStore.Store(&store)
	for _, k := range slices.Clone(m.order) {
		cs := m.chars[k]
		if cs.browse != nil {
			cs.browse.exportDir = cfg.ExportDir
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
		cs.ch, cs.orphan = ch, false
		if err := cs.compile(); err != nil {
			m.setStatus(true, "%s: %v", k, err)
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

func (cs *charState) compile() error {
	cls, err := classify.New(cs.ch.Rules.Classify, cs.ch.Name, cs.ch.Aliases)
	if err != nil {
		return err
	}
	hl, err := rules.New(cs.ch.Rules.Highlight)
	if err != nil {
		return err
	}
	cs.cls, cs.hl = cls, hl
	return nil
}

// preload fills the scrollback with the tail of the most recent log days
// and hands everything older to the scrollback to page in on demand, so
// scrollback is unlimited.
func (m *Model) preload(cs *charState) {
	if m.d.LogRoot == "" {
		return
	}
	hist, err := history.NewReader(m.d.LogRoot, cs.ch.World, cs.ch.ID)
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
	for _, text := range cs.renderDays(entries, startsDay) {
		cs.sb.Append(text)
	}
	cs.sb.Append(style.Dim("─── history ends " + entries[len(entries)-1].Time.Format("Mon Jan 2 15:04") + " ───"))
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
	key, h, cls, hl, leftover := cs.key, cs.hist, cs.cls, cs.hl, cs.leftover
	cs.leftover = nil
	return func() tea.Msg {
		msg := sbOlderMsg{key: key, hist: h}
		if leftover != nil {
			msg.lines, msg.more = renderDays(cls, hl, leftover, true), !h.Exhausted()
			return msg
		}
		if es, _, ok, err := h.LoadOlder(); ok && err == nil {
			msg.lines, msg.more = renderDays(cls, hl, es, true), !h.Exhausted()
		}
		return msg
	}
}

// renderDays renders log entries with a dim divider before the first line
// of each day. The very first entry gets one only if startsDay, i.e. it
// really is the first line of its day.
func (cs *charState) renderDays(entries []logstore.Entry, startsDay bool) []string {
	return renderDays(cs.cls, cs.hl, entries, startsDay)
}

// renderDays is charState.renderDays with the given rules; like
// renderLine it is safe off the UI goroutine.
func renderDays(cls *classify.Classifier, hl *rules.Highlighter, entries []logstore.Entry, startsDay bool) []string {
	out := make([]string, 0, len(entries)+2)
	prev := ""
	for i, e := range entries {
		day := e.Time.Local().Format("2006-01-02")
		if day != prev && (i > 0 || startsDay) {
			out = append(out, style.Dim("── "+dayLabel(day)+" ──"))
		}
		prev = day
		text, _ := renderLine(cls, hl, e)
		out = append(out, text)
	}
	return out
}

// render turns a log entry into a drawable line; the bool reports whether
// a highlight rule asked for attention.
func (cs *charState) render(e logstore.Entry) (string, bool) {
	return renderLine(cs.cls, cs.hl, e)
}

// renderLine styles e with the given rules. It only reads cls and hl
// (compiled once, never mutated), so it is safe off the UI goroutine.
func renderLine(cls *classify.Classifier, hl *rules.Highlighter, e logstore.Entry) (string, bool) {
	text := ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return style.Dim("> " + text), false
	case logstore.Sys:
		return style.Dim("* " + text), false
	}
	plain := ansi.Strip(text)
	res := hl.Apply(plain, cls.Classify(plain))
	if !res.Styled {
		return text + style.Reset, res.Attention
	}
	return style.Apply(text, res.Style), res.Attention
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
		Log:  m.d.NewLog(cs.ch.World, cs.ch.ID),
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
	cs.state = session.Connecting // until the session says otherwise; no ✕ flash
	go s.Run(ctx)
	return waitEvent(cs.key, s)
}

// resizeDebounce is how long the window size must hold still before it is
// reported to servers, so a drag-resize doesn't flood them with NAWS.
const resizeDebounce = 200 * time.Millisecond

// resizeMsg fires resizeDebounce after a WindowSizeMsg; it carries that
// message's generation, and only the latest generation reports.
type resizeMsg int

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
	if older := m.pageOlder(); older != nil {
		cmd = tea.Batch(cmd, older)
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
	case tickMsg:
		return m, tick(time.Time(msg))
	case reloadMsg:
		cfg, err := m.d.Load(m.d.ConfigDir)
		if err != nil {
			m.setStatus(true, "config not reloaded: %v", err)
		} else {
			m.applyConfig(cfg)
			m.setStatus(false, "config reloaded")
		}
		return m, m.watch()
	case eventMsg:
		return m, m.handleEvent(msg)
	case sbOlderMsg:
		if cs := m.chars[msg.key]; cs != nil && cs.hist == msg.hist {
			cs.sb.Prepend(msg.lines, msg.more)
		}
	case olderMsg:
		if cs := m.chars[msg.key]; cs != nil && cs.browse == msg.b {
			return m, cs.browse.receive(msg)
		}
	case tea.PasteMsg:
		if m.picker != nil {
			m.picker.form.paste(msg.Content)
			m.fixPick()
		} else if cs := m.cur(); cs != nil && cs.browse != nil {
			// Only a text prompt takes a paste; the chat draft must not.
			if p := cs.browse.prompt; p == promptFind || p == promptDate || p == promptFilename {
				cs.browse.pin.InsertText(strings.ReplaceAll(msg.Content, "\n", " "))
			}
		} else if cs := m.cur(); cs != nil && m.mode == modeNormal {
			cs.in.InsertText(msg.Content)
			m.confirm = false
		}
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseWheelMsg:
		return m, m.handleWheel(msg)
	case tea.MouseClickMsg:
		return m, m.handleClick(msg)
	}
	return m, nil
}

func (m *Model) cur() *charState { return m.chars[m.active] }

// passwordStore is the password_store setting.
func (m *Model) passwordStore() string {
	if p := m.pwStore.Load(); p != nil && *p != "" {
		return *p
	}
	return config.DefaultPasswordStore
}

func (m *Model) setStatus(isErr bool, format string, args ...any) {
	m.status, m.statusErr = fmt.Sprintf(format, args...), isErr
}

func (m *Model) handleEvent(msg eventMsg) tea.Cmd {
	cs, ok := m.chars[msg.key]
	if !ok || cs.sess != msg.sess || !msg.ok {
		return nil // stale session, or it has shut down
	}
	ev := msg.ev
	switch ev.Kind {
	case session.EventLine:
		text, attn := cs.render(ev.Entry)
		cs.sb.Append(text)
		if cs.browse != nil {
			cs.browse.appendLive(ev.Entry)
		}
		if msg.key != m.active && ev.Entry.Dir == logstore.In {
			cs.unread++
			cs.attention = cs.attention || attn
		}
	case session.EventPrompt:
		cs.sb.SetPrompt(ansi.Sanitize(ev.Entry.Text))
	case session.EventState:
		cs.state = ev.State
		if cs.orphan && (ev.State == session.Disconnected || ev.State == session.Failed) {
			m.close(msg.key) // don't reconnect (and log in) a deleted character
			return nil
		}
		var pin *conn.PinMismatchError
		if ev.State == session.Failed && errors.As(ev.Err, &pin) {
			cs.pin = pin
			m.setStatus(true, "%s: certificate changed; /trust to accept", cs.ch.Name)
		}
		if ev.State == session.Connected {
			cs.pin = nil
		}
		if ev.State != session.Connected && cs.needPW {
			cs.endPassword()
		}
	case session.EventLogError:
		m.setStatus(true, "%s: log write failed: %v", cs.ch.Name, ev.Err)
	case session.EventNeedPassword:
		cs.startPassword()
	}
	return waitEvent(msg.key, msg.sess)
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	cs := m.cur()
	if m.mode == modeSavePassword {
		switch k.String() {
		case "enter", "y", "Y":
			// Save for the character that was asked about, even if another
			// one is active now.
			store := m.passwordStore()
			switch err := m.d.SavePassword(store, m.pendingCh[0], m.pendingCh[1], m.pendingPW); {
			case err != nil && store == "keychain":
				m.setStatus(true, `keychain: %v (set password_store = "file" or "none" in config.toml)`, err)
			case err != nil:
				m.setStatus(true, "password not saved: %v", err)
			default:
				m.setStatus(false, "password saved")
			}
		case "n", "N", "esc", "ctrl+c":
			m.setStatus(false, "password not saved")
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
	}
	if m.picker != nil {
		return m.pickerKey(k)
	}
	if cs != nil && cs.browse != nil {
		cmd, closed := cs.browse.key(k, m.browseBodyH())
		if closed {
			cs.browse = nil
		}
		return cmd
	}
	switch k.String() {
	case openPickerKey:
		m.openPicker()
		return nil
	case openBrowseKey:
		if cs != nil {
			m.openBrowse(cs)
		}
		return nil
	case "ctrl+c":
		if cs == nil || cs.in.Empty() {
			return m.quit()
		}
		cs.in.Reset()
	case "pgup":
		if cs != nil {
			cs.sb.ScrollUp(max(1, m.layout().sbH-1))
		}
	case "pgdown":
		if cs != nil {
			cs.sb.ScrollDown(max(1, m.layout().sbH-1))
		}
	case "esc":
		m.confirm = false
		if cs != nil && cs.needPW {
			cs.endPassword()
			m.setStatus(false, "skipped login")
		}
	case "enter":
		return m.submit()
	}
	if cs == nil {
		return nil
	}
	switch k.String() {
	case "shift+enter", "alt+enter":
		cs.in.Newline()
	case "up":
		cs.in.Up()
	case "down":
		cs.in.Down()
	default:
		if !editKey(cs.in, k) {
			return nil
		}
	}
	m.confirm = false
	return nil
}

func (m *Model) quit() tea.Cmd {
	for _, cs := range m.chars {
		if cs.cancel != nil {
			cs.cancel()
		}
	}
	return tea.Quit
}

// switchBy moves the active character through the sidebar order.
func (m *Model) switchBy(delta int) {
	if len(m.order) == 0 {
		return
	}
	i := 0
	for j, k := range m.order {
		if k == m.active {
			i = j
		}
	}
	i = (i + delta + len(m.order)) % len(m.order)
	m.switchTo(m.order[i])
}

func (m *Model) switchTo(k string) {
	cs, ok := m.chars[k]
	if !ok {
		return
	}
	m.active, m.confirm = k, false
	cs.unread, cs.attention = 0, false
	delete(m.collapsed, cs.ch.World)
}

// submit handles Enter: a command, a password, or lines for the server.
func (m *Model) submit() tea.Cmd {
	cs := m.cur()
	if cs == nil {
		return nil
	}
	if cs.needPW {
		pw := cs.in.CommitSecret()
		e, err := cs.sess.Login(pw)
		if err != nil && !isLogErr(err) {
			m.setStatus(true, "login not sent: %v", err)
			return nil
		}
		cs.endPassword()
		cs.sb.Append(style.Dim("> " + e.Text))
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
		m.setStatus(true, "%s is not connected (/connect)", cs.ch.Name)
		return nil
	}
	if cs.in.OverLimit(cs.ch.MaxLineBytes, cs.ch.NewlineMode == "flatten") && !m.confirm {
		m.confirm = true
		m.setStatus(true, "over %d bytes: Enter again to send anyway", cs.ch.MaxLineBytes)
		return nil
	}
	m.confirm = false
	lines := strings.Split(text, "\n")
	if cs.ch.NewlineMode == "flatten" {
		lines = []string{strings.Join(lines, " ")}
	}
	secret := false
	for _, line := range lines {
		e, err := cs.sess.Send(line)
		if err != nil && !isLogErr(err) {
			m.setStatus(true, "not sent: %v", err)
			break
		}
		if err != nil {
			m.setStatus(true, "%v", err)
		}
		secret = secret || e.Text != line // the session redacted a typed password
		cs.sb.Append(style.Dim("> " + ansi.Sanitize(e.Text)))
	}
	if secret {
		cs.in.CommitSecret()
	} else {
		cs.in.Commit()
	}
	cs.sb.ToBottom()
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
	switch args[0] {
	case "/connect":
		if cs.sess != nil && cs.state == session.Connected {
			m.setStatus(false, "%s is already connected", cs.ch.Name)
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
			m.setStatus(true, "no changed certificate to trust")
			return nil
		}
		if err := m.d.KnownHosts.Trust(cs.pin.HostPort, cs.pin.Got); err != nil {
			m.setStatus(true, "trust: %v", err)
			return nil
		}
		m.setStatus(false, "trusted new certificate for %s", cs.pin.HostPort)
		cs.pin = nil
		return m.connect(cs)
	case "/close":
		m.close(cs.key)
	case "/quit":
		return m.quit()
	case "/browse":
		m.openBrowse(cs)
	case "/highlight":
		text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), args[0]))
		if err := config.AppendHighlight(m.d.ConfigDir, cs.ch.World, text); err != nil {
			m.setStatus(true, "highlight: %v", err)
			return nil
		}
		m.setStatus(false, "added highlight for %q", text)
	default:
		m.setStatus(true, "unknown command %s", args[0])
	}
	return nil
}

// openBrowse opens browse mode for cs.
func (m *Model) openBrowse(cs *charState) {
	cs.browse = newBrowse(cs, m.d.LogRoot)
	cs.browse.exportDir = m.exportDir
	m.status = ""
}

// browseBodyH is the number of line rows in browse mode: the pane less
// two header rows, two rules, the action bar and the statusline.
func (m *Model) browseBodyH() int { return max(1, m.height-6) }

func (m *Model) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	l := m.layout()
	cs := m.cur()
	if cs != nil && cs.browse != nil && msg.X > l.sw {
		switch msg.Button {
		case tea.MouseWheelUp:
			return cs.browse.moveCursor(-3)
		case tea.MouseWheelDown:
			return cs.browse.moveCursor(3)
		}
		return nil
	}
	if msg.X < l.sw {
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrollSidebar(-3)
		case tea.MouseWheelDown:
			m.scrollSidebar(3)
		}
		return nil
	}
	if cs == nil || msg.X <= l.sw || msg.Y >= l.sbH {
		return nil
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		cs.sb.ScrollUp(3)
	case tea.MouseWheelDown:
		cs.sb.ScrollDown(3)
	}
	return nil
}

// doubleClick is the longest gap between the clicks of a double-click.
const doubleClick = 400 * time.Millisecond

func (m *Model) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	if msg.Button != tea.MouseLeft {
		return nil
	}
	l := m.layout()
	if msg.X < l.sw {
		sv := m.sidebarView()
		r, hint := sv.at(msg.Y)
		switch {
		case hint != 0:
			m.scrollSidebar(hint * max(1, sv.avail-1))
		case r == nil:
		case m.picker != nil:
			if r.kind == rowChar {
				return m.pick(r.char)
			}
		case r.kind == rowAdd:
			m.openPicker()
		case r.kind == rowWorld:
			m.collapsed[r.world] = !m.collapsed[r.world]
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
	if cs := m.cur(); cs != nil && cs.browse != nil {
		cs.browse.click(msg.X-l.sw-1, msg.Y, msg.Mod&tea.ModShift != 0)
		return nil
	}
	cs := m.cur()
	if cs == nil {
		return nil
	}
	if cs.sb.Scrolled() && msg.Y == l.sbH-1 && msg.X >= m.width-l.pillW {
		cs.sb.ToBottom()
	}
	if y := msg.Y - l.sbH - 1; !l.prompt && y >= 0 && y < len(l.inRows) && msg.X > l.sw {
		cs.in.Click(msg.X-l.sw-3, l.inTop+y) // past the separator and gutter
	}
	return nil
}
