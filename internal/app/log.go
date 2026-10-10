package app

import (
	"regexp"
	"slices"
	"strings"
	"time"

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
	Cursor     *LogLine
	Start, End *LogLine // the marked range; End nil while only Start is marked
	Excluded   map[*LogLine]bool
	Find       string
	Status     string // log mode's message; the front end shows it and clears it
	StatusErr  bool
	HistDone   bool   // every log day is loaded (or there is no history)
	Loading    bool   // an older day is being read
	Searching  bool   // a find is reading older days for a match
	Panel      *Panel // the filter panel, while it's open
	// Shown is a line's text as the front end shows it, unstyled: what
	// find matches. nil: Line.Plain.
	Shown func(*LogLine) string

	a           *App
	c           *Char
	hist        *history.Reader
	loadedTo    time.Time       // newest logged time at open, at log (millisecond) precision
	pending     func() []Effect // what to do when the day being read arrives; see Older
	tagNames    []string        // the distinct tags on Lines[:tagNamesN]; see Items
	tagNamesN   int
	anyUntagged bool // some line in Lines[:tagNamesN] has no tags
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
	g.Cursor = g.LastVisible()
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

// appendLive adds a line that arrived while the log is open. Lines
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
	atEnd := g.Cursor == nil || g.Cursor == g.LastVisible()
	g.Lines = append(g.Lines, makeLogLine(g.c.Rules, e))
	if atEnd {
		g.Cursor = g.LastVisible()
	}
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

// Shows reports whether l is drawn: sent lines only with local_echo on
// (they're logged either way), and whatever the filter lets through. The
// lines stay loaded, so turning local_echo on brings them back.
func (g *Log) Shows(l *LogLine) bool {
	return g.c.Echoes(l.Entry) && g.c.Filter.Visible(l.Tags, l.Lower)
}

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

// InRange reports whether l is inside the marked range. With only a
// start marked, just the start counts.
func (g *Log) InRange(l *LogLine) bool {
	if g.Start == nil {
		return false
	}
	if g.End == nil {
		return l == g.Start
	}
	i := g.Index(l)
	return i >= g.Index(g.Start) && i <= g.Index(g.End)
}

// Items is the filter panel's list: Untagged, every tag on a loaded line
// or with a filter set (with their parents), then the text rows. It
// syncs the filter, so a tag seen for the first time obeys the filter
// already set.
func (g *Log) Items() []scene.Item {
	if g.tagNamesN != len(g.Lines) { // lines are only ever added
		seen := map[string]bool{}
		g.tagNames, g.anyUntagged = nil, false
		for _, l := range g.Lines {
			if len(l.Tags) == 0 {
				g.anyUntagged = true
			}
			for _, t := range l.Tags {
				if !seen[t] {
					seen[t] = true
					g.tagNames = append(g.tagNames, t)
				}
			}
		}
		g.tagNamesN = len(g.Lines)
	}
	f := &g.c.Filter
	untagged := scene.Item{Untagged: true}
	only, hasOnly := f.Only()
	var items []scene.Item
	if g.anyUntagged || f.Hidden(untagged) || hasOnly && only == untagged {
		items = append(items, untagged)
	}
	items = append(items, scene.TagItems(append(slices.Clone(g.tagNames), f.Tags()...))...)
	items = append(items, f.Texts()...)
	f.Sync(items)
	return items
}

// Refilter keeps the reader's place after the filter changes: a cursor
// on a line now hidden moves to the nearest visible one.
func (g *Log) Refilter() {
	g.Items() // sync
	if g.Cursor != nil && !g.Shows(g.Cursor) {
		if n := g.nearestVisible(g.Cursor); n != nil {
			g.Cursor = n
		}
	}
}

// nearestVisible returns the first visible line after l, or failing that
// the last visible line before it, so hiding l keeps the reader's place.
func (g *Log) nearestVisible(l *LogLine) *LogLine {
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

// SetCursor puts the cursor on l: a click, or the view dragging it along.
func (g *Log) SetCursor(l *LogLine) { g.Cursor = l }

// MoveCursor moves by delta visible lines. Moving up past the oldest
// loaded line stops there and pages in older history; the rest of the
// move happens when it arrives.
func (g *Log) MoveCursor(delta int) []Effect {
	g.StopSearch() // the wheel, say: you've moved on
	v := g.Visible()
	if len(v) == 0 {
		if delta < 0 { // everything loaded is hidden; look further back
			return g.Older(func() []Effect { return g.MoveCursor(delta) })
		}
		return nil
	}
	i := slices.Index(v, g.Cursor)
	if i < 0 {
		i = len(v) - 1
	}
	g.Cursor = v[min(max(0, i+delta), len(v)-1)]
	if rest := i + delta; rest < 0 {
		return g.Older(func() []Effect { return g.MoveCursor(rest) })
	}
	return nil
}

// ToTop pages in all history, then moves to the oldest visible line.
func (g *Log) ToTop() []Effect {
	if v := g.Visible(); len(v) > 0 {
		g.Cursor = v[0]
	}
	return g.Older(g.ToTop)
}

// ToBottom moves to the newest visible line.
func (g *Log) ToBottom() { g.Cursor = g.LastVisible() }

// Mark marks the cursor's line as the start of a range, or, with a start
// marked, as its other end.
func (g *Log) Mark() {
	if g.Cursor == nil {
		return
	}
	if g.Start == nil || g.End != nil {
		g.NewRange(g.Cursor, nil)
		g.SetStatus(false, str.BrowseRangeStart())
		return
	}
	g.End = g.Cursor
	if g.Index(g.End) < g.Index(g.Start) {
		g.Start, g.End = g.End, g.Start
	}
	g.RangeStatus()
}

// NewRange starts a range, dropping exclusions left from an earlier one
// so they can't resurface if it grows over them.
func (g *Log) NewRange(start, end *LogLine) {
	g.Start, g.End = start, end
	clear(g.Excluded)
}

// RangeStatus says how many lines the range holds.
func (g *Log) RangeStatus() {
	g.SetStatus(false, str.BrowseLinesInRange(g.Index(g.End)-g.Index(g.Start)+1))
}

// ExtendTo grows the range to take in l, from whichever end is nearer.
// With only a start marked, l becomes the other end.
func (g *Log) ExtendTo(l *LogLine) {
	if g.End == nil {
		g.End = g.Start
	}
	if g.Index(l) < g.Index(g.Start) {
		g.Start = l
	} else if g.Index(l) > g.Index(g.End) {
		g.End = l
	}
	g.RangeStatus()
}

// ToggleExclude takes l out of the range, or puts it back.
func (g *Log) ToggleExclude(l *LogLine) {
	if l == nil || g.End == nil || !g.InRange(l) {
		g.SetStatus(true, str.BrowseExcludeNeedsRange())
		return
	}
	g.Excluded[l] = !g.Excluded[l]
}

// Selection is what an export contains: received lines inside the range,
// not excluded and not hidden by the filter.
func (g *Log) Selection() []logstore.Entry {
	if g.Start == nil || g.End == nil {
		return nil
	}
	var out []logstore.Entry
	for _, l := range g.Lines[g.Index(g.Start) : g.Index(g.End)+1] {
		if !g.Excluded[l] && g.Shows(l) && scene.Exportable(l.Entry) {
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
// mark a range first.
func (g *Log) Copy() []Effect {
	sel := g.Selection()
	if len(sel) == 0 {
		g.SetStatus(true, str.BrowseMarkRange())
		return nil
	}
	g.SetStatus(false, str.BrowseCopied(len(sel)))
	return []Effect{Copy{Text: scene.Plain(sel)}}
}

// ExportReady reports whether there's a selection to export, and if not
// says to mark a range first.
func (g *Log) ExportReady() bool {
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

// SetFind sets the find term and goes to its nearest match at the cursor
// or older.
func (g *Log) SetFind(term string) []Effect {
	g.Find = term
	return g.FindOlder(true)
}

// FindOlder moves to the nearest match older than the cursor (or at it,
// with includeCursor), the way find goes: newest first. It reads in
// older days until one turns up or history runs out; it doesn't wrap.
func (g *Log) FindOlder(includeCursor bool) []Effect {
	if g.Find == "" {
		return nil
	}
	i := len(g.Lines) - 1
	if ci := g.Index(g.Cursor); ci >= 0 {
		i = ci
		if !includeCursor {
			i--
		}
	}
	return g.findOlderFrom(i)
}

// findOlderFrom looks for a match at line i or older; see FindOlder.
func (g *Log) findOlderFrom(i int) []Effect {
	re := FindRE(g.Find)
	g.Searching = false
	for ; i >= 0; i-- {
		if l := g.Lines[i]; g.matchAt(re, l) {
			g.Cursor = l
			g.Status = ""
			return nil
		}
	}
	if !g.HistDone {
		var oldest *LogLine // where this pass stopped; the next goes on below it
		if len(g.Lines) > 0 {
			oldest = g.Lines[0]
		}
		effs := g.Older(func() []Effect { return g.findOlderFrom(g.Index(oldest) - 1) })
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

// StopSearch ends a find that's reading older days, leaving the cursor
// where it was, and reports whether there was one. The day being read
// still arrives, to nothing.
func (g *Log) StopSearch() bool {
	if !g.Searching {
		return false
	}
	g.Searching, g.pending = false, nil
	g.SetStatus(false, str.BrowseSearchCancelled())
	return true
}

// FindNewer moves to the nearest match newer than the cursor. Everything
// newer is always loaded; it doesn't wrap.
func (g *Log) FindNewer() {
	if g.Find == "" {
		return
	}
	re := FindRE(g.Find)
	for _, l := range g.Lines[g.Index(g.Cursor)+1:] {
		if g.matchAt(re, l) {
			g.Cursor = l
			g.Status = ""
			return
		}
	}
	g.findFailed(str.BrowseNoNewerMatches(g.Find))
}

// findFailed says there's nothing further: that the term matches nothing
// at all, if so, else msg.
func (g *Log) findFailed(msg string) {
	if len(g.Matches()) == 0 && g.HistDone {
		msg = str.BrowseNoMatches(g.Find)
	}
	g.SetStatus(true, msg)
}

// matchPos is the cursor's match counted from the newest, of n loaded;
// more says older history not yet loaded may hold others.
func (g *Log) matchPos() (i, n int, more bool) {
	ms := g.Matches()
	if k := slices.Index(ms, g.Cursor); k >= 0 {
		i = len(ms) - k
	}
	return i, len(ms), !g.HistDone
}

// FindStatus is the find term and match position, for the top bar; ""
// without a find.
func (g *Log) FindStatus() string {
	if g.Find == "" {
		return ""
	}
	if g.Searching {
		return str.BrowseSearching(g.Find, g.searchedTo())
	}
	i, n, more := g.matchPos()
	if more {
		return str.BrowseFindStatusMore(g.Find, i, n)
	}
	return str.BrowseFindStatus(g.Find, i, n)
}

// GotoDate pages in history back to day (YYYY-MM-DD), then moves to its
// first visible line and hands it to show, for the front end to put at
// the top of what it draws.
func (g *Log) GotoDate(day string, show func(*LogLine)) []Effect {
	if _, err := time.Parse("2006-01-02", day); err != nil {
		g.SetStatus(true, str.BrowseDateFormat())
		return nil
	}
	if (len(g.Lines) == 0 || g.Lines[0].Day > day) && !g.HistDone {
		return g.Older(func() []Effect { return g.GotoDate(day, show) })
	}
	for _, l := range g.Visible() {
		if l.Day >= day {
			g.Cursor = l
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
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format(str.DateDay())
}
