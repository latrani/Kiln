package ui

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/rules"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// browseInitialLines is how much history browse loads up front (whole
// days, newest first, until at least this many lines).
const browseInitialLines = 200

// browsePrefixW is the width of "HH:MM" + space + gutter + space.
const browsePrefixW = 8

type bline struct {
	app.Line
	tags  []string // TagNames, kept for the filters
	text  string   // painted for display
	lower string   // plain text, lowercased, for text filters
}

type promptKind int

const (
	promptNone promptKind = iota
	promptFind
	promptDate
	promptFormat
	promptFilename
	promptFilterText
)

// browse is one character's browse-mode state. Lines are referenced by
// pointer so paging in older history never disturbs marks or exclusions.
type browse struct {
	cs           *charState
	hist         *history.Reader
	lines        []*bline // oldest first
	cursor       *bline
	top          *bline // first line drawn at the top of the body
	start        *bline
	end          *bline
	excluded     map[*bline]bool
	find         string
	prompt       promptKind
	pin          *Input
	copy         func(text string) tea.Cmd            // the model's clipboard write; nil: plain OSC 52
	saveFile     func(name string, data []byte) error // Deps.SaveFile; set, saving downloads instead
	format       string
	status       string
	statusErr    bool
	rowLines     []*bline     // body row → line (nil for dividers), from the last draw
	panel        *filterPanel // non-nil while the filter panel is open
	tagNames     []string     // the distinct tags on lines[:tagNamesN]; see items
	tagNamesN    int
	anyUntagged  bool      // some line in lines[:tagNamesN] has no tags
	loadedTo     time.Time // newest logged time at open, at log (millisecond) precision
	exportDir    string
	exportName   string         // file name template; see config.ExportNameVars
	exportFormat string         // preselected format; "" asks
	histDone     bool           // every log day is loaded (or there is no history)
	loading      bool           // an older day is being read in a tea.Cmd
	pending      func() tea.Cmd // what to do when it arrives; see requestOlder
	scrolled     bool           // top was set by scrollBy: the cursor follows the view, not the other way round
	searching    bool           // pending is find, reading older days for a match
}

// olderMsg carries one older day, read and rendered off the UI goroutine.
type olderMsg struct {
	key   string
	b     *browse // the browse that asked; stale if it has since closed
	lines []*bline
	done  bool // history is now exhausted
	err   error
	theme *theme.Theme // the theme active when the read started
}

// newBrowse opens browse mode over the logs l describes (none unless
// hasLogs).
func newBrowse(cs *charState, l logstore.Layout, hasLogs bool) *browse {
	b := &browse{cs: cs, excluded: map[*bline]bool{}, pin: NewInput()}
	if hasLogs {
		if h, err := history.NewReader(l); err == nil {
			b.hist = h
		}
	}
	b.histDone = b.hist == nil || b.hist.Exhausted()
	for len(b.lines) < browseInitialLines && b.loadOlder() {
	}
	b.cursor = b.lastVisible()
	if l := b.last(); l != nil {
		b.loadedTo = l.Entry.Time
	}
	cs.filter.ResetSeen() // this session's items light by the filter as it stands
	return b
}

// setExport takes the export settings from cfg.
func (b *browse) setExport(cfg *config.Config) {
	b.exportDir, b.exportName, b.exportFormat = cfg.ExportDir, cfg.ExportName, cfg.ExportFormat
}

func (b *browse) newLine(e logstore.Entry) *bline { return makeLine(b.cs.Rules, b.cs.hl, e) }

// makeLine makes and paints e. Like paint it only reads its arguments,
// so older days can be prepared off the UI goroutine.
func makeLine(r app.Rules, hl *rules.Highlighter, e logstore.Entry) *bline {
	l := r.Line(e)
	return &bline{Line: l, tags: l.TagNames(), text: paint(hl, l), lower: strings.ToLower(l.Plain)}
}

// readOlder reads and renders the next older day. Only one call may run
// at a time, and nothing else may touch h meanwhile.
func readOlder(h *history.Reader, r app.Rules, hl *rules.Highlighter) olderMsg {
	es, _, _, err := h.LoadOlder()
	msg := olderMsg{done: h.Exhausted(), err: err}
	for _, e := range es {
		msg.lines = append(msg.lines, makeLine(r, hl, e))
	}
	return msg
}

// loadOlder synchronously prepends the next older day; only newBrowse
// uses it, for the bounded initial load. It reports false when there is
// nothing more to load.
func (b *browse) loadOlder() bool {
	if b.histDone {
		return false
	}
	b.prepend(readOlder(b.hist, b.cs.Rules, b.cs.hl))
	return true
}

func (b *browse) prepend(msg olderMsg) {
	if msg.err != nil {
		b.setStatus(true, str.BrowseReadingLogs(msg.err))
	}
	b.histDone = msg.done
	b.lines = append(msg.lines, b.lines...)
}

// requestOlder starts reading the next older day in a tea.Cmd, so a big
// log never stalls Update. then runs when it arrives (it may request
// again to keep paging). While a read is in flight, a newer request just
// replaces then. It returns nil if history is exhausted.
func (b *browse) requestOlder(then func() tea.Cmd) tea.Cmd {
	if b.histDone {
		return nil
	}
	b.pending = then
	b.searching = false // a find waiting on older days gives way to whatever asked now
	if b.loading {
		return nil
	}
	b.loading = true
	h, r, hl, key, th := b.hist, b.cs.Rules, b.cs.hl, b.cs.Key, theme.Active()
	return func() tea.Msg {
		msg := readOlder(h, r, hl)
		msg.key, msg.b, msg.theme = key, b, th
		return msg
	}
}

// receive prepends a day read by requestOlder and runs what was waiting.
func (b *browse) receive(msg olderMsg) tea.Cmd {
	b.loading = false
	if msg.theme != theme.Active() { // made in a theme (and maybe rules) since replaced
		for _, l := range msg.lines {
			l.Line = b.cs.Rules.Reline(l.Line)
			l.tags, l.text = l.TagNames(), paint(b.cs.hl, l.Line)
		}
	}
	b.prepend(msg)
	then := b.pending
	b.pending = nil
	if then != nil {
		return then()
	}
	return nil
}

// dedupeTail is how many of the newest loaded lines appendLive checks for
// a copy of an incoming line.
const dedupeTail = 256

// appendLive adds a line that arrived while browsing. Lines logged before
// browse opened can still arrive as events afterwards; those are already
// loaded from the log (at millisecond precision), so a line no newer than
// the load watermark that matches a recently loaded one is skipped.
func (b *browse) appendLive(e logstore.Entry) {
	t := e.Time.Truncate(time.Millisecond)
	if !b.loadedTo.IsZero() && !t.After(b.loadedTo) {
		for _, l := range b.lines[max(0, len(b.lines)-dedupeTail):] {
			if l.Entry.Text == e.Text && l.Entry.Dir == e.Dir && l.Entry.Time.Truncate(time.Millisecond).Equal(t) {
				return
			}
		}
	}
	atEnd := b.cursor == nil || b.cursor == b.lastVisible()
	b.lines = append(b.lines, b.newLine(e))
	if atEnd {
		b.cursor = b.lastVisible()
	}
}

func (b *browse) last() *bline {
	if len(b.lines) == 0 {
		return nil
	}
	return b.lines[len(b.lines)-1]
}

// copyCmd puts text on the clipboard, by the model's way of doing it.
func (b *browse) copyCmd(text string) tea.Cmd {
	if b.copy == nil {
		return tea.SetClipboard(text)
	}
	return b.copy(text)
}

func (b *browse) setStatus(isErr bool, msg string) {
	b.status, b.statusErr = msg, isErr
}

func (b *browse) visible() []*bline {
	out := make([]*bline, 0, len(b.lines))
	for _, l := range b.lines {
		if b.shows(l) {
			out = append(out, l)
		}
	}
	return out
}

func (b *browse) lastVisible() *bline {
	v := b.visible()
	if len(v) == 0 {
		return nil
	}
	return v[len(v)-1]
}

func (b *browse) index(l *bline) int { return slices.Index(b.lines, l) }

// inRange reports whether l is inside the marked range. With only a
// start marked, just the start counts.
func (b *browse) inRange(l *bline) bool {
	if b.start == nil {
		return false
	}
	if b.end == nil {
		return l == b.start
	}
	i := b.index(l)
	return i >= b.index(b.start) && i <= b.index(b.end)
}

// items is the filter panel's list: Untagged, every tag on a loaded line
// or with a filter set (with their parents), then the text rows. It
// syncs the filter, so a tag seen for the first time obeys the filter
// already set.
func (b *browse) items() []scene.Item {
	if b.tagNamesN != len(b.lines) { // lines are only ever added
		seen := map[string]bool{}
		b.tagNames, b.anyUntagged = nil, false
		for _, l := range b.lines {
			if len(l.tags) == 0 {
				b.anyUntagged = true
			}
			for _, t := range l.tags {
				if !seen[t] {
					seen[t] = true
					b.tagNames = append(b.tagNames, t)
				}
			}
		}
		b.tagNamesN = len(b.lines)
	}
	f := &b.cs.filter
	untagged := scene.Item{Untagged: true}
	only, hasOnly := f.Only()
	var items []scene.Item
	if b.anyUntagged || f.Hidden(untagged) || hasOnly && only == untagged {
		items = append(items, untagged)
	}
	items = append(items, scene.TagItems(append(slices.Clone(b.tagNames), f.Tags()...))...)
	items = append(items, f.Texts()...)
	f.Sync(items)
	return items
}

// findStatus is the find term and match position, for the top bar; ""
// without a find.
func (b *browse) findStatus() string {
	if b.find == "" {
		return ""
	}
	if b.searching {
		return str.BrowseSearching(b.find, b.searchedTo())
	}
	i, n, more := b.matchPos()
	if more {
		return str.BrowseFindStatusMore(b.find, i, n)
	}
	return str.BrowseFindStatus(b.find, i, n)
}

// shows reports whether l is drawn: sent lines only with local_echo on
// (they're logged either way), and whatever the filter lets through. The
// lines stay loaded, so turning local_echo on brings them back.
func (b *browse) shows(l *bline) bool {
	return b.cs.echoes(l.Entry) && b.cs.filter.Visible(l.tags, l.lower)
}

// moveCursor moves by delta visible lines. Moving up past the oldest
// loaded line stops there and pages in older history; the rest of the
// move happens when it arrives.
func (b *browse) moveCursor(delta int) tea.Cmd {
	b.stopSearch() // the wheel, say: you've moved on
	v := b.visible()
	if len(v) == 0 {
		if delta < 0 { // everything loaded is hidden; look further back
			return b.requestOlder(func() tea.Cmd { return b.moveCursor(delta) })
		}
		return nil
	}
	i := slices.Index(v, b.cursor)
	if i < 0 {
		i = len(v) - 1
	}
	b.cursor = v[min(max(0, i+delta), len(v)-1)]
	if rest := i + delta; rest < 0 {
		return b.requestOlder(func() tea.Cmd { return b.moveCursor(rest) })
	}
	return nil
}

// scrollBy scrolls the view by delta lines, as the wheel does, leaving
// the cursor where it is unless it would go out of sight; then it's
// dragged along at the top or bottom edge. Scrolling up past the oldest
// loaded line pages in older history, and the rest of the scroll happens
// when it arrives.
func (b *browse) scrollBy(delta int) tea.Cmd {
	b.stopSearch()
	v := b.visible()
	if len(v) == 0 {
		return nil
	}
	ti := slices.Index(v, b.top)
	if ti < 0 {
		ti = max(0, slices.Index(v, b.cursor))
	}
	b.top, b.scrolled = v[min(max(0, ti+delta), len(v)-1)], true
	if rest := ti + delta; rest < 0 {
		return b.requestOlder(func() tea.Cmd { return b.scrollBy(rest) })
	}
	return nil
}

// toTop pages in all history, then moves to the oldest visible line.
func (b *browse) toTop() tea.Cmd {
	if v := b.visible(); len(v) > 0 {
		b.cursor = v[0]
	}
	return b.requestOlder(b.toTop)
}

func (b *browse) mark() {
	if b.cursor == nil {
		return
	}
	if b.start == nil || b.end != nil {
		b.newRange(b.cursor, nil)
		b.setStatus(false, str.BrowseRangeStart())
		return
	}
	b.end = b.cursor
	if b.index(b.end) < b.index(b.start) {
		b.start, b.end = b.end, b.start
	}
	b.rangeStatus()
}

// newRange starts a range, dropping exclusions left from an earlier one
// so they can't resurface if it grows over them.
func (b *browse) newRange(start, end *bline) {
	b.start, b.end = start, end
	clear(b.excluded)
}

func (b *browse) rangeStatus() {
	b.setStatus(false, str.BrowseLinesInRange(b.index(b.end)-b.index(b.start)+1))
}

// extendTo grows the range to take in l, from whichever end is nearer.
// With only a start marked, l becomes the other end.
func (b *browse) extendTo(l *bline) {
	if b.end == nil {
		b.end = b.start
	}
	if b.index(l) < b.index(b.start) {
		b.start = l
	} else if b.index(l) > b.index(b.end) {
		b.end = l
	}
	b.rangeStatus()
}

func (b *browse) toggleExclude(l *bline) {
	if l == nil || b.end == nil || !b.inRange(l) {
		b.setStatus(true, str.BrowseExcludeNeedsRange())
		return
	}
	b.excluded[l] = !b.excluded[l]
}

// findRE matches the find term literally and case-insensitively. Match
// offsets always index the original text: lowercasing a copy and reusing
// its offsets breaks on letters whose case forms differ in byte length.
func findRE(term string) *regexp.Regexp {
	return regexp.MustCompile("(?i)" + regexp.QuoteMeta(term))
}

func (b *browse) matches() []*bline {
	if b.find == "" {
		return nil
	}
	re := findRE(b.find)
	var out []*bline
	for _, l := range b.visible() {
		if re.MatchString(ansi.Strip(l.text)) {
			out = append(out, l)
		}
	}
	return out
}

// matchAt reports whether l is a find match that's shown.
func (b *browse) matchAt(re *regexp.Regexp, l *bline) bool {
	return b.shows(l) && re.MatchString(ansi.Strip(l.text))
}

// findOlder moves to the nearest match older than the cursor (or at it,
// with includeCursor), the way find goes: newest first. It reads in
// older days until one turns up or history runs out; it doesn't wrap.
func (b *browse) findOlder(includeCursor bool) tea.Cmd {
	if b.find == "" {
		return nil
	}
	i := len(b.lines) - 1
	if ci := b.index(b.cursor); ci >= 0 {
		i = ci
		if !includeCursor {
			i--
		}
	}
	return b.findOlderFrom(i)
}

// findOlderFrom looks for a match at line i or older; see findOlder.
func (b *browse) findOlderFrom(i int) tea.Cmd {
	re := findRE(b.find)
	b.searching = false
	for ; i >= 0; i-- {
		if l := b.lines[i]; b.matchAt(re, l) {
			b.cursor = l
			b.status = ""
			return nil
		}
	}
	if !b.histDone {
		var oldest *bline // where this pass stopped; the next goes on below it
		if len(b.lines) > 0 {
			oldest = b.lines[0]
		}
		cmd := b.requestOlder(func() tea.Cmd { return b.findOlderFrom(b.index(oldest) - 1) })
		b.searching = true
		return cmd
	}
	b.findFailed(str.BrowseNoOlderMatches(b.find))
	return nil
}

// searchedTo is the oldest day loaded, which a search has read through.
func (b *browse) searchedTo() string {
	if len(b.lines) == 0 {
		return ""
	}
	return dayLabel(b.lines[0].Day)
}

// stopSearch ends a find that's reading older days, leaving the cursor
// where it was, and reports whether there was one. The day being read
// still arrives, to nothing.
func (b *browse) stopSearch() bool {
	if !b.searching {
		return false
	}
	b.searching, b.pending = false, nil
	b.setStatus(false, str.BrowseSearchCancelled())
	return true
}

// findNewer moves to the nearest match newer than the cursor. Everything
// newer is always loaded; it doesn't wrap.
func (b *browse) findNewer() {
	if b.find == "" {
		return
	}
	re := findRE(b.find)
	for _, l := range b.lines[b.index(b.cursor)+1:] {
		if b.matchAt(re, l) {
			b.cursor = l
			b.status = ""
			return
		}
	}
	b.findFailed(str.BrowseNoNewerMatches(b.find))
}

// findFailed says there's nothing further: that the term matches nothing
// at all, if so, else msg.
func (b *browse) findFailed(msg string) {
	if len(b.matches()) == 0 && b.histDone {
		msg = str.BrowseNoMatches(b.find)
	}
	b.setStatus(true, msg)
}

// matchPos is the cursor's match counted from the newest, of n loaded;
// more says older history not yet loaded may hold others.
func (b *browse) matchPos() (i, n int, more bool) {
	ms := b.matches()
	if k := slices.Index(ms, b.cursor); k >= 0 {
		i = len(ms) - k
	}
	return i, len(ms), !b.histDone
}

// gotoDate pages in history back to day, then moves to its first
// visible line.
func (b *browse) gotoDate(day string) tea.Cmd {
	if _, err := time.Parse("2006-01-02", day); err != nil {
		b.setStatus(true, str.BrowseDateFormat())
		return nil
	}
	if (len(b.lines) == 0 || b.lines[0].Day > day) && !b.histDone {
		return b.requestOlder(func() tea.Cmd { return b.gotoDate(day) })
	}
	for _, l := range b.visible() {
		if l.Day >= day {
			b.cursor = l
			b.top = l
			b.status = ""
			return nil
		}
	}
	b.setStatus(true, str.BrowseNoLogsAfter(day))
	return nil
}

// selection is what an export contains: received lines inside the range,
// not excluded and not hidden by chips.
func (b *browse) selection() []logstore.Entry {
	if b.start == nil || b.end == nil {
		return nil
	}
	var out []logstore.Entry
	for _, l := range b.lines[b.index(b.start) : b.index(b.end)+1] {
		if !b.excluded[l] && b.shows(l) && scene.Exportable(l.Entry) {
			out = append(out, l.Entry)
		}
	}
	return out
}

func (b *browse) title() string {
	ch := b.cs.Ch
	when := ""
	if b.start != nil {
		when = " — " + b.start.Entry.Time.Local().Format(str.DateDayYear())
	}
	return ch.World + " " + ch.Name + when
}

// save writes the export to path, refusing to overwrite. "~/" is
// expanded and a relative path is placed in the export directory. With
// saveFile set (the web build) it offers a download named after path's
// last element instead.
func (b *browse) save(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		b.setStatus(true, str.BrowseNoFileName())
		return
	}
	if b.saveFile != nil {
		sel := b.selection()
		if len(sel) == 0 {
			b.setStatus(true, str.BrowseNothingToExport())
			return
		}
		name := filepath.Base(path)
		if err := b.saveFile(name, []byte(scene.Render(b.format, sel, b.title()))); err != nil {
			b.setStatus(true, err.Error())
			return
		}
		b.setStatus(false, str.StatusDownloaded(name))
		return
	}
	path, err := config.ExpandHome(path)
	if err != nil {
		b.setStatus(true, err.Error())
		return
	}
	if !filepath.IsAbs(path) {
		if b.exportDir == "" {
			b.setStatus(true, str.BrowseNoExportDir())
			return
		}
		path = filepath.Join(b.exportDir, path)
	}
	sel := b.selection()
	if len(sel) == 0 {
		b.setStatus(true, str.BrowseNothingToExport())
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.setStatus(true, err.Error())
		return
	}
	// O_EXCL makes "never overwrite" atomic: no window between checking
	// for the file and creating it.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		b.setStatus(true, str.BrowseFileExists(filepath.Base(path)))
		return
	} else if err != nil {
		b.setStatus(true, err.Error())
		return
	}
	_, err = f.WriteString(scene.Render(b.format, sel, b.title()))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		b.setStatus(true, err.Error())
		return
	}
	b.setStatus(false, str.BrowseSaved(path))
}

// key handles a key press in browse mode. It returns (cmd, close).
func (b *browse) key(k tea.KeyPressMsg, pageH int) (tea.Cmd, bool) {
	s := k.String()
	if b.stopSearch() && s == "esc" { // Esc stops a search, not log mode; any other key stops it and does its thing
		return nil, false
	}
	if b.prompt != promptNone {
		return b.promptKey(k), false
	}
	if b.panel != nil {
		return b.panelKey(k), false
	}
	if s == openFilterKey {
		b.togglePanel()
		return nil, false
	}
	b.status = ""
	switch browseKeys[s] {
	case actBack:
		return nil, true
	case actUp:
		return b.moveCursor(-1), false
	case actDown:
		return b.moveCursor(1), false
	case actPageUp:
		return b.moveCursor(-max(1, pageH-1)), false
	case actPageDown:
		return b.moveCursor(max(1, pageH-1)), false
	case actTop:
		return b.toTop(), false
	case actBottom:
		b.cursor = b.lastVisible()
	case actMark:
		b.mark()
	case actExclude:
		b.toggleExclude(b.cursor)
	case actFind:
		b.prompt = promptFind
		b.pin.SetValue(b.find)
	case actNextMatch: // n goes on the way find goes: older
		return b.findOlder(false), false
	case actPrevMatch:
		b.findNewer()
	case actDate:
		b.prompt = promptDate
		b.pin.SetValue("")
	case actExport:
		if len(b.selection()) == 0 {
			b.setStatus(true, str.BrowseMarkRange())
			break
		}
		b.prompt = promptFormat
	case actCopy:
		sel := b.selection()
		if len(sel) == 0 {
			b.setStatus(true, str.BrowseMarkRange())
			break
		}
		b.setStatus(false, str.BrowseCopied(len(sel)))
		return b.copyCmd(scene.Plain(sel)), false
	case actTags:
		if b.cursor != nil {
			b.setStatus(false, b.lineTags(b.cursor))
		}
	}
	return nil, false
}

// lineTags says which tags l has, and for each one without a style of
// its own, whose style it takes.
func (b *browse) lineTags(l *bline) string {
	if len(l.tags) == 0 {
		return str.BrowseNoLineTags()
	}
	parts := make([]string, len(l.tags))
	for i, tag := range l.tags {
		switch styled, ok := b.cs.hl.Styled(tag); {
		case !ok:
			parts[i] = str.BrowseTagUnstyled(tag)
		case styled != tag:
			parts[i] = str.BrowseTagAs(tag, styled)
		default:
			parts[i] = tag
		}
	}
	return str.BrowseLineTags(strings.Join(parts, str.Separator()))
}

// refilter keeps the reader's place after the filter changes: a cursor
// on a line now hidden moves to the nearest visible one.
func (b *browse) refilter() {
	b.items() // sync
	if b.cursor != nil && !b.shows(b.cursor) {
		if n := b.nearestVisible(b.cursor); n != nil {
			b.cursor = n
		}
	}
}

// nearestVisible returns the first visible line after l, or failing that
// the last visible line before it, so hiding l keeps the reader's place.
func (b *browse) nearestVisible(l *bline) *bline {
	i := b.index(l)
	for j := i + 1; j < len(b.lines); j++ {
		if b.shows(b.lines[j]) {
			return b.lines[j]
		}
	}
	for j := i - 1; j >= 0; j-- {
		if b.shows(b.lines[j]) {
			return b.lines[j]
		}
	}
	return nil
}

func (b *browse) promptKey(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if s == "esc" {
		b.prompt = promptNone
		return nil
	}
	if b.prompt == promptFormat {
		f, ok := exportFormatKeys[s]
		if s == "enter" && b.exportFormat != "" {
			f, ok = b.exportFormat, true
		}
		if ok {
			sel := b.selection()
			if len(sel) == 0 {
				b.prompt = promptNone
				b.setStatus(true, str.BrowseNothingLeft())
				return nil
			}
			b.format = f
			b.prompt = promptFilename
			dir := b.exportDir
			if b.saveFile != nil {
				dir = "" // a download: just a name
			}
			b.pin.SetValue(scene.FileName(dir, b.exportName, sel[0].Time.Local(), b.cs.Ch.World, b.cs.Ch.Name, f))
		}
		return nil
	}
	switch s {
	case "enter":
		v := strings.TrimSpace(b.pin.Value())
		kind := b.prompt
		b.prompt = promptNone
		switch kind {
		case promptFind:
			b.find = v
			return b.findOlder(true)
		case promptDate:
			return b.gotoDate(v)
		case promptFilename:
			b.save(v)
		case promptFilterText:
			if b.cs.filter.AddText(v, b.items()) && b.panel != nil {
				b.panel.sel = filterSel{item: scene.Item{Name: v, Text: true}}
				b.panel.follow = true
			}
			b.refilter()
		}
	default:
		editKey(b.pin, k) // the same line editing as the input and forms
	}
	return nil
}

// promptText is the action bar's prompt label.
func (b *browse) promptLabel() string {
	switch b.prompt {
	case promptFind:
		return str.BrowseFindPrompt()
	case promptDate:
		return str.BrowseDatePrompt()
	case promptFormat:
		if b.exportFormat != "" {
			return str.BrowseFormatPromptDefault(b.exportFormat)
		}
		return str.BrowseFormatPrompt()
	case promptFilename:
		return str.BrowseSavePrompt()
	case promptFilterText:
		return str.FilterPrompt()
	}
	return ""
}

// rowsFor is how many body rows a line takes, including a day divider.
func (b *browse) rowsFor(l *bline, prev *bline, w int) int {
	n := len(ansi.Wrap(l.text, max(1, w-browsePrefixW)))
	if prev == nil || prev.Day != l.Day {
		n++
	}
	return n
}

// scrollToCursor adjusts top so the cursor is within h body rows and no
// blank rows are left below the newest line.
func (b *browse) scrollToCursor(v []*bline, h, w int) {
	if len(v) == 0 {
		b.top = nil
		return
	}
	ci := slices.Index(v, b.cursor)
	if ci < 0 {
		b.cursor, ci = v[len(v)-1], len(v)-1
	}
	// rows is how many rows v[i] takes with v[top] at the top, where its
	// day's divider always shows (see view).
	rows := func(i, top int) int {
		var prev *bline
		if i > 0 && i != top {
			prev = v[i-1]
		}
		return b.rowsFor(v[i], prev, w)
	}
	ti := slices.Index(v, b.top)
	if b.scrolled && ti >= 0 {
		b.scrolled = false
		ti = min(ti, b.bottomTop(v, rows, h)) // no scrolling past the newest line
		if ci < ti {                          // dragged along at the top edge
			ci = ti
		}
		last, n := ti, rows(ti, ti) // the last line wholly in view
		for last+1 < len(v) && n+rows(last+1, ti) <= h {
			last++
			n += rows(last, ti)
		}
		if ci > last { // or at the bottom one
			ci = last
		}
		b.cursor, b.top = v[ci], v[ti]
		return
	}
	b.scrolled = false
	if ti < 0 || ci < ti {
		ti = ci
	}
	if ci-ti > h { // every line takes at least one row
		ti = ci - h
	}
	for ti < ci {
		n := 0
		for i := ti; i <= ci; i++ {
			n += rows(i, ti)
		}
		if n <= h {
			break
		}
		ti++
	}
	b.top = v[min(ti, b.bottomTop(v, rows, h))]
}

// bottomTop is the earliest top line that still fits everything through
// the newest line in h rows: a later one would leave rows blank below it.
func (b *browse) bottomTop(v []*bline, rows func(i, top int) int, h int) int {
	fill, n := len(v)-1, 0
	for ; fill >= 0; fill-- {
		if n+rows(fill, fill) > h {
			break
		}
		n += rows(fill, -1)
	}
	return fill + 1
}

// view draws the right pane (w×h) in browse mode. When a prompt is being
// typed, showCur is true and (curX, curY) is the cursor within the pane.
func (b *browse) view(w, h int) (rows []string, curX, curY int, showCur bool) {
	b.items() // new tags take the filter already set
	v := b.visible()
	bodyH := max(1, h-3)
	b.scrollToCursor(v, bodyH, w)

	// Body.
	b.rowLines = b.rowLines[:0]
	if len(b.lines) == 0 && b.histDone {
		rows = append(rows, theme.Paint(theme.LogLoading, str.BrowseNoLogs()))
		b.rowLines = append(b.rowLines, nil)
	}
	ms := b.matches()
	start := slices.Index(v, b.top)
	if b.loading && start <= 0 { // the oldest loaded line is at the top
		rows = append(rows, theme.Paint(theme.LogLoading, str.BrowseLoading()))
		b.rowLines = append(b.rowLines, nil)
	}
	for i := max(0, start); i < len(v) && len(b.rowLines) < bodyH; i++ {
		l := v[i]
		if i == max(0, start) || v[i-1].Day != l.Day { // the top line's day always shows, so the date stays in sight
			rows = append(rows, theme.Paint(theme.LogDay, "── "+dayLabel(l.Day)+" ──"))
			b.rowLines = append(b.rowLines, nil)
			if len(b.rowLines) >= bodyH {
				break
			}
		}
		ts := l.Entry.Time.Local().Format("15:04")
		if l == b.cursor {
			ts = theme.Paint(theme.LogCursor, ts)
		} else {
			ts = theme.Paint(theme.LogTime, ts)
		}
		gutter := " "
		if b.inRange(l) {
			gutter = theme.Paint(theme.LogSelected, glyphSelected)
			if b.excluded[l] {
				gutter = theme.Paint(theme.LogExcluded, glyphExcluded)
			}
		}
		text := l.text
		if b.find != "" && slices.Contains(ms, l) {
			text = highlightFind(ansi.Strip(l.text), b.find)
		}
		for j, row := range ansi.Wrap(text, max(1, w-browsePrefixW)) {
			prefix := ts + " " + gutter + " "
			if j > 0 {
				prefix = "      " + gutter + " "
			}
			rows = append(rows, prefix+row)
			b.rowLines = append(b.rowLines, l)
			if len(b.rowLines) >= bodyH {
				break
			}
		}
	}
	// Too few lines to fill the body sit at its bottom, newest last, as
	// in the scrollback; "no logs yet" stays at the top.
	if pad := bodyH - len(rows); pad > 0 && len(b.lines) > 0 {
		rows = append(make([]string, pad), rows...)
		b.rowLines = append(make([]*bline, pad), b.rowLines...)
	}
	for len(rows) < bodyH {
		rows = append(rows, "")
		b.rowLines = append(b.rowLines, nil)
	}

	// Action bar.
	rows = append(rows, theme.Paint(theme.Rule, strings.Repeat("─", w)))
	switch {
	case b.prompt == promptFormat:
		rows = append(rows, theme.Fill(theme.LogBar, b.promptLabel(), w))
	case b.prompt != promptNone:
		label := b.promptLabel()
		text, x := promptWindow([]rune(b.pin.Value()), b.pin.col, w-xansi.StringWidth(label)-1)
		rows = append(rows, theme.Fill(theme.LogBar, label+text, w))
		curX, curY, showCur = xansi.StringWidth(label)+x, h-2, true
	default: // messages go to the bottom bar

		hints := str.BrowseHints()
		if b.panel != nil {
			hints = str.FilterHints()
		}
		rows = append(rows, theme.Fill(theme.LogBar, theme.Paint(theme.LogHints, hints), w))
	}
	rows = append(rows, theme.Paint(theme.RuleStatus, strings.Repeat("─", w)))
	return rows, curX, curY, showCur
}

// restyle repaints every line with paint, after a theme change.
func (b *browse) restyle(paint func(app.Line) string) {
	for _, l := range b.lines {
		l.text = paint(l.Line)
	}
}

// dayLabel formats "2026-09-24" as "Thu Sep 24".
func dayLabel(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format(str.DateDay())
}

// highlightFind reverses every case-insensitive occurrence of needle.
func highlightFind(plain, needle string) string {
	if needle == "" {
		return plain
	}
	var out strings.Builder
	last := 0
	for _, m := range findRE(needle).FindAllStringIndex(plain, -1) {
		out.WriteString(plain[last:m[0]] + theme.Paint(theme.LogFind, plain[m[0]:m[1]]))
		last = m[1]
	}
	out.WriteString(plain[last:])
	return out.String()
}

// click handles a left click at (x, y) within the right pane. Clicks are
// ignored while a prompt is open so the selection can't change under it.
func (b *browse) click(x, y int, shift bool) {
	b.stopSearch()
	if b.prompt != promptNone {
		return
	}
	row := y
	if row < 0 || row >= len(b.rowLines) || b.rowLines[row] == nil {
		return
	}
	// Like selecting files in Finder: a click selects its line, a
	// shift-click outside the range extends it, and one inside toggles
	// that line out of (or back into) it. The gutter column toggles too.
	l := b.rowLines[row]
	switch {
	case shift && b.start != nil && b.end != nil && b.inRange(l):
		b.toggleExclude(l)
	case shift:
		if b.start == nil {
			b.newRange(b.cursor, nil)
		}
		b.cursor = l
		b.extendTo(l)
	case x == browsePrefixW-2 && b.inRange(l) && b.end != nil:
		b.toggleExclude(l)
	default:
		b.cursor = l
		b.newRange(l, l)
		b.rangeStatus()
	}
}

// promptWindow fits a one-line value into avail cells, scrolling
// horizontally so the cursor (at rune index col) stays visible; hidden
// text on the left is marked with "…". It returns the text to draw and
// the cursor's cell offset within it.
func promptWindow(rs []rune, col, avail int) (string, int) {
	avail = max(2, avail)
	width := func(r []rune) int { return xansi.StringWidth(string(r)) }
	if width(rs) <= avail {
		return string(rs), width(rs[:col])
	}
	start := 0
	for start < col && width(rs[start:col])+1 > avail-1 {
		start++
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	shown := string(rs[start:])
	shown = xansi.Truncate(shown, avail-xansi.StringWidth(prefix), "")
	return prefix + shown, xansi.StringWidth(prefix) + width(rs[start:col])
}
