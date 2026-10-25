package app

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/latrani/Kiln/internal/classify"
	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
)

// LogInitialLines is how much history log mode loads up front (whole
// days, newest first, until at least this many lines).
const LogInitialLines = 200

// dedupeTail is how many of the newest loaded lines appendLive checks for
// a copy of an incoming line.
const dedupeTail = 256

// LogLine is a logged line in log mode, with what its filters read.
type LogLine struct {
	Line
	Tags  []string // Line.TagNames, kept for the filters
	Lower string   // Line.Plain, lowercased, for text filters
}

func makeLogLine(r Rules, e logstore.Entry) *LogLine {
	l := r.Line(e)
	return &LogLine{Line: l, Tags: l.TagNames(), Lower: strings.ToLower(l.Plain())}
}

// Log is one character's log mode. Lines are referenced by pointer so
// paging in older history never disturbs marks or exclusions. Front ends
// read its fields and set Shown; only its methods change the rest.
type Log struct {
	Lines      []*LogLine // oldest first
	Start, End *LogLine   // the marked range, both set or both nil
	Marking    bool       // the range is live: its far end follows the cursor; see Mark
	Excluded   map[*LogLine]bool
	Find       string // the find term while find is on; "" when it's off
	LastFind   string // the last term find had, kept when it ends, for the next find to start with
	Status     string // log mode's message; the front end shows it and clears it
	StatusErr  bool
	HistDone   bool // every log day is loaded (or there is no history)
	Loading    bool // an older day is being read
	Searching  bool // a find is reading older days for a match
	// Shown is a line's text as the front end shows it, unstyled: what
	// find matches. nil: Line.Plain.
	Shown func(*LogLine) string

	a           *App
	c           *Char
	hist        *history.Reader
	anchor      *LogLine        // the line m was pressed on, while Marking
	loadedTo    time.Time       // newest logged time at open, at log (millisecond) precision
	pending     func() []Effect // what to do when the day being read arrives; see Older
	tagNames    []string        // the distinct tags on Lines[:tagNamesN]; see Items
	tagNamesN   int
	tagNamesGen int       // c.rulesGen when tagNames was worked out
	seq         numbering // Lines' numbers
}

// LogOlderMsg is one older day for log mode, read off the loop. It goes
// to HandleLogOlder.
type LogOlderMsg struct {
	Key   string
	Log   *Log // the log that asked; stale if it has since closed
	Lines []*LogLine
	Done  bool // history is now exhausted
	Err   error
	gen   int // the character's rulesGen when the read started
}

// OpenLog opens log mode for open character k over its logs, loading
// whole days until at least LogInitialLines, and returns it; an open log
// is returned as it is. nil if k isn't open.
func (a *App) OpenLog(k string) *Log {
	c := a.chars[k]
	if c == nil {
		return nil
	}
	if c.Log != nil {
		return c.Log
	}
	g := &Log{a: a, c: c, Excluded: map[*LogLine]bool{}}
	if l, ok := a.LogLayout(c.Ch); ok {
		if h, err := history.NewReader(l); err == nil {
			g.hist = h
		}
	}
	g.HistDone = g.hist == nil || g.hist.Exhausted()
	for len(g.Lines) < LogInitialLines && g.loadOlder() {
	}
	if l := g.Last(); l != nil {
		g.loadedTo = l.Entry.Time
	}
	c.Filter.ResetSeen() // this session's items light by the filter as it stands
	c.Log = g
	return g
}

// CloseLog closes k's log mode. A day it was reading arrives to nothing.
func (a *App) CloseLog(k string) {
	if c := a.chars[k]; c != nil {
		c.Log = nil
	}
}

// readLogOlder reads and makes the next older day. Only one call may run
// at a time, and nothing else may touch h meanwhile.
func readLogOlder(h *history.Reader, r Rules) LogOlderMsg {
	es, _, _, err := h.LoadOlder()
	msg := LogOlderMsg{Done: h.Exhausted(), Err: err}
	for _, e := range es {
		msg.Lines = append(msg.Lines, makeLogLine(r, e))
	}
	return msg
}

// loadOlder synchronously prepends the next older day; only OpenLog uses
// it, for the bounded initial load. It reports false when there is
// nothing more to load.
func (g *Log) loadOlder() bool {
	if g.HistDone {
		return false
	}
	g.prepend(readLogOlder(g.hist, g.c.Rules))
	return true
}

func (g *Log) prepend(msg LogOlderMsg) {
	if msg.Err != nil {
		g.SetStatus(true, str.BrowseReadingLogs(msg.Err))
	}
	g.HistDone = msg.Done
	for i := len(msg.Lines) - 1; i >= 0; i-- {
		msg.Lines[i].Seq = g.seq.older()
	}
	g.Lines = append(msg.Lines, g.Lines...)
}

// Older starts reading the next older day off the loop, so a big log
// never stalls it. then runs when it arrives (it may ask again to keep
// paging). While a read is in flight, a newer request just replaces then.
// Any request stops a find waiting on older days. It returns nothing if
// history is exhausted.
func (g *Log) Older(then func() []Effect) []Effect {
	if g.HistDone {
		return nil
	}
	g.pending = then
	g.Searching = false // a find waiting on older days gives way to whatever asked now
	if g.Loading {
		return nil
	}
	g.Loading = true
	h, r, gen, k := g.hist, g.c.Rules, g.c.rulesGen, g.c.Key
	return []Effect{Run{Func: func() any {
		msg := readLogOlder(h, r)
		msg.Key, msg.Log, msg.gen = k, g, gen
		return msg
	}}}
}

// HandleLogOlder prepends a day read by Older and runs what was waiting.
// A day for a log that has since closed arrives to nothing.
func (a *App) HandleLogOlder(msg LogOlderMsg) []Effect {
	c := a.chars[msg.Key]
	if c == nil || c.Log == nil || c.Log != msg.Log {
		return nil
	}
	g := c.Log
	g.Loading = false
	if msg.gen != c.rulesGen { // read under rules since replaced
		for _, l := range msg.Lines {
			l.Line = c.Rules.Reline(l.Line)
			l.Tags = l.TagNames()
		}
	}
	g.prepend(msg)
	then := g.pending
	g.pending = nil
	if then != nil {
		return then()
	}
	return nil
}

// appendLive adds a line that arrived while the log is open; a front
// end following the newest line moves itself. Lines
// logged before it opened can still arrive as events afterwards; those
// are already loaded from the log (at millisecond precision), so a line
// no newer than the load watermark that matches a recently loaded one is
// skipped.
func (g *Log) appendLive(e logstore.Entry) {
	t := e.Time.Truncate(time.Millisecond)
	if !g.loadedTo.IsZero() && !t.After(g.loadedTo) {
		for _, l := range g.Lines[max(0, len(g.Lines)-dedupeTail):] {
			if l.Entry.Text == e.Text && l.Entry.Dir == e.Dir && l.Entry.Time.Truncate(time.Millisecond).Equal(t) {
				return
			}
		}
	}
	l := makeLogLine(g.c.Rules, e)
	l.Seq = g.seq.newer()
	g.Lines = append(g.Lines, l)
}

// Last is the newest loaded line; nil with none.
func (g *Log) Last() *LogLine {
	if len(g.Lines) == 0 {
		return nil
	}
	return g.Lines[len(g.Lines)-1]
}

// SetStatus puts msg on log mode's status, as an error if isErr.
func (g *Log) SetStatus(isErr bool, msg string) { g.Status, g.StatusErr = msg, isErr }

// ClearStatus takes log mode's message down.
func (g *Log) ClearStatus() { g.Status = "" }

// Shows reports whether l is drawn: whatever the filter lets through.
func (g *Log) Shows(l *LogLine) bool { return g.c.Filter.Visible(l.Tags, l.Lower) }

// sentItem is the filter's row for sent lines. It's hidden by default
// unless local_echo is on.
var sentItem = scene.Item{Name: classify.SentTag}

// Visible is the shown lines, oldest first.
func (g *Log) Visible() []*LogLine {
	out := make([]*LogLine, 0, len(g.Lines))
	for _, l := range g.Lines {
		if g.Shows(l) {
			out = append(out, l)
		}
	}
	return out
}

// LastVisible is the newest shown line; nil with none.
func (g *Log) LastVisible() *LogLine {
	v := g.Visible()
	if len(v) == 0 {
		return nil
	}
	return v[len(v)-1]
}

// Index is l's position in Lines; -1 if it isn't there.
func (g *Log) Index(l *LogLine) int { return slices.Index(g.Lines, l) }

// InRange reports whether l is inside the marked range.
func (g *Log) InRange(l *LogLine) bool {
	if g.Start == nil {
		return false
	}
	i := g.Index(l)
	return i >= g.Index(g.Start) && i <= g.Index(g.End)
}

// Items is the filter panel's list: Categories, then every category on
// a loaded line or with a filter set (with their parents), the same for
// Flags, then the text rows. A group is listed while it has a tag under
// it or a filter set. It syncs the filter, so a tag seen for the first
// time obeys the filter already set.
func (g *Log) Items() []scene.Item {
	flag := g.c.Ch.Rules.IsFlag
	if g.tagNamesN != len(g.Lines) || g.tagNamesGen != g.c.rulesGen { // lines are only ever added
		seen := map[string]bool{}
		g.tagNames = nil
		for _, l := range g.Lines {
			for _, t := range l.Tags {
				if !seen[t] {
					seen[t] = true
					g.tagNames = append(g.tagNames, t)
				}
			}
		}
		g.tagNamesN, g.tagNamesGen = len(g.Lines), g.c.rulesGen
	}
	f := &g.c.Filter
	only, hasOnly := f.Only()
	tags := append(slices.Clone(g.tagNames), f.Tags()...)
	cats, flags := slices.Clone(tags), slices.Clone(tags)
	cats = slices.DeleteFunc(cats, flag)
	flags = slices.DeleteFunc(flags, func(t string) bool { return !flag(t) })
	var items []scene.Item
	for _, grp := range []struct {
		name string
		tags []string
	}{{scene.CategoryGroup, cats}, {scene.FlagGroup, flags}} {
		it := scene.Item{Group: grp.name}
		if len(grp.tags) > 0 || f.Hidden(it) || hasOnly && only == it {
			items = append(items, it)
			items = append(items, scene.TagItems(grp.tags)...)
		}
	}
	items = append(items, f.Texts()...)
	f.Sync(items)
	return items
}

// Refilter syncs the filter with the items after it changes, so a tag
// seen for the first time obeys it. Keeping the reader's place is the
// front end's: see NearestVisible.
func (g *Log) Refilter() { g.Items() }

// NearestVisible returns the first shown line after l, or failing that
// the last shown line before it, so hiding l keeps the reader's place.
func (g *Log) NearestVisible(l *LogLine) *LogLine {
	i := g.Index(l)
	for j := i + 1; j < len(g.Lines); j++ {
		if g.Shows(g.Lines[j]) {
			return g.Lines[j]
		}
	}
	for j := i - 1; j >= 0; j-- {
		if g.Shows(g.Lines[j]) {
			return g.Lines[j]
		}
	}
	return nil
}

// Mark is m on l, the cursor's line. With no live range it starts one
// there: l is its anchor, and MarkTo moves its other end. With one, it
// takes the range to l and locks it. A nil l starts nothing.
func (g *Log) Mark(l *LogLine) {
	if g.Marking {
		g.MarkTo(l)
		g.lock()
		g.RangeStatus()
		return
	}
	if l == nil {
		return
	}
	g.NewRange(l, l)
	g.Marking, g.anchor = true, l
	g.SetStatus(false, str.BrowseRangeStart())
}

// MarkTo moves a live range's far end to l, the cursor's line: the range
// runs from the anchor to l, either way round. Exclusions it no longer
// covers are forgotten, so growing back over them doesn't bring them
// back. Nothing when the range isn't live.
func (g *Log) MarkTo(l *LogLine) {
	if !g.Marking || l == nil {
		return
	}
	g.Start, g.End = g.anchor, l
	if g.Index(l) < g.Index(g.anchor) {
		g.Start, g.End = l, g.anchor
	}
	for x := range g.Excluded {
		if !g.InRange(x) {
			delete(g.Excluded, x)
		}
	}
}

// GrowTo takes l into a live range, as a click outside it while
// marking: the end away from l becomes the anchor and l the far end, so
// the range keeps what it had and the cursor moves on from l.
func (g *Log) GrowTo(l *LogLine) {
	if !g.Marking || l == nil {
		return
	}
	g.anchor = g.Start
	if g.Index(l) < g.Index(g.Start) {
		g.anchor = g.End
	}
	g.MarkTo(l)
}

// EndMark drops a live range: Esc while marking.
func (g *Log) EndMark() {
	g.Start, g.End = nil, nil
	g.lock()
	clear(g.Excluded)
}

// lock stops a live range following the cursor, leaving it where it is.
func (g *Log) lock() { g.Marking, g.anchor = false, nil }

// NewRange starts a range from start to end, either way round, dropping
// exclusions left from an earlier one so they can't resurface if it
// grows over them. A live range locks: this is a range of its own.
func (g *Log) NewRange(start, end *LogLine) {
	if g.Index(end) < g.Index(start) {
		start, end = end, start
	}
	g.Start, g.End = start, end
	g.lock()
	clear(g.Excluded)
}

// RangeStatus says how many lines the range holds.
func (g *Log) RangeStatus() {
	g.SetStatus(false, str.BrowseLinesInRange(g.Index(g.End)-g.Index(g.Start)+1))
}

// ToggleExclude takes l out of the range, or puts it back.
func (g *Log) ToggleExclude(l *LogLine) {
	if l == nil || !g.InRange(l) {
		g.SetStatus(true, str.BrowseExcludeNeedsRange())
		return
	}
	g.Excluded[l] = !g.Excluded[l]
}

// Selection is what an export contains: the lines inside the range, not
// excluded and not hidden by the filter, so an export is what log mode
// shows of it.
func (g *Log) Selection() []logstore.Entry {
	if g.Start == nil {
		return nil
	}
	var out []logstore.Entry
	for _, l := range g.Lines[g.Index(g.Start) : g.Index(g.End)+1] {
		if !g.Excluded[l] && g.Shows(l) {
			out = append(out, l.Entry)
		}
	}
	return out
}

// Title is an export's title: world, character, and the range's day.
func (g *Log) Title() string {
	ch := g.c.Ch
	when := ""
	if g.Start != nil {
		when = " — " + g.Start.Entry.Time.Local().Format(str.DateDayYear())
	}
	return ch.World + " " + ch.Name + when
}

// Copy puts the selection on the clipboard as plain text, or says to
// mark a range first. A live range locks.
func (g *Log) Copy() []Effect {
	g.lock()
	sel := g.Selection()
	if len(sel) == 0 {
		g.SetStatus(true, str.BrowseMarkRange())
		return nil
	}
	g.SetStatus(false, str.BrowseCopied(len(sel)))
	return []Effect{Copy{Text: scene.Plain(sel)}}
}

// ExportReady reports whether there's a selection to export, and if not
// says to mark a range first. A live range locks.
func (g *Log) ExportReady() bool {
	g.lock()
	if len(g.Selection()) == 0 {
		g.SetStatus(true, str.BrowseMarkRange())
		return false
	}
	return true
}

// ExportFileName is the file name an export in format starts with, from
// export_name, in dir ("" for a download: just a name). false, saying
// so, if nothing's left to export.
func (g *Log) ExportFileName(format, dir string) (string, bool) {
	sel := g.Selection()
	if len(sel) == 0 {
		g.SetStatus(true, str.BrowseNothingLeft())
		return "", false
	}
	return scene.FileName(dir, g.a.cfg.ExportName, sel[0].Time.Local(), g.c.Ch.World, g.c.Ch.Name, format), true
}

// Export renders the selection in format and offers it as a file called
// name.
func (g *Log) Export(format, name string) []Effect {
	name = strings.TrimSpace(name)
	if name == "" {
		g.SetStatus(true, str.BrowseNoFileName())
		return nil
	}
	sel := g.Selection()
	if len(sel) == 0 {
		g.SetStatus(true, str.BrowseNothingToExport())
		return nil
	}
	return []Effect{SaveFile{Key: g.c.Key, Name: name, Data: []byte(scene.Render(format, sel, g.Title()))}}
}

// FindRE matches a find term literally and case-insensitively. Match
// offsets always index the original text: lowercasing a copy and reusing
// its offsets breaks on letters whose case forms differ in byte length.
func FindRE(term string) *regexp.Regexp {
	return regexp.MustCompile("(?i)" + regexp.QuoteMeta(term))
}

// LineTags says which tags a line has and, for each one without a style
// of its own, whose style it takes (styled is the character's
// highlighter's Styled): what log mode's statusline shows for the cursor
// line. "" for a line with none.
func LineTags(tags []string, styled func(tag string) (string, bool)) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, len(tags))
	for i, tag := range tags {
		switch s, ok := styled(tag); {
		case !ok:
			parts[i] = str.BrowseTagUnstyled(tag)
		case s != tag:
			parts[i] = str.BrowseTagAs(tag, s)
		default:
			parts[i] = tag
		}
	}
	return str.BrowseLineTags(strings.Join(parts, str.ListSeparator()))
}

func (g *Log) shown(l *LogLine) string {
	if g.Shown != nil {
		return g.Shown(l)
	}
	return l.Plain()
}

// Matches is the shown lines the find term matches, oldest first.
func (g *Log) Matches() []*LogLine {
	if g.Find == "" {
		return nil
	}
	re := FindRE(g.Find)
	var out []*LogLine
	for _, l := range g.Visible() {
		if re.MatchString(g.shown(l)) {
			out = append(out, l)
		}
	}
	return out
}

// matchAt reports whether l is a find match that's shown.
func (g *Log) matchAt(re *regexp.Regexp, l *LogLine) bool {
	return g.Shows(l) && re.MatchString(g.shown(l))
}

// SetFind turns find on with term and finds its nearest match at from
// or older (see FindOlder); an empty term turns it off.
func (g *Log) SetFind(term string, from *LogLine, found func(*LogLine)) []Effect {
	if term == "" {
		g.EndFind()
		return nil
	}
	g.Find, g.LastFind = term, term
	return g.FindOlder(from, true, found)
}

// EndFind turns find off: no matches marked, n and N do nothing. Its
// term stays as LastFind, for the next find to start with.
func (g *Log) EndFind() {
	g.Searching, g.pending = false, nil
	g.Find = ""
}

// FindOlder finds the nearest match older than from (or at it, with
// includeFrom; from the newest line if from is nil), the way find goes:
// newest first, and hands it to found. It reads in older days until one
// turns up or history runs out; it doesn't wrap.
func (g *Log) FindOlder(from *LogLine, includeFrom bool, found func(*LogLine)) []Effect {
	if g.Find == "" {
		return nil
	}
	i := len(g.Lines) - 1
	if fi := g.Index(from); fi >= 0 {
		i = fi
		if !includeFrom {
			i--
		}
	}
	return g.findOlderFrom(i, found)
}

// findOlderFrom looks for a match at line i or older; see FindOlder.
func (g *Log) findOlderFrom(i int, found func(*LogLine)) []Effect {
	re := FindRE(g.Find)
	g.Searching = false
	for ; i >= 0; i-- {
		if l := g.Lines[i]; g.matchAt(re, l) {
			g.Status = ""
			if found != nil {
				found(l)
			}
			return nil
		}
	}
	if !g.HistDone {
		var oldest *LogLine // where this pass stopped; the next goes on below it
		if len(g.Lines) > 0 {
			oldest = g.Lines[0]
		}
		effs := g.Older(func() []Effect { return g.findOlderFrom(g.Index(oldest)-1, found) })
		g.Searching = true
		return effs
	}
	g.findFailed(str.BrowseNoOlderMatches(g.Find))
	return nil
}

// searchedTo is the oldest day loaded, which a search has read through.
func (g *Log) searchedTo() string {
	if len(g.Lines) == 0 {
		return ""
	}
	return DayLabel(g.Lines[0].Day)
}

// StopSearch ends a find that's reading older days, finding nothing,
// and reports whether there was one. The day being read still arrives,
// to nothing.
func (g *Log) StopSearch() bool {
	if !g.Searching {
		return false
	}
	g.Searching, g.pending = false, nil
	g.SetStatus(false, str.BrowseSearchCancelled())
	return true
}

// FindNewer returns the nearest match newer than from (from the oldest
// line if from is nil). Everything newer is always loaded; it doesn't
// wrap. nil, saying so, if there's none.
func (g *Log) FindNewer(from *LogLine) *LogLine {
	if g.Find == "" {
		return nil
	}
	re := FindRE(g.Find)
	for _, l := range g.Lines[g.Index(from)+1:] {
		if g.matchAt(re, l) {
			g.Status = ""
			return l
		}
	}
	g.findFailed(str.BrowseNoNewerMatches(g.Find))
	return nil
}

// findFailed says there's nothing further: that the term matches nothing
// at all, if so, else msg.
func (g *Log) findFailed(msg string) {
	if len(g.Matches()) == 0 && g.HistDone {
		msg = str.BrowseNoMatches(g.Find)
	}
	g.SetStatus(true, msg)
}

// matchPos is at's match counted from the newest, of n loaded; more says
// older history not yet loaded may hold others.
func (g *Log) matchPos(at *LogLine) (i, n int, more bool) {
	ms := g.Matches()
	if k := slices.Index(ms, at); k >= 0 {
		i = len(ms) - k
	}
	return i, len(ms), !g.HistDone
}

// FindStatus is the find term and at's place among the matches, with
// the keys that step through them, for the statusline; "" with find off.
func (g *Log) FindStatus(at *LogLine) string {
	if g.Find == "" {
		return ""
	}
	if g.Searching {
		return str.BrowseSearching(g.Find, g.searchedTo())
	}
	i, n, more := g.matchPos(at)
	if more {
		return str.BrowseFindStatusMore(g.Find, i, n)
	}
	return str.BrowseFindStatus(g.Find, i, n)
}

// Go-to's forms, in Go's reference-time notation: a day and a time of
// day, as log mode draws them, or a day alone.
const (
	whenLayout = "2006-01-02 15:04" //str:ok a time layout
	dayLayout  = "2006-01-02"       //str:ok a time layout
)

// WhenOf is l's day and time of day as go-to takes them: what its prompt
// starts with.
func WhenOf(l *LogLine) string { return l.Day + " " + TimeOfDay(l.Entry.Time) }

// GotoTime pages in history back to when, then hands a line to show, for
// the front end to go to: for a day and time ("2026-09-24 21:04"), the
// last shown line in or before that minute (failing that, the first one
// after it); for a day alone, the day's first shown line.
func (g *Log) GotoTime(when string, show func(*LogLine)) []Effect {
	when = strings.Join(strings.Fields(when), " ")
	if t, err := time.ParseInLocation(whenLayout, when, time.Local); err == nil {
		return g.gotoMinute(t, show)
	}
	if _, err := time.Parse(dayLayout, when); err == nil {
		return g.gotoDay(when, show)
	}
	g.SetStatus(true, str.BrowseDateFormat())
	return nil
}

// gotoMinute is GotoTime for a day and time: minute t.
func (g *Log) gotoMinute(t time.Time, show func(*LogLine)) []Effect {
	end := t.Add(time.Minute) // a line drawn at 21:04 is in 21:04, seconds and all
	if (len(g.Lines) == 0 || !g.Lines[0].Entry.Time.Before(end)) && !g.HistDone {
		return g.Older(func() []Effect { return g.gotoMinute(t, show) })
	}
	v := g.Visible()
	if len(v) == 0 {
		g.SetStatus(true, str.BrowseNoLogs())
		return nil
	}
	to := v[0]
	for _, l := range v {
		if !l.Entry.Time.Before(end) {
			break
		}
		to = l
	}
	if show != nil {
		show(to)
	}
	g.Status = ""
	return nil
}

// gotoDay is GotoTime for a day alone.
func (g *Log) gotoDay(day string, show func(*LogLine)) []Effect {
	if (len(g.Lines) == 0 || g.Lines[0].Day > day) && !g.HistDone {
		return g.Older(func() []Effect { return g.gotoDay(day, show) })
	}
	for _, l := range g.Visible() {
		if l.Day >= day {
			if show != nil {
				show(l)
			}
			g.Status = ""
			return nil
		}
	}
	g.SetStatus(true, str.BrowseNoLogsAfter(day))
	return nil
}

// DayLabel formats "2026-09-24" as "Thu Sep 24".
func DayLabel(day string) string {
	t, err := time.Parse(dayLayout, day)
	if err != nil {
		return day
	}
	return t.Format(str.DateDay())
}
