// Package ui is Kiln's terminal interface: a sidebar of worlds and
// characters, and a right pane with scrollback, input box and statusline.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/conn"
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
	ConfigDir    string
	LogRoot      string
	KnownHosts   conn.KnownHosts
	Load         func(dir string) (*config.Config, error)
	Dial         func(ctx context.Context, ch config.Character) (session.LineConn, error)
	NewLog       func(world, char string) session.Appender
	Password     func(world, char string) (string, error)
	SavePassword func(world, char, password string) error // nil: never offer
	Changes      <-chan struct{}                          // config changes; nil: no hot reload
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
	confirm   bool      // next Enter sends an over-limit line anyway
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

// New builds the model from an already-loaded config and preloads each
// character's recent history. Nothing connects until Init.
func New(d Deps, cfg *config.Config) *Model {
	if d.Now == nil {
		d.Now = time.Now
	}
	m := &Model{d: d, chars: map[string]*charState{}, collapsed: map[string]bool{}}
	m.applyConfig(cfg)
	for _, k := range m.order {
		m.preload(m.chars[k])
	}
	if len(m.order) > 0 {
		m.active = m.order[0]
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

// applyConfig adds, updates and removes characters to match cfg.
// Connected characters that vanished from the config stay until they
// disconnect.
func (m *Model) applyConfig(cfg *config.Config) {
	var order []string
	seen := map[string]bool{}
	for _, w := range cfg.Worlds {
		for _, ch := range w.Characters {
			k := key(ch.World, ch.ID)
			seen[k] = true
			order = append(order, k)
			cs, ok := m.chars[k]
			if !ok {
				cs = &charState{key: k, in: NewInput()}
				m.chars[k] = cs
			}
			cs.ch = ch
			if err := cs.compile(); err != nil {
				m.setStatus(true, "%s: %v", k, err)
			}
			if cs.sess != nil {
				cs.sess.SetChar(ch)
			}
		}
	}
	for _, k := range m.order {
		if seen[k] {
			continue
		}
		cs := m.chars[k]
		if cs.sess != nil && cs.state != session.Disconnected && cs.state != session.Failed {
			order = append(order, k) // keep until it disconnects
			continue
		}
		if cs.cancel != nil {
			cs.cancel()
		}
		delete(m.chars, k)
	}
	m.order = order
	if _, ok := m.chars[m.active]; !ok {
		m.active = ""
		if len(order) > 0 {
			m.active = order[0]
		}
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

// preload fills the scrollback with the tail of the most recent log days.
func (m *Model) preload(cs *charState) {
	if m.d.LogRoot == "" {
		return
	}
	days, err := logstore.Days(m.d.LogRoot, cs.ch.World, cs.ch.ID)
	if err != nil || len(days) == 0 {
		return
	}
	var entries []logstore.Entry
	for i := len(days) - 1; i >= 0 && len(entries) < HistoryLines; i-- {
		es, err := logstore.ReadDay(m.d.LogRoot, cs.ch.World, cs.ch.ID, days[i])
		if err != nil {
			continue
		}
		entries = append(es, entries...)
	}
	if len(entries) > HistoryLines {
		entries = entries[len(entries)-HistoryLines:]
	}
	for _, e := range entries {
		text, _ := cs.render(e)
		cs.sb.Append(text)
	}
	cs.sb.Append(style.Dim("─── history ends " + entries[len(entries)-1].Time.Format("Mon Jan 2 15:04") + " ───"))
}

// render turns a log entry into a drawable line; the bool reports whether
// a highlight rule asked for attention.
func (cs *charState) render(e logstore.Entry) (string, bool) {
	text := ansi.Sanitize(e.Text)
	switch e.Dir {
	case logstore.Out:
		return style.Dim("> " + text), false
	case logstore.Sys:
		return style.Dim("* " + text), false
	}
	plain := ansi.Strip(text)
	res := cs.hl.Apply(plain, cs.cls.Classify(plain))
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
			return m.d.Password(ch.World, ch.ID)
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cs.sess, cs.cancel = s, cancel
	go s.Run(ctx)
	return waitEvent(cs.key, s)
}

// Update handles one message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
	case tea.PasteMsg:
		if cs := m.cur(); cs != nil && m.mode == modeNormal {
			cs.in.InsertText(msg.Content)
			m.confirm = false
		}
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.MouseWheelMsg:
		m.handleWheel(msg)
	case tea.MouseClickMsg:
		m.handleClick(msg)
	}
	return m, nil
}

func (m *Model) cur() *charState { return m.chars[m.active] }

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
		if msg.key != m.active && ev.Entry.Dir == logstore.In {
			cs.unread++
			cs.attention = cs.attention || attn
		}
	case session.EventPrompt:
		cs.sb.SetPrompt(ansi.Sanitize(ev.Entry.Text))
	case session.EventState:
		cs.state = ev.State
		var pin *conn.PinMismatchError
		if ev.State == session.Failed && errors.As(ev.Err, &pin) {
			cs.pin = pin
			m.setStatus(true, "%s: certificate changed; /trust to accept", cs.ch.Name)
		}
		if ev.State == session.Connected {
			cs.pin = nil
		}
		if ev.State != session.Connected {
			cs.needPW = false
		}
	case session.EventLogError:
		m.setStatus(true, "%s: log write failed: %v", cs.ch.Name, ev.Err)
	case session.EventNeedPassword:
		cs.needPW = true
		if msg.key == m.active {
			m.setStatus(false, "enter password for %s (Esc to skip)", cs.ch.Name)
		}
	}
	return waitEvent(msg.key, msg.sess)
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	cs := m.cur()
	if m.mode == modeSavePassword {
		switch k.String() {
		case "y", "Y":
			// Save for the character that was asked about, even if another
			// one is active now.
			if err := m.d.SavePassword(m.pendingCh[0], m.pendingCh[1], m.pendingPW); err != nil {
				m.setStatus(true, "keychain: %v", err)
			} else {
				m.setStatus(false, "password saved")
			}
		default:
			m.setStatus(false, "password not saved")
		}
		m.mode, m.pendingPW, m.pendingCh = modeNormal, "", [2]string{}
		return nil
	}
	switch k.String() {
	case "ctrl+c":
		if cs == nil || cs.in.Empty() {
			return m.quit()
		}
		cs.in.Reset()
	case "ctrl+up":
		m.switchBy(-1)
	case "ctrl+down":
		m.switchBy(1)
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
			cs.needPW = false
			cs.in.Reset()
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
	case "left":
		cs.in.Left()
	case "right":
		cs.in.Right()
	case "home", "ctrl+a":
		cs.in.Home()
	case "end", "ctrl+e":
		cs.in.End()
	case "backspace":
		cs.in.Backspace()
	case "delete":
		cs.in.Delete()
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			cs.in.InsertText(k.Text)
		} else {
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
	if cs.needPW {
		m.setStatus(false, "enter password for %s (Esc to skip)", cs.ch.Name)
	}
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
		cs.needPW = false
		cs.sb.Append(style.Dim("> " + e.Text))
		if m.d.SavePassword != nil && pw != "" {
			m.mode, m.pendingPW = modeSavePassword, pw
			m.pendingCh = [2]string{cs.ch.World, cs.ch.ID}
			m.setStatus(false, "save password for %s in the keychain? [y/n]", cs.ch.Name)
		}
		return nil
	}
	m.status = ""
	text := cs.in.Value()
	if strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//") {
		cs.in.Commit()
		return m.command(cs, strings.Fields(text))
	}
	text = strings.TrimPrefix(text, "/") // "//foo" sends "/foo"
	if cs.sess == nil || cs.state != session.Connected {
		m.setStatus(true, "%s is not connected (/connect)", cs.ch.Name)
		return nil
	}
	if cs.in.OverLimit(cs.ch.MaxLineBytes) && !m.confirm {
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

func (m *Model) command(cs *charState, args []string) tea.Cmd {
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
	case "/quit":
		return m.quit()
	default:
		m.setStatus(true, "unknown command %s", args[0])
	}
	return nil
}

func (m *Model) handleWheel(msg tea.MouseWheelMsg) {
	l := m.layout()
	cs := m.cur()
	if cs == nil || msg.X <= l.sw || msg.Y >= l.sbH {
		return
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		cs.sb.ScrollUp(3)
	case tea.MouseWheelDown:
		cs.sb.ScrollDown(3)
	}
}

func (m *Model) handleClick(msg tea.MouseClickMsg) {
	if msg.Button != tea.MouseLeft {
		return
	}
	l := m.layout()
	if msg.X < l.sw {
		rows := m.sidebarRows()
		if msg.Y < len(rows) {
			r := rows[msg.Y]
			if r.char == "" {
				m.collapsed[r.world] = !m.collapsed[r.world]
			} else {
				m.switchTo(r.char)
			}
		}
		return
	}
	if cs := m.cur(); cs != nil && cs.sb.Scrolled() && msg.Y == l.sbH-1 && msg.X >= m.width-l.pillW {
		cs.sb.ToBottom()
	}
}
