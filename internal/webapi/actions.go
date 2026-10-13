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
	Do                                                 string
	Key, Text, Term, Tag, Day, Format, Name, ID, World string
	Char                                               string
	Seq, Start, End, Epoch, Cols, Rows                 int
	N                                                  int // numbers input, submit, older and newer (see feed.Input.Ack); step's distance; next_unread's direction
	In, Shut, Save                                     bool
	Item                                               *scene.Item
	WorldSettings                                      *config.WorldSettings
	CharSettings                                       *config.CharacterSettings
}

// do carries out act. See step for what it returns.
func (l *Loop) do(act Action) (extra []any, effs []app.Effect, all bool) {
	a := l.a
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
	case "step": //str:ok
		if k := a.StepTarget(act.N); k != "" {
			a.Switch(k)
		}
	case "next_unread": //str:ok
		if k := a.UnreadTarget(act.N); k != "" {
			a.Switch(k)
		}
	case "input", "submit", "older", "newer": //str:ok
		k := l.inputKey()
		if act.Key != k {
			break // the page hadn't heard the active input changed: drop it rather than send it somewhere else
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
			_, effs = a.Submit()
			if _, ok := l.onDisk[k]; ok && a.Draft(k) == "" {
				l.saveDrafts() // sent: it mustn't come back as a draft
			}
		case "older": //str:ok
			a.HistoryOlder(act.Text)
		case "newer": //str:ok
			a.HistoryNewer()
		}
		if act.Do != "input" { //str:ok
			l.f.Resend(k) // the page waits to hear the action was taken, changed or not
		}
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
	default:
		var ok bool
		if extra, effs, ok = l.doLog(act); !ok {
			a.SetStatus(true, str.StatusBadAction(fmt.Errorf("%q", act.Do)))
		}
	}
	return extra, effs, all
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
		a.OpenLog(act.Key)
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
	case "log_older", "mark", "range", "extend", "exclude", "find", "find_older", "stop_search", //str:ok
		"goto_date", "copy", "export", "hide", "only", "fold", "add_text", "remove_text": //str:ok
		if g == nil {
			return nil, nil, true
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
		effs = g.SetFind(act.Term, at(act.Seq), nil)
	case "find_older": //str:ok
		effs = g.FindOlder(at(act.Seq), false, nil)
	case "stop_search": //str:ok
		g.StopSearch()
	case "goto_date": //str:ok
		effs = g.GotoDate(act.Day, nil)
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
	return nil, effs, true
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
// config, as the TUI does after an edit) or Problems. ok is false for an
// action that isn't one.
func (l *Loop) doSettings(act Action) (extra []any, ok bool) {
	a := l.a
	var err error
	key, saved := "", "" // the new character's key; the status to show once reloaded
	reload := true
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
			return []any{Problems{Op: act.Do}}, true // nothing to forget here; the page shouldn't offer it
		}
		if err = a.ForgetPassword(act.World, act.Char); err == nil {
			a.SetStatus(false, str.EditorForgotPassword(act.World, act.Char))
			reload = false
		}
	default:
		return nil, false
	}
	if err != nil {
		p := Problems{Op: act.Do}
		for _, e := range app.Problems(err) {
			pr := Problem{Msg: e.Error()}
			var fe *app.FieldError
			if errors.As(e, &fe) {
				pr.Field = fe.Field
			}
			p.Problems = append(p.Problems, pr)
		}
		return []any{p}, true
	}
	if reload {
		res, ok := a.Reload()
		if !ok { // Reload said why on the status line
			return []any{Problems{Op: act.Do, Problems: []Problem{{Msg: a.Status().Text}}}}, true
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
	return []any{Saved{Op: act.Do, Key: key}}, true
}
