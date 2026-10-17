package app

import (
	"errors"
	"slices"
	"strings"

	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/session"
	"github.com/latrani/Kiln/internal/str"
)

// SubmitResult is what Submit did that the front end shows: whether
// lines went to the server, and whether any of them echoed.
type SubmitResult struct {
	Sent, Echoed bool
}

// startPassword puts up the password prompt, stashing the draft so it
// neither becomes part of the password nor is lost.
func (c *Char) startPassword() {
	if c.NeedPW {
		return
	}
	if c.History.IsOpen() {
		c.Text = c.History.Close()
	}
	c.NeedPW, c.pwDraft, c.Text = true, c.Text, ""
}

// endPassword takes the prompt down (submitted, skipped, or the
// connection dropped), bringing the draft back.
func (c *Char) endPassword() {
	c.NeedPW, c.Text, c.pwDraft = false, c.pwDraft, ""
}

// SetInput is the active input's text, as the front end has it now.
// While the history panel is up, it's the panel's query.
func (a *App) SetInput(text string) {
	if c := a.chars[a.active]; c != nil {
		c.Text = text
	} else {
		a.idleText = text
	}
	if h := a.History(); h.IsOpen() {
		h.SetQuery(text)
	}
}

// Text is the active input's text.
func (a *App) Text() string {
	if c := a.chars[a.active]; c != nil {
		return c.Text
	}
	return a.idleText
}

// Draft is what open character k's input holds that a person was
// typing to say: at the password prompt, the draft it put aside, never
// the password. "" if k isn't open.
func (a *App) Draft(k string) string {
	c := a.chars[k]
	switch {
	case c == nil:
		return ""
	case c.NeedPW:
		return c.pwDraft
	}
	return c.Text
}

// SetDraft puts text in open character k's input as what a person is
// typing to say: a draft saved from before, or typed in a front end that
// keeps the password apart. At the password prompt it's the draft put
// aside, never the password. Nothing if k isn't open.
func (a *App) SetDraft(k, text string) {
	switch c := a.chars[k]; {
	case c == nil:
	case c.NeedPW:
		c.pwDraft = text
	default:
		c.Text = text
	}
}

// IdleText is the text of the input used while nothing is open.
func (a *App) IdleText() string { return a.idleText }

// History is the active input's history: the active character's, or
// the idle input's. Front ends read its panel from it; App changes it.
func (a *App) History() *History {
	if c := a.chars[a.active]; c != nil {
		return &c.History
	}
	return &a.idleHist
}

// HistoryOpen puts the active input's history panel up, its text the
// query. The front end shows the panel while History().IsOpen(). With
// nothing open there's no panel.
func (a *App) HistoryOpen() []Effect {
	if a.chars[a.active] == nil {
		return nil
	}
	h := a.History()
	if !h.IsOpen() {
		h.Open(a.Text())
	}
	return a.moreSent()
}

// HistoryMove moves the panel's selection delta lines newer (negative:
// older). Past the newest the panel closes, the draft back in the input.
func (a *App) HistoryMove(delta int) []Effect {
	h := a.History()
	if !h.IsOpen() {
		return nil
	}
	if draft, closed := h.Move(delta); closed {
		a.SetInput(draft)
		return nil
	}
	return a.moreSent()
}

// HistorySelect selects the panel's Shown()[i], as a click on it does.
func (a *App) HistorySelect(i int) []Effect {
	a.History().Select(i)
	return a.moreSent()
}

// HistoryPick closes the panel and puts its selected line in the input,
// to edit and send. Nothing if nothing's shown.
func (a *App) HistoryPick() {
	if v, ok := a.History().Pick(); ok {
		a.SetInput(v)
	}
}

// HistoryClose closes the panel, the draft back in the input.
func (a *App) HistoryClose() {
	if h := a.History(); h.IsOpen() {
		a.SetInput(h.Close())
	}
}

// HistoryToggle opens the panel, or closes it if it's up: the History
// button.
func (a *App) HistoryToggle() []Effect {
	if a.History().IsOpen() {
		a.HistoryClose()
		return nil
	}
	return a.HistoryOpen()
}

// HistoryScope is which lines the history panels list. It holds until
// Kiln quits.
func (a *App) HistoryScope() Scope { return a.sent.scope }

// SetHistoryScope has the history panels list s's lines; each panel
// selects its newest.
func (a *App) SetHistoryScope(s Scope) []Effect {
	if s != a.sent.scope {
		a.sent.scope = s
		a.sent.gen++
		a.idleHist.back = 0
		for _, c := range a.chars {
			c.History.back = 0
		}
	}
	return a.moreSent()
}

// SwapHistoryScope sets the other scope: Tab on the panel.
func (a *App) SwapHistoryScope() []Effect {
	if a.sent.scope == ScopeSession {
		return a.SetHistoryScope(ScopeCharacter)
	}
	return a.SetHistoryScope(ScopeSession)
}

// Unconfirm drops the confirmation, as Esc does: the next Submit of an
// over-limit line asks again.
func (a *App) Unconfirm() { a.confirm = "" }

// Confirming reports whether an over-limit line is waiting for Enter
// again.
func (a *App) Confirming() bool { return a.confirm != "" }

// SkipLogin takes down the active character's password prompt, if it's
// up, and says so; false if there was none.
func (a *App) SkipLogin() bool {
	c := a.chars[a.active]
	if c == nil || !c.NeedPW {
		return false
	}
	c.endPassword()
	a.SetStatus(false, str.StatusSkippedLogin())
	return true
}

// PendingSave is the character whose just-used password awaits "save
// it?"; ok is false when nothing's asked.
func (a *App) PendingSave() (world, char string, ok bool) {
	if a.pending == nil {
		return "", "", false
	}
	return a.pending.world, a.pending.char, true
}

// AnswerSave answers "save it?", saving to password_store for the
// character that was asked about, even if another is active now.
func (a *App) AnswerSave(save bool) {
	p := a.pending
	if p == nil {
		return
	}
	a.pending = nil
	if !save {
		a.SetStatus(false, str.StatusPasswordNotSaved())
		return
	}
	store := a.PasswordStore()
	switch err := a.d.SavePassword(store, p.world, p.char, p.pw); {
	case err != nil && store == "keychain":
		a.SetStatus(true, str.StatusKeychainFailed(err))
	case err != nil:
		a.SetStatus(true, str.StatusPasswordNotSavedErr(err))
	default:
		a.SetStatus(false, str.StatusPasswordSaved())
	}
}

func isLogErr(err error) bool {
	var le *session.LogError
	return errors.As(err, &le)
}

func isCommand(text string) bool {
	return strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//")
}

// OverLimit reports whether text is too long to send as ch: a line over
// max_line_bytes, or with newline_mode "flatten" the lines joined.
func OverLimit(text string, ch config.Character) bool {
	return overLimit(text, ch.MaxLineBytes, ch.NewlineMode == "flatten")
}

// overLimit reports whether any line of text is longer than limit bytes,
// or with joined whether its lines joined by spaces are.
func overLimit(text string, limit int, joined bool) bool {
	if limit <= 0 {
		return false
	}
	lines := strings.Split(text, "\n")
	if joined {
		return len(strings.Join(lines, " ")) > limit
	}
	for _, l := range lines {
		if len(l) > limit {
			return true
		}
	}
	return false
}

// Submit is Enter on the active input: a command, the password at the
// prompt, a connect when there's nothing to send, or lines for the
// server. A front end handles Enter on a world's overview, and on an
// empty input with nothing open, itself.
func (a *App) Submit() (SubmitResult, []Effect) {
	if a.History().IsOpen() { // Enter on the panel takes its line to edit
		a.HistoryPick()
		return SubmitResult{}, nil
	}
	c := a.chars[a.active]
	if c == nil {
		text := a.idleText
		if isCommand(text) {
			a.idleHist.Add(text, a.d.Now())
			a.idleText = ""
			return SubmitResult{}, a.command(nil, text)
		}
		a.SetStatus(true, str.StatusNothingOpen())
		return SubmitResult{}, nil
	}
	if c.NeedPW {
		pw := c.Text
		c.Text = ""
		e, err := c.Sess.Login(pw)
		if err != nil && !isLogErr(err) {
			a.SetStatus(true, str.StatusLoginNotSent(err))
			return SubmitResult{}, nil
		}
		c.endPassword()
		a.Echo(c.Key, e)
		if a.d.SavePassword != nil && pw != "" && a.PasswordStore() != "none" {
			a.pending = &pendingSave{world: c.Ch.World, char: c.Ch.ID, pw: pw}
		}
		return SubmitResult{}, nil
	}
	a.ClearStatus()
	if c.Text == "" && c.State != session.Connected {
		if c.State == session.Connecting || c.Pin != nil {
			return SubmitResult{}, nil // nothing Enter can do; the prompt says why
		}
		return SubmitResult{}, a.Connect(c.Key)
	}
	// Each line is its own input: a command runs, and anything else is
	// sent. With newline_mode "flatten" the lines are joined into one.
	text := c.Text
	lines := strings.Split(text, "\n")
	if c.Ch.NewlineMode == "flatten" {
		lines = []string{strings.Join(lines, " ")}
	}
	sends := slices.ContainsFunc(lines, func(l string) bool { return !isCommand(l) })
	if sends && (c.Sess == nil || c.State != session.Connected) {
		a.SetStatus(true, str.StatusNotConnected(c.Ch.Name))
		return SubmitResult{}, nil
	}
	if sends && OverLimit(text, c.Ch) && text != a.confirm {
		a.confirm = text
		a.SetStatus(true, str.StatusOverLimit(c.Ch.MaxLineBytes))
		return SubmitResult{}, nil
	}
	a.confirm = ""
	c.Text = ""
	var res SubmitResult
	var effs []Effect
	for _, line := range lines {
		if isCommand(line) {
			c.History.Add(line, a.d.Now())
			effs = append(effs, a.command(c, line)...)
			continue
		}
		if c.Sess == nil || c.State != session.Connected { // a command before it disconnected
			a.SetStatus(true, str.StatusNotConnected(c.Ch.Name))
			break
		}
		out := strings.TrimPrefix(line, "/") // "//foo" sends "/foo"
		e, err := c.Sess.Send(out)
		if err != nil && !isLogErr(err) {
			a.SetStatus(true, str.StatusNotSent(err))
			break
		}
		if err != nil {
			a.SetStatus(true, err.Error())
		}
		if e.Text == out { // unless the session redacted a typed password
			c.History.Add(line, a.d.Now())
		}
		res.Sent = true
		res.Echoed = a.Echo(c.Key, e) != nil
	}
	return res, effs
}

// command runs a slash command. text is the whole input line, so commands
// that take free text (/highlight) keep its spacing. c is nil with
// nothing open.
func (a *App) command(c *Char, text string) []Effect {
	args := strings.Fields(text)
	key := ""
	if c != nil {
		key = c.Key
	}
	do := func() []Effect { return []Effect{Do{Cmd: args[0], Args: args[1:], Key: key}} }
	switch args[0] {
	case "/backup", "/restore", "/reset": // need no character; the front end knows whether it has them
		return do()
	case "/away":
		a.SetAway()
		return nil
	}
	if c == nil && args[0] != "/quit" && args[0] != "/open" {
		a.SetStatus(true, str.StatusNeedsCharacter(args[0]))
		return nil
	}
	switch args[0] {
	case "/connect":
		if c.Sess != nil && c.State == session.Connected {
			a.SetStatus(false, str.StatusAlreadyConnected(c.Ch.Name))
			return nil
		}
		return a.Connect(c.Key)
	case "/reconnect":
		return a.Connect(c.Key)
	case "/disconnect":
		if c.Sess != nil {
			c.Sess.Disconnect()
		}
	case "/trust":
		if c.Pin == nil {
			a.SetStatus(true, str.StatusNoChangedCert())
			return nil
		}
		if err := a.d.KnownHosts.Trust(c.Pin.HostPort, c.Pin.Got); err != nil {
			a.SetStatus(true, str.StatusTrustFailed(err))
			return nil
		}
		a.SetStatus(false, str.StatusTrusted(c.Pin.HostPort))
		c.Pin = nil
		return a.Connect(c.Key)
	case "/close":
		a.Close(c.Key)
	case "/quit":
		return a.Quit()
	case "/open", "/log", "/edit":
		return do()
	case "/notify":
		a.notifyCommand(c, args[1:])
	case "/highlight":
		text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), args[0]))
		if err := config.AppendHighlight(a.d.ConfigDir, c.Ch.World, text); err != nil {
			a.SetStatus(true, str.StatusHighlightFailed(err))
			return nil
		}
		a.SetStatus(false, str.StatusHighlightAdded(text))
	default:
		a.SetStatus(true, str.StatusUnknownCommand(args[0]))
	}
	return nil
}
