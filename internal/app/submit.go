package app

import (
	"errors"
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
	c.NeedPW, c.pwDraft, c.Text = true, c.Text, ""
	c.History.Stop()
}

// endPassword takes the prompt down (submitted, skipped, or the
// connection dropped), bringing the draft back.
func (c *Char) endPassword() {
	c.NeedPW, c.Text, c.pwDraft = false, c.pwDraft, ""
	c.History.Stop()
}

// SetInput is the active input's text, as the front end has it now.
func (a *App) SetInput(text string) {
	if c := a.chars[a.active]; c != nil {
		c.Text = text
	} else {
		a.idleText = text
	}
}

// Text is the active input's text.
func (a *App) Text() string {
	if c := a.chars[a.active]; c != nil {
		return c.Text
	}
	return a.idleText
}

// IdleHistory is the history of the input used while nothing is open.
func (a *App) IdleHistory() *History { return &a.idleHist }

// Unconfirm is an edit: the next Submit of an over-limit line asks again.
func (a *App) Unconfirm() { a.confirm = false }

// Confirming reports whether the next Submit sends an over-limit line.
func (a *App) Confirming() bool { return a.confirm }

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
	c := a.chars[a.active]
	if c == nil {
		text := a.idleText
		if isCommand(text) {
			a.idleHist.Add(text)
			a.idleText = ""
			return SubmitResult{}, a.command(nil, text)
		}
		a.SetStatus(true, str.StatusNothingOpen())
		return SubmitResult{}, nil
	}
	if c.NeedPW {
		pw := c.Text
		c.Text = ""
		c.History.Stop()
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
	text := c.Text
	if isCommand(text) {
		c.History.Add(text)
		c.Text = ""
		return SubmitResult{}, a.command(c, text)
	}
	if c.Sess == nil || c.State != session.Connected {
		a.SetStatus(true, str.StatusNotConnected(c.Ch.Name))
		return SubmitResult{}, nil
	}
	flatten := c.Ch.NewlineMode == "flatten"
	if overLimit(text, c.Ch.MaxLineBytes, flatten) && !a.confirm {
		a.confirm = true
		a.SetStatus(true, str.StatusOverLimit(c.Ch.MaxLineBytes))
		return SubmitResult{}, nil
	}
	a.confirm = false
	lines := strings.Split(strings.TrimPrefix(text, "/"), "\n") // "//foo" sends "/foo"
	if flatten {
		lines = []string{strings.Join(lines, " ")}
	}
	res := SubmitResult{Sent: true}
	secret := false
	for _, line := range lines {
		e, err := c.Sess.Send(line)
		if err != nil && !isLogErr(err) {
			a.SetStatus(true, str.StatusNotSent(err))
			break
		}
		if err != nil {
			a.SetStatus(true, err.Error())
		}
		secret = secret || e.Text != line // the session redacted a typed password
		res.Echoed = a.Echo(c.Key, e) != nil
	}
	if !secret {
		c.History.Add(text)
	}
	c.Text = ""
	c.History.Stop()
	return res, nil
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
	case "/backup", "/restore": // need no character; the front end knows whether it has them
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
