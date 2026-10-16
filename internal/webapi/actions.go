package webapi

import (
	"errors"
	"fmt"

	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/feed"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
)

// Action is one thing the page asks for, as JSON: Do names it and the
// other fields are its arguments. Lines are named by number (Seq, Start,
// End).
type Action struct {
	Do                                                  string
	Key, Text, Term, Tag, When, Format, Name, ID, World string
	Char                                                string
	Seq, Start, End, Epoch, Cols, Rows                  int
	N                                                   int // numbers input, submit and older (see feed.Input.Ack); step's distance; next_unread's direction; history_move's distance; history_pick's line
	In, Shut, Save                                      bool
	Err                                                 bool // say's
	Item                                                *scene.Item
	WorldSettings                                       *config.WorldSettings
	CharSettings                                        *config.CharacterSettings
	Data                                                []byte // a file's bytes (restore), base64 in JSON
}

// do carries out act. See step for what it returns.
func (l *Loop) do(act Action) (extra []any, effs []app.Effect, all bool) {
	a := l.a
	// /reset's arm lasts one step of the person's: anything they do but
	// Enter on it again (an edit, another command, Esc) asks again. What
	// the page sends by itself doesn't count.
	armed := l.resetArmed
	if !pageOwn(act, a.Text()) {
		l.resetArmed = false
	}
	switch act.Do {
	case "open": //str:ok
		l.open(act.Key)
	case "save_drafts": //str:ok
		l.saveDrafts()
	case "close": //str:ok
		if a.Char(act.Key) != nil {
			a.Close(act.Key)
		}
	case "switch": //str:ok
		a.Switch(act.Key)
	case "pick": //str:ok
		// The picker's Enter on a character, as the TUI's pick: open it,
		// show it and connect it. One already open (autoconnect, or opened
		// since the page drew the picker) is only shown.
		was := a.Char(act.Key) != nil
		l.open(act.Key)
		if a.Char(act.Key) != nil {
			a.Switch(act.Key)
			if !was {
				effs = a.Connect(act.Key)
			}
		}
	case "step": //str:ok
		if k := a.StepTarget(act.N); k != "" {
			a.Switch(k)
		}
	case "next_unread": //str:ok
		if k := a.UnreadTarget(act.N); k != "" {
			a.Switch(k)
		}
	case "input", "submit", "older": //str:ok
		k := l.inputKey()
		if c := a.Char(act.Key); act.Key != k || c != nil && c.NeedPW {
			// The page hadn't heard the active input changed, or that the
			// password prompt went up. Typed or sent for a character still
			// open, the text is its draft, never sent somewhere else or as
			// the password (that's the password action); the page cleared
			// what it sent, so it hears it back. Anything else is dropped.
			if c != nil && (act.Do == "input" || act.Do == "submit") { //str:ok
				a.SetDraft(act.Key, act.Text)
				l.f.Told(act.Key, act.N)
				if act.Do == "submit" { //str:ok
					l.f.Resend(act.Key)
				}
			}
			break
		}
		before, gen := a.Text(), a.Status().Gen
		a.SetInput(act.Text)
		l.f.Told(k, act.N)
		switch act.Do {
		case "input": //str:ok
			if act.Text != before && a.Status().Gen == gen {
				a.ClearStatus() // typing again dismisses what was said before
			}
		case "submit": //str:ok
			var res app.SubmitResult
			res, effs = a.Submit()
			effs = l.reset(l.backup(effs), armed)
			if c := a.Char(k); res.Sent && c != nil {
				extra = append(extra, Sent{Key: k, Seen: seenAfterSend(c.Lines, res.Echoed)})
			}
			if _, ok := l.onDisk[k]; ok && a.Draft(k) == "" {
				l.saveDrafts() // sent: it mustn't come back as a draft
			}
		case "older": //str:ok
			effs = a.HistoryOpen() // ↑ on the first row: the history panel
		}
		if act.Do != "input" { //str:ok
			l.f.Resend(k) // the page waits to hear the action was taken, changed or not
		}
	case "password": //str:ok
		// The password form's submit: the password prompt's answer, and
		// only that.
		if c := a.Char(act.Key); c != nil && c.NeedPW && act.Key == l.inputKey() {
			a.SetInput(act.Text)
			l.f.Told(act.Key, act.N)
			_, effs = a.Submit()
			l.f.Resend(act.Key)
		}
	case "history_move": //str:ok
		effs = a.HistoryMove(act.N)
	case "history_pick": //str:ok
		// A tap on line N, or Enter on the selected one (N < 0).
		if act.N >= 0 {
			effs = a.HistorySelect(act.N)
		}
		a.HistoryPick()
	case "history_more": //str:ok
		effs = a.HistoryMore()
	case "history_close": //str:ok
		a.HistoryClose()
	case "history_toggle": //str:ok
		effs = a.HistoryToggle()
	case "cancel": //str:ok
		a.Unconfirm()
		a.SkipLogin()
	case "here": //str:ok
		a.Here()
	case "away": //str:ok
		a.SetAway()
	case "scrolled": //str:ok
		a.Scrolled()
	case "focus": //str:ok
		a.Focus(act.In)
	case "page": //str:ok
		effs = a.RequestOlder(act.Key)
	case "restore": //str:ok
		l.restore(act.Data)
	case "answer_save": //str:ok
		a.AnswerSave(act.Save)
	case "resize": //str:ok
		a.SetPane(act.Cols, act.Rows)
		a.ReportSize()
	case "all": //str:ok
		all = true
	case "since": //str:ok
		if ls, ok := l.f.Since(act.Key, act.Epoch, act.Seq); ok {
			extra = append(extra, feed.Lines{Key: act.Key, Epoch: act.Epoch, Op: feed.Append, Lines: ls})
		} else {
			all = true
		}
	case "quit": //str:ok
		effs = a.Quit()
	case "say": //str:ok
		a.SetStatus(act.Err, act.Text)
	default:
		var ok bool
		if extra, effs, ok = l.doLog(act); !ok {
			a.SetStatus(true, str.StatusBadAction(fmt.Errorf("%q", act.Do)))
		}
	}
	return extra, effs, all
}

// Sent says a line went out for Key: the page shows the end, as the
// TUI's scrollback does on Enter (ToBottomKeeping), with every line up
// to Seen seen. An echoed line is left unseen, so if what comes next
// fills the pane the pager stops with it at the top.
type Sent struct {
	Key  string
	Seen int
}

// seenAfterSend is the Seq of the last line seen after a send: the
// newest, or with echoed the one before it; -1 for none.
func seenAfterSend(ls []*app.Line, echoed bool) int {
	n := len(ls)
	if echoed {
		n--
	}
	if n <= 0 {
		return -1
	}
	return ls[n-1].Seq
}

// backup does /backup when Run was given a way to: the page gets the
// file to download, or the status line says why there's none.
func (l *Loop) backup(effs []app.Effect) []app.Effect {
	if l.opts.Backup == nil {
		return effs
	}
	out := effs[:0:0]
	for _, e := range effs {
		if d, ok := e.(app.Do); ok && d.Cmd == "/backup" { //str:ok
			data, err := l.opts.Backup()
			if err != nil {
				l.a.SetStatus(true, str.StatusBackupFailed(err))
				continue
			}
			e = app.SaveFile{Name: str.BackupFileName(l.a.Now().Format("2006-01-02")), Data: data} //str:ok
		}
		out = append(out, e)
	}
	return out
}

// reset passes /reset on to the page (which wipes what it saved and
// reloads) only on Enter again, armed: the first keeps /reset in the
// input and says what it does.
func (l *Loop) reset(effs []app.Effect, armed bool) []app.Effect {
	out := effs[:0:0]
	for _, e := range effs {
		if d, ok := e.(app.Do); ok && d.Cmd == "/reset" && !armed { //str:ok
			l.resetArmed = true
			l.a.SetInput(d.Cmd)
			l.a.SetStatus(true, str.StatusResetConfirm())
			continue
		}
		out = append(out, e)
	}
	return out
}

// pageOwn reports whether act is one the page sends by itself, not the
// person: presence, focus, size, scrolling, catching up, and an input
// that changes nothing (text is the input's now).
func pageOwn(act Action, text string) bool {
	switch act.Do {
	case "here", "away", "focus", "resize", "scrolled", "since", "all", "page": //str:ok
		return true
	case "input": //str:ok
		return act.Text == text
	}
	return false
}

// restore puts back the backup the page read for /restore, then reads
// the config again; Reload says why when it can't.
func (l *Loop) restore(data []byte) {
	if l.opts.Restore == nil {
		return
	}
	n, err := l.opts.Restore(data)
	switch {
	case err != nil:
		l.a.SetStatus(true, str.StatusRestoreFailed(err))
	case n > 0:
		if _, ok := l.a.Reload(); ok {
			l.a.SetStatus(false, str.StatusRestored(n))
		}
	}
}

// inputKey names the active input: the active character's key, or "" for
// the idle input (a world's overview, or nothing open).
func (l *Loop) inputKey() string {
	if k := l.a.Active(); l.a.Char(k) != nil {
		return k
	}
	return ""
}

// doLog carries out log-mode, filter and settings actions; ok is false
// for one it doesn't know. A log-mode action for a character without log
// mode open does nothing, and a line that isn't loaded is no line.
func (l *Loop) doLog(act Action) (extra []any, effs []app.Effect, ok bool) {
	a := l.a
	if extra, ok := l.doSettings(act); ok {
		return extra, nil, true
	}
	switch act.Do {
	case "log": //str:ok
		// Opened, or shown again: what the status said was about before,
		// as the TUI's openBrowse.
		a.OpenLog(act.Key)
		a.ClearStatus()
		return nil, nil, true
	case "log_close": //str:ok
		a.CloseLog(act.Key)
		return nil, nil, true
	}
	var g *app.Log
	if c := a.Char(act.Key); c != nil {
		g = c.Log
	}
	at := func(seq int) *app.LogLine { return lineAt(g, seq) }
	switch act.Do {
	case "log_older", "mark", "range", "extend", "exclude", "find", "find_older", "find_newer", "stop_search", //str:ok
		"goto", "tags", "export_name", "copy", "export", "hide", "only", "fold", "add_text", "remove_text": //str:ok
		if g == nil {
			return nil, nil, true
		}
		if act.Do != "log_older" { //str:ok
			l.clearLogStatus() // you've acted again (scrolling to older history isn't acting)
		}
	default:
		return nil, nil, false
	}
	switch act.Do {
	case "log_older": //str:ok
		effs = g.Older(nil)
	case "mark": //str:ok
		g.Mark(at(act.Seq))
	case "range": //str:ok
		if s, e := at(act.Start), at(act.End); s != nil && e != nil {
			g.NewRange(s, e)
			g.RangeStatus()
		}
	case "extend": //str:ok
		if ln := at(act.Seq); ln != nil && g.Start != nil {
			g.ExtendTo(ln)
		}
	case "exclude": //str:ok
		g.ToggleExclude(at(act.Seq))
	case "find": //str:ok
		effs = g.SetFind(act.Term, at(act.Seq), l.land(act.Key))
	case "find_older": //str:ok
		effs = g.FindOlder(at(act.Seq), false, l.land(act.Key))
	case "find_newer": //str:ok
		if ln := g.FindNewer(at(act.Seq)); ln != nil {
			l.land(act.Key)(ln)
		}
	case "stop_search": //str:ok
		g.StopSearch()
	case "goto": //str:ok
		effs = g.GotoTime(act.When, l.land(act.Key))
	case "tags": //str:ok
		if ln := at(act.Seq); ln != nil {
			styled := func(string) (string, bool) { return "", false }
			if hl := l.lk.highlighter(act.Key); hl != nil {
				styled = hl.Styled
			}
			g.SetStatus(false, app.LineTags(ln.Tags, styled))
		}
	case "export_name": //str:ok
		if g.ExportReady() {
			if name, ok := g.ExportFileName(act.Format, ""); ok {
				extra = append(extra, ExportName{Key: act.Key, Format: act.Format, Name: name})
			}
		}
	case "copy": //str:ok
		effs = g.Copy()
	case "export": //str:ok
		effs = g.Export(act.Format, act.Name)
	case "hide", "only": //str:ok
		if act.Item != nil {
			if act.Do == "hide" { //str:ok
				g.ToggleHide(*act.Item)
			} else {
				g.PressOnly(*act.Item)
			}
			g.Refilter()
		}
	case "fold": //str:ok
		g.Fold(act.Tag, act.Shut)
	case "add_text": //str:ok
		g.AddText(act.Term)
		g.Refilter()
	case "remove_text": //str:ok
		g.RemoveText(act.Term)
		g.Refilter()
	}
	return extra, effs, true
}

// lineAt is g's loaded line numbered seq; nil if there's none (or no g).
func lineAt(g *app.Log, seq int) *app.LogLine {
	if g == nil {
		return nil
	}
	for _, l := range g.Lines {
		if l.Seq == seq {
			return l
		}
	}
	return nil
}

// doSettings carries out a settings action: Saved (after reloading the
// config, as the TUI does after an edit) or Problems; or for settings,
// what an editor starts from. ok is false for an
// action that isn't one.
func (l *Loop) doSettings(act Action) (extra []any, ok bool) {
	if act.Do == "settings" { //str:ok
		return l.settings(act.World, act.Char), true
	}
	a := l.a
	var err error
	key, saved := "", "" // the new character's key; the status to show once reloaded
	reload := true
	// answer is the action's answer, for the editor that sent it: Saved
	// with no problems, else Problems.
	answer := func(p []Problem) []any {
		if p == nil {
			return []any{Saved{Op: act.Do, Key: key, World: act.World, Char: act.Char}}
		}
		return []any{Problems{Op: act.Do, World: act.World, Char: act.Char, Problems: p}}
	}
	switch act.Do {
	case "add_world": //str:ok
		if act.WorldSettings == nil {
			act.WorldSettings = &config.WorldSettings{}
		}
		err = a.AddWorld(act.ID, *act.WorldSettings)
	case "save_world": //str:ok
		if act.WorldSettings == nil {
			act.WorldSettings = &config.WorldSettings{}
		}
		err = a.SaveWorld(act.World, *act.WorldSettings)
		saved = str.EditorSaved(act.World)
	case "add_character": //str:ok
		var id string
		if id, err = a.AddCharacter(act.World, act.Name); err == nil {
			key = app.Key(act.World, id)
		}
	case "save_character": //str:ok
		if act.CharSettings == nil {
			act.CharSettings = &config.CharacterSettings{}
		}
		err = a.SaveCharacter(act.World, act.Char, *act.CharSettings)
		saved = str.EditorSaved(act.World + "/" + act.Char)
	case "delete_world": //str:ok
		err = a.DeleteWorld(act.World)
		saved = str.EditorDeleted(act.World)
	case "delete_character": //str:ok
		var pwErr error
		what := act.World + "/" + act.Char
		if pwErr, err = a.DeleteCharacter(act.World, act.Char); err == nil {
			if saved = str.EditorDeleted(what); pwErr != nil {
				saved = "" // the kept password's message stands instead
				a.SetStatus(true, str.EditorDeletedPasswordKept(what, pwErr))
			}
		}
	case "forget_password": //str:ok
		if !a.CanForgetPasswords() {
			return answer([]Problem{}), true // nothing to forget here; the page shouldn't offer it
		}
		if err = a.ForgetPassword(act.World, act.Char); err == nil {
			a.SetStatus(false, str.EditorForgotPassword(act.World, act.Char))
			reload = false
		}
	default:
		return nil, false
	}
	if err != nil {
		p := []Problem{}
		for _, e := range app.Problems(err) {
			pr := Problem{Msg: e.Error()}
			var fe *app.FieldError
			if errors.As(e, &fe) {
				pr.Field = fe.Field
			}
			p = append(p, pr)
		}
		return answer(p), true
	}
	if reload {
		res, ok := a.Reload()
		if !ok { // Reload said why on the status line
			return answer([]Problem{{Msg: a.Status().Text}}), true
		}
		for _, k := range a.Order() { // in order, so which error shows is settled
			if c := a.Char(k); c != nil && !c.Orphan && res.Errs[k] != nil {
				a.SetStatus(true, str.StatusCharError(k, res.Errs[k]))
				saved = ""
			}
		}
		if saved != "" {
			a.SetStatus(false, saved)
		}
	}
	return answer(nil), true
}
