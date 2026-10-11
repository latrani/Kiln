package webapi

import (
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
		if c, err := a.Open(act.Key); c != nil && err != nil {
			a.SetStatus(true, str.StatusCharError(act.Key, err))
		}
	case "close": //str:ok
		if a.Char(act.Key) != nil {
			a.Close(act.Key)
		}
	case "switch": //str:ok
		a.Switch(act.Key)
	case "input": //str:ok
		if act.Key == a.Active() || act.Key == "" && a.Char(a.Active()) == nil {
			l.told(act.Text)
		}
	case "submit": //str:ok
		l.told(act.Text)
		_, effs = a.Submit()
	case "older": //str:ok
		l.told(act.Text)
		a.HistoryOlder(act.Text)
	case "newer": //str:ok
		l.told(act.Text)
		a.HistoryNewer()
	case "cancel": //str:ok
		a.Unconfirm()
		a.SkipLogin()
	case "here": //str:ok
		a.Here()
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

// told sets the active input to text the page sent, and tells the feed
// it needn't go back.
func (l *Loop) told(text string) {
	l.a.SetInput(text)
	k := l.a.Active()
	if l.a.Char(k) == nil { // a world's overview, or nothing: the idle input
		k = ""
	}
	l.f.Told(k)
}

// doLog carries out log-mode, filter and settings actions; ok is false
// for one it doesn't know.
func (l *Loop) doLog(act Action) (extra []any, effs []app.Effect, ok bool) {
	return nil, nil, false
}
