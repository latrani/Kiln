package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/app"
	"github.com/latrani/Kiln/internal/config"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/str"
	"github.com/latrani/Kiln/internal/theme"
)

// browsePrefixW is the width of "HH:MM" + space + gutter + space;
// browseTimeW, of the time.
const browsePrefixW, browseTimeW = 8, 5

type promptKind int

const (
	promptNone promptKind = iota
	promptFind
	promptDate
	promptFormat
	promptFilename
	promptFilterText
)

// browse is one character's log mode as the terminal draws it, over
// the core's log.
type browse struct {
	*app.Log
	cs         *charState
	Cursor     *app.LogLine            // the highlighted line; see logcursor.go
	top        *app.LogLine            // the line whose rows start the body
	skip       int                     // rows of top's (its day's rule first, when it has one) scrolled above the body
	width      int                     // the pane's width at the last draw, for the wheel's rows
	scrolled   bool                    // top was set by scrollBy: the cursor follows the view, not the other way round
	rowLines   []*app.LogLine          // body row → line (nil for dividers), from the last draw
	painted    map[*app.LogLine]string // each line as painted, once it's been needed; see text
	prompt     promptKind
	pin        *Input
	Panel      *filterPanel // the filter panel, always there in log mode
	panelFocus bool         // keys go to the panel, not the log
	drag       *logDrag     // a press in the log, and the drag it may become; see logdrag.go
	format     string
	run        func([]app.Effect) tea.Cmd // the model's run
	cfg        func() *config.Config      // the config as it is now, for the export settings
}

// text is l painted, painting it the first time it's needed.
func (b *browse) text(l *app.LogLine) string {
	s, ok := b.painted[l]
	if !ok {
		s = paint(b.cs.hl, l.Line)
		b.painted[l] = s
	}
	return s
}

// scrollBy scrolls the view by delta rows, as the wheel does, leaving
// the cursor where it is unless it would go out of sight; then it's
// dragged along at the top or bottom edge. Scrolling up past the oldest
// loaded line pages in older history, and the rest of the scroll happens
// when it arrives.
func (b *browse) scrollBy(delta int) []app.Effect {
	b.StopSearch()
	v := b.Visible()
	if len(v) == 0 {
		return nil
	}
	g := b.grid(v, b.width)
	at, rest := g.advance(g.start(b), delta)
	b.setStart(v, at)
	b.scrolled = true
	if rest < 0 {
		return b.Older(func() []app.Effect { return b.scrollBy(rest) })
	}
	return nil
}

// save offers the export as a file called path; see Model.saveFile.
func (b *browse) save(path string) tea.Cmd { return b.run(b.Export(b.format, path)) }

// key handles a key press in browse mode. It returns (cmd, close).
func (b *browse) key(k tea.KeyPressMsg, pageH int) (tea.Cmd, bool) {
	s := k.String()
	if b.StopSearch() && s == "esc" { // Esc stops a search, not log mode; any other key stops it and does its thing
		return nil, false
	}
	if b.prompt != promptNone {
		return b.promptKey(k), false
	}
	switch {
	case s == openFilterKey:
		b.focusPanel(!b.panelFocus)
		return nil, false
	case panelOnlyKeys[s] || sharedKeys[s] && b.panelFocus:
		return b.panelKey(k), false
	}
	b.ClearStatus()
	switch browseKeys[s] {
	case actBack: // marking, then find, as Esc backs out of one thing at a time
		if b.Marking {
			b.EndMark()
			return nil, false
		}
		if b.Find != "" {
			b.EndFind()
			return nil, false
		}
		return nil, true
	case actUp:
		return b.run(b.MoveCursor(-1)), false
	case actDown:
		return b.run(b.MoveCursor(1)), false
	case actPageUp:
		return b.run(b.MoveCursor(-max(1, pageH-1))), false
	case actPageDown:
		return b.run(b.MoveCursor(max(1, pageH-1))), false
	case actTop:
		return b.run(b.ToTop()), false
	case actBottom:
		b.ToBottom()
	case actSelect:
		b.Mark()
	case actExclude:
		b.ToggleExclude(b.Cursor)
	case actFind: // starting with the last term, so / and Enter bring it back
		b.prompt = promptFind
		b.pin.SetValue(b.LastFind)
	case actNextMatch: // n goes on the way find goes: older
		return b.run(b.FindOlder(false)), false
	case actPrevMatch:
		b.FindNewer()
	case actDate: // starting from the cursor's line, to edit
		b.prompt = promptDate
		b.pin.SetValue("")
		if b.Cursor != nil {
			b.pin.SetValue(app.WhenOf(b.Cursor))
		}
	case actExport:
		if !b.ExportReady() {
			break
		}
		b.prompt = promptFormat
	case actCopy:
		return b.run(b.Copy()), false
	}
	return nil, false
}

// statusParts is what log mode's statusline says with no message up:
// find, while it's on, then the cursor line's tags (if it has any), then
// how many lines are selected, while a range is; most specific first, so
// a narrow screen cuts the least. Selecting is the Select chip's to say.
func (b *browse) statusParts() string {
	var parts []string
	if f := b.FindStatus(); f != "" {
		parts = append(parts, f)
	}
	if b.Cursor != nil {
		if t := b.lineTags(b.Cursor); t != "" {
			parts = append(parts, t)
		}
	}
	if b.Start != nil {
		parts = append(parts, str.ViewSelected(len(b.Selection())))
	}
	return strings.Join(parts, str.Separator())
}

// hints is the action bar's keys for the state select mode is in:
// nothing selected, selecting, or a range selected; while the panel has
// focus, only those that still reach the log.
func (b *browse) hints() string {
	away := b.panelFocus // only the keys that reach the log from the panel
	switch {
	case b.Marking && away:
		return str.BrowseHintsSelectingAway()
	case b.Marking:
		return str.BrowseHintsSelecting()
	case b.Start != nil && away:
		return str.BrowseHintsSelectedAway()
	case b.Start != nil:
		return str.BrowseHintsSelected()
	case away:
		return str.BrowseHintsNoneAway()
	}
	return str.BrowseHintsNone()
}

// lineTags says which tags l has, and for each one without a style of
// its own, whose style it takes.
func (b *browse) lineTags(l *app.LogLine) string {
	return app.LineTags(l.Tags, b.cs.hl.Styled)
}

func (b *browse) promptKey(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if s == "esc" {
		b.prompt = promptNone
		return nil
	}
	if b.prompt == promptFormat {
		f, ok := exportFormatKeys[s]
		if def := b.cfg().ExportFormat; s == "enter" && def != "" {
			f, ok = def, true
		}
		if ok {
			name, ok := b.ExportFileName(f, b.cfg().ExportDir)
			if !ok {
				b.prompt = promptNone
				return nil
			}
			b.format = f
			b.prompt = promptFilename
			b.pin.SetValue(name)
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
			return b.run(b.SetFind(v))
		case promptDate:
			return b.run(b.GotoTime(v))
		case promptFilename:
			return b.save(v)
		case promptFilterText:
			if b.AddText(v) && b.Panel != nil { // a new row becomes the highlight
				b.Panel.Sel = filterSel{Item: scene.Item{Name: v, Text: true}}
				b.Panel.follow = true
			}
			b.Refilter()
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
		if def := b.cfg().ExportFormat; def != "" {
			return str.BrowseFormatPromptDefault(def)
		}
		return str.BrowseFormatPrompt()
	case promptFilename:
		return str.BrowseSavePrompt()
	}
	return ""
}

// rowPos is a row of the body's rows laid end to end: row r of v[i]'s,
// its day's rule first when it has one. {len(v), 0} is past the end.
type rowPos struct{ i, r int }

func (a rowPos) before(c rowPos) bool { return a.i < c.i || a.i == c.i && a.r < c.r }

// logGrid is the shown lines laid out in rows w wide: each line's text
// wrapped, after its day's rule when the day changes (and before the
// first). It wraps a line only when it's asked about.
type logGrid struct {
	v    []*app.LogLine
	b    *browse
	w    int
	rows map[int]int
}

func (b *browse) grid(v []*app.LogLine, w int) *logGrid {
	return &logGrid{v: v, b: b, w: max(1, w-browsePrefixW), rows: map[int]int{}}
}

// rule reports whether v[i] starts with its day's rule.
func (g *logGrid) rule(i int) bool { return i == 0 || g.v[i-1].Day != g.v[i].Day }

// height is how many rows v[i] takes, its rule included.
func (g *logGrid) height(i int) int {
	if n, ok := g.rows[i]; ok {
		return n
	}
	n := len(ansi.Wrap(g.b.text(g.v[i]), g.w))
	if g.rule(i) {
		n++
	}
	g.rows[i] = n
	return n
}

// text is where v[i]'s text starts, past its rule.
func (g *logGrid) text(i int) rowPos {
	if g.rule(i) {
		return rowPos{i, 1}
	}
	return rowPos{i, 0}
}

// end is past v[i]'s last row.
func (g *logGrid) end(i int) rowPos { return g.advanced(rowPos{i, 0}, g.height(i)) }

// advance moves p by n rows, stopping at the first row or past the
// last; rest is what's left of n when it stops.
func (g *logGrid) advance(p rowPos, n int) (at rowPos, rest int) {
	for n > 0 {
		if p.i >= len(g.v) {
			return p, n
		}
		left := g.height(p.i) - p.r
		if n < left {
			return rowPos{p.i, p.r + n}, 0
		}
		n -= left
		p = rowPos{p.i + 1, 0}
	}
	for n < 0 {
		if -n <= p.r {
			return rowPos{p.i, p.r + n}, 0
		}
		n += p.r
		if p.i == 0 {
			return rowPos{0, 0}, n
		}
		p = rowPos{p.i - 1, g.height(p.i - 1)}
	}
	return p, 0
}

func (g *logGrid) advanced(p rowPos, n int) rowPos {
	at, _ := g.advance(p, n)
	return at
}

// dist is how many rows c is after a, up to past (a further one is past
// as far).
func (g *logGrid) dist(a, c rowPos, past int) int {
	if c.before(a) {
		return -g.dist(c, a, past)
	}
	n := -a.r
	for i := a.i; i < c.i; i++ {
		if n += g.height(i); n > past {
			return past
		}
	}
	return n + c.r
}

// start is the body's first row, as top and skip say.
func (g *logGrid) start(b *browse) rowPos {
	ti := slices.Index(g.v, b.top)
	if ti < 0 {
		return rowPos{0, 0}
	}
	return rowPos{ti, min(b.skip, g.height(ti)-1)}
}

// setStart makes p the body's first row.
func (b *browse) setStart(v []*app.LogLine, p rowPos) {
	p.i = min(p.i, len(v)-1)
	b.top, b.skip = v[p.i], p.r
}

// bottom is the latest first row that still fills h rows: one later
// would leave rows blank below the newest line.
func (g *logGrid) bottom(h int) rowPos {
	return g.advanced(g.end(len(g.v)-1), -h)
}

// scrollToCursor sets the body's first row so the cursor is in view,
// under the day row (see view), and no blank rows are left below the
// newest line. A line taller than the body shows its start.
func (b *browse) scrollToCursor(v []*app.LogLine, h, w int) {
	if len(v) == 0 {
		b.top, b.skip = nil, 0
		return
	}
	ci := slices.Index(v, b.Cursor)
	if ci < 0 { // hidden (or none): the nearest shown line, as Refilter, else the newest
		ci = len(v) - 1
		if n := b.NearestVisible(b.Cursor); b.Index(b.Cursor) >= 0 && n != nil {
			ci = slices.Index(v, n)
		}
		b.SetCursor(v[ci])
	}
	g := b.grid(v, w)
	lo := min(1, h-1) // the first row a line's text is in sight on: the day row covers the body's first
	clamp := func(p rowPos) rowPos {
		if bot := g.bottom(h); bot.before(p) {
			return bot
		}
		return p
	}
	start := g.start(b)
	if slices.Index(v, b.top) < 0 {
		start = g.advanced(g.text(ci), -lo)
	}
	if b.scrolled && b.Marking { // the wheel only scrolls: the cursor is the range's end
		b.setStart(v, clamp(start)) // and scrolled stays set until the cursor moves
		return
	}
	if b.scrolled { // the wheel: the cursor's dragged along at the top or bottom edge
		start = clamp(start)
		b.setStart(v, start)
		first, last := -1, -1 // the lines wholly in sight
		for i := start.i; i < len(v) && g.dist(start, g.text(i), h) < h; i++ {
			if g.dist(start, g.text(i), h) >= lo && g.dist(start, g.end(i), h) <= h {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		switch {
		case first < 0: // one line fills the body
			ci = g.advanced(start, lo).i
		case ci < first:
			ci = first
		case ci > last:
			ci = last
		}
		b.SetCursor(v[ci])
		return
	}
	switch top, bot := g.dist(start, g.text(ci), h), g.dist(start, g.end(ci), h+1); {
	case top < lo: // above: its text just under the day row
		start = g.advanced(g.text(ci), -lo)
	case bot > h: // below: its last row at the bottom, or its first under the day row if it's taller
		start = g.advanced(g.end(ci), -h)
		if g.dist(start, g.text(ci), h) < lo {
			start = g.advanced(g.text(ci), -lo)
		}
	}
	b.setStart(v, clamp(start))
}

// awayArrow puts arrow in the space after the time that starts row.
func awayArrow(row, arrow string) string {
	return xansi.Cut(row, 0, browseTimeW) + theme.Paint(theme.LogAway, arrow) + xansi.Cut(row, browseTimeW+1, xansi.StringWidth(row))
}

// view draws the right pane (w×h) in browse mode. When a prompt is being
// typed, showCur is true and (curX, curY) is the cursor within the pane.
func (b *browse) view(w, h int) (rows []string, curX, curY int, showCur bool) {
	b.Items() // new tags take the filter already set
	v := b.Visible()
	bodyH := max(1, h-3)
	b.width = w
	b.scrollToCursor(v, bodyH, w)

	// Body.
	b.rowLines = b.rowLines[:0]
	if len(b.Lines) == 0 && b.HistDone {
		rows = append(rows, theme.Paint(theme.LogLoading, str.BrowseNoLogs()))
		b.rowLines = append(b.rowLines, nil)
	}
	ms := b.Matches()
	g := b.grid(v, w)
	start := g.start(b)
	if b.Loading && start == (rowPos{}) { // the oldest loaded line is at the top
		rows = append(rows, theme.Paint(theme.LogLoading, str.BrowseLoading()))
		b.rowLines = append(b.rowLines, nil)
	}
	dayRow := func(day string) string { return theme.Paint(theme.LogDay, dayRule+" "+app.DayLabel(day)+" "+dayRule) }
	ci := slices.Index(v, b.Cursor)
	firstRow, firstI, lastRow, lastI := -1, -1, -1, -1 // the rows starting the first and last lines whose text starts in sight, and their indexes
	for i := start.i; len(v) > 0 && i < len(v) && len(b.rowLines) < bodyH; i++ {
		l := v[i]
		skip := 0
		if i == start.i {
			skip = start.r
		}
		if g.rule(i) {
			if skip == 0 {
				rows = append(rows, dayRow(l.Day))
				b.rowLines = append(b.rowLines, nil)
				if len(b.rowLines) >= bodyH {
					break
				}
			} else {
				skip--
			}
		}
		ts := app.TimeOfDay(l.Entry.Time)
		if l == b.Cursor {
			ts = theme.Paint(theme.LogCursor, ts)
		} else {
			ts = theme.Paint(theme.LogTime, ts)
		}
		gutter := " "
		if b.InRange(l) {
			gutter = theme.Paint(theme.LogSelected, glyphSelected)
			if b.Excluded[l] {
				gutter = theme.Paint(theme.LogExcluded, glyphExcluded)
			}
		}
		text := b.text(l)
		if b.Find != "" && slices.Contains(ms, l) {
			text = highlightFind(ansi.Strip(b.text(l)), b.Find)
		}
		for j, row := range ansi.Wrap(text, max(1, w-browsePrefixW)) {
			if j < skip {
				continue
			}
			prefix := ts + " " + gutter + " "
			if j > 0 {
				prefix = "      " + gutter + " "
			} else if len(rows) > 0 || bodyH < 2 { // not under the day row
				if firstRow < 0 {
					firstRow, firstI = len(rows), i
				}
				lastRow, lastI = len(rows), i
			}
			rows = append(rows, prefix+row)
			b.rowLines = append(b.rowLines, l)
			if len(b.rowLines) >= bodyH {
				break
			}
		}
	}
	// The body's first row is always a day's: the top line's, over its
	// row, as the page's sticks at the top of the scroll.
	if len(rows) > 1 && b.rowLines[0] != nil {
		rows[0], b.rowLines[0] = dayRow(b.rowLines[0].Day), nil
	}
	// A cursor scrolled out of view (while marking: see scrollToCursor)
	// gets an arrow by the time of the line nearest it.
	switch {
	case ci >= 0 && ci < firstI && firstRow >= 0:
		rows[firstRow] = awayArrow(rows[firstRow], glyphAbove)
	case ci > lastI && lastRow >= 0:
		rows[lastRow] = awayArrow(rows[lastRow], glyphBelow)
	}
	// Too few lines to fill the body sit at its bottom, newest last, as
	// in the scrollback; "no logs yet" stays at the top.
	if pad := bodyH - len(rows); pad > 0 && len(b.Lines) > 0 {
		rows = append(make([]string, pad), rows...)
		b.rowLines = append(make([]*app.LogLine, pad), b.rowLines...)
	}
	for len(rows) < bodyH {
		rows = append(rows, "")
		b.rowLines = append(b.rowLines, nil)
	}

	// Action bar.
	rows = append(rows, theme.Paint(theme.Rule, strings.Repeat(glyph.Rule, w)))
	switch {
	case b.prompt == promptFormat:
		rows = append(rows, theme.Fill(theme.LogBar, b.promptLabel(), w))
	case b.prompt == promptFilterText: // typed in the panel; its keys here
		rows = append(rows, theme.Fill(theme.LogBar, theme.Paint(theme.LogHints, str.FilterFieldHint()), w))
	case b.prompt != promptNone:
		label := b.promptLabel()
		text, x := promptWindow([]rune(b.pin.Value()), b.pin.col, w-xansi.StringWidth(label)-1)
		rows = append(rows, theme.Fill(theme.LogBar, label+text, w))
		curX, curY, showCur = xansi.StringWidth(label)+x, h-2, true
	default: // messages go to the bottom bar
		hints := theme.LogHints
		if b.panelFocus { // the keys aren't the log's: faded
			hints = theme.LogHintsAway
		}
		rows = append(rows, theme.Fill(theme.LogBar, theme.Paint(hints, b.hints()), w))
	}
	rows = append(rows, theme.Paint(theme.RuleStatus, strings.Repeat(glyph.Rule, w)))
	return rows, curX, curY, showCur
}

// restyle repaints every line, after a theme change.
func (b *browse) restyle() { clear(b.painted) }

// highlightFind reverses every case-insensitive occurrence of needle.
func highlightFind(plain, needle string) string {
	if needle == "" {
		return plain
	}
	var out strings.Builder
	last := 0
	for _, m := range app.FindRE(needle).FindAllStringIndex(plain, -1) {
		out.WriteString(plain[last:m[0]] + theme.Paint(theme.LogFind, plain[m[0]:m[1]]))
		last = m[1]
	}
	out.WriteString(plain[last:])
	return out.String()
}

// selectChip is a click on the Select chip: s, as a key would be. It
// stops a search reading older days, and does nothing while a prompt is
// open, as a click on the log doesn't.
func (b *browse) selectChip() {
	b.StopSearch()
	if b.prompt != promptNone {
		return
	}
	b.ClearStatus()
	b.Mark()
}

// click is a left click on l, the line pressed (nil: none), as it's
// let go. Clicks are ignored while a prompt is open so the selection
// can't change under it.
func (b *browse) click(l *app.LogLine, shift bool) {
	b.StopSearch()
	if b.prompt != promptNone {
		return
	}
	b.focusPanel(false)
	if l == nil {
		return
	}
	// Outside select mode a click only moves the cursor. While selecting
	// it's a shift-click: inside the range it toggles its line, leaving
	// the cursor (the range's end) where it is; outside it grows the
	// range to the line. A shift-click inside a locked range (a drag's)
	// toggles its line too.
	switch {
	case b.InRange(l) && (b.Marking || shift):
		b.ToggleExclude(l)
	case b.Marking:
		b.GrowTo(l)
		b.SetCursor(l)
	default:
		b.SetCursor(l)
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
