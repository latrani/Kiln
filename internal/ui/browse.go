package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/latrani/Kiln/internal/ansi"
	"github.com/latrani/Kiln/internal/history"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/scene"
	"github.com/latrani/Kiln/internal/style"
)

// browseInitialLines is how much history browse loads up front (whole
// days, newest first, until at least this many lines).
const browseInitialLines = 200

// browsePrefixW is the width of "HH:MM" + space + gutter + space.
const browsePrefixW = 8

type bline struct {
	e    logstore.Entry
	tags []string
	text string // sanitized and styled for display
	day  string // local "2006-01-02"
}

type promptKind int

const (
	promptNone promptKind = iota
	promptFind
	promptDate
	promptFormat
	promptFilename
)

// browse is one character's browse-mode state. Lines are referenced by
// pointer so paging in older history never disturbs marks or exclusions.
type browse struct {
	cs        *charState
	hist      *history.Reader
	lines     []*bline // oldest first
	cursor    *bline
	top       *bline // first line drawn at the top of the body
	start     *bline
	end       *bline
	excluded  map[*bline]bool
	chips     map[string]scene.Chip
	find      string
	prompt    promptKind
	pin       *Input
	format    string
	status    string
	statusErr bool
	rowLines  []*bline // body row → line (nil for dividers), from the last draw
	chipSpans []chipSpan
}

type chipSpan struct {
	tag      string
	from, to int // columns within the right pane
}

func newBrowse(cs *charState, logRoot string) *browse {
	b := &browse{cs: cs, excluded: map[*bline]bool{}, chips: map[string]scene.Chip{}, pin: NewInput()}
	if logRoot != "" {
		if h, err := history.NewReader(logRoot, cs.ch.World, cs.ch.ID); err == nil {
			b.hist = h
		}
	}
	for len(b.lines) < browseInitialLines && b.loadOlder() {
	}
	b.cursor = b.last()
	return b
}

func (b *browse) newLine(e logstore.Entry) *bline {
	text, _ := b.cs.render(e)
	plain := ansi.Strip(ansi.Sanitize(e.Text))
	var tags []string
	if e.Dir == logstore.In {
		tags = b.cs.cls.Classify(plain)
	}
	return &bline{e: e, tags: tags, text: text, day: e.Time.Local().Format("2006-01-02")}
}

// loadOlder prepends the next older day. It reports false when there is
// nothing more to load.
func (b *browse) loadOlder() bool {
	if b.hist == nil || b.hist.Exhausted() {
		return false
	}
	es, _, ok, err := b.hist.LoadOlder()
	if err != nil {
		b.setStatus(true, "reading logs: %v", err)
	}
	if !ok {
		return false
	}
	older := make([]*bline, 0, len(es))
	for _, e := range es {
		older = append(older, b.newLine(e))
	}
	b.lines = append(older, b.lines...)
	return true
}

// appendLive adds a line that arrived while browsing. A line that is
// already the newest loaded one (logged just before browse opened) is
// skipped.
func (b *browse) appendLive(e logstore.Entry) {
	if l := b.last(); l != nil && l.e.Time.Equal(e.Time) && l.e.Text == e.Text {
		return
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

func (b *browse) setStatus(isErr bool, format string, args ...any) {
	b.status, b.statusErr = fmt.Sprintf(format, args...), isErr
}

func (b *browse) visible() []*bline {
	out := make([]*bline, 0, len(b.lines))
	for _, l := range b.lines {
		if scene.Visible(l.tags, b.chips) {
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

// tagList is every tag in the loaded lines, sorted, for the chip row.
func (b *browse) tagList() []string {
	seen := map[string]bool{}
	for _, l := range b.lines {
		for _, t := range l.tags {
			seen[t] = true
		}
	}
	tags := make([]string, 0, len(seen))
	for t := range seen {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// moveCursor moves by delta visible lines, paging in older history when
// moving up past the oldest loaded line.
func (b *browse) moveCursor(delta int) {
	v := b.visible()
	if len(v) == 0 {
		return
	}
	i := slices.Index(v, b.cursor)
	if i < 0 {
		i = len(v) - 1
	}
	for i+delta < 0 && b.loadOlder() {
		nv := b.visible()
		i += len(nv) - len(v)
		v = nv
	}
	i = min(max(0, i+delta), len(v)-1)
	b.cursor = v[i]
}

func (b *browse) mark() {
	if b.cursor == nil {
		return
	}
	if b.start == nil || b.end != nil {
		b.start, b.end = b.cursor, nil
		b.setStatus(false, "range start marked; m again at the end")
		return
	}
	b.end = b.cursor
	if b.index(b.end) < b.index(b.start) {
		b.start, b.end = b.end, b.start
	}
	b.setStatus(false, "%d lines in range", b.index(b.end)-b.index(b.start)+1)
}

func (b *browse) toggleExclude(l *bline) {
	if l == nil || b.end == nil || !b.inRange(l) {
		b.setStatus(true, "exclude works inside a marked range")
		return
	}
	b.excluded[l] = !b.excluded[l]
}

func (b *browse) matches() []*bline {
	if b.find == "" {
		return nil
	}
	needle := strings.ToLower(b.find)
	var out []*bline
	for _, l := range b.visible() {
		if strings.Contains(strings.ToLower(ansi.Strip(l.text)), needle) {
			out = append(out, l)
		}
	}
	return out
}

// jumpMatch moves to the next (dir=1) or previous (dir=-1) match,
// wrapping around. With from=true the cursor's own line counts.
func (b *browse) jumpMatch(dir int, includeCursor bool) {
	ms := b.matches()
	if len(ms) == 0 {
		b.setStatus(true, "no matches for %q", b.find)
		return
	}
	ci := b.index(b.cursor)
	pick := -1
	if dir > 0 {
		for k, m := range ms {
			if i := b.index(m); i > ci || (includeCursor && i == ci) {
				pick = k
				break
			}
		}
		if pick < 0 {
			pick = 0
		}
	} else {
		for k := len(ms) - 1; k >= 0; k-- {
			if b.index(ms[k]) < ci {
				pick = k
				break
			}
		}
		if pick < 0 {
			pick = len(ms) - 1
		}
	}
	b.cursor = ms[pick]
	b.status = ""
}

func (b *browse) matchPos() (int, int) {
	ms := b.matches()
	return slices.Index(ms, b.cursor) + 1, len(ms)
}

// gotoDate loads history back to day and moves to its first visible line.
func (b *browse) gotoDate(day string) {
	if _, err := time.Parse("2006-01-02", day); err != nil {
		b.setStatus(true, "dates look like 2026-09-24")
		return
	}
	for (len(b.lines) == 0 || b.lines[0].day > day) && b.loadOlder() {
	}
	for _, l := range b.visible() {
		if l.day >= day {
			b.cursor = l
			b.top = l
			b.status = ""
			return
		}
	}
	b.setStatus(true, "no logs on or after %s", day)
}

// selection is what an export contains: received lines inside the range,
// not excluded and not hidden by chips.
func (b *browse) selection() []logstore.Entry {
	if b.start == nil || b.end == nil {
		return nil
	}
	var out []logstore.Entry
	for _, l := range b.lines[b.index(b.start) : b.index(b.end)+1] {
		if !b.excluded[l] && scene.Visible(l.tags, b.chips) && scene.Exportable(l.e) {
			out = append(out, l.e)
		}
	}
	return out
}

func (b *browse) title() string {
	ch := b.cs.ch
	when := ""
	if b.start != nil {
		when = " — " + b.start.e.Time.Local().Format("Mon Jan 2 2006")
	}
	return ch.World + " " + ch.Name + when
}

// save writes the export to path, refusing to overwrite.
func (b *browse) save(path string) {
	sel := b.selection()
	if len(sel) == 0 {
		b.setStatus(true, "nothing to export")
		return
	}
	if _, err := os.Stat(path); err == nil {
		b.setStatus(true, "%s exists; pick another name", filepath.Base(path))
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		b.setStatus(true, "%v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.setStatus(true, "%v", err)
		return
	}
	if err := os.WriteFile(path, []byte(scene.Render(b.format, sel, b.title())), 0o644); err != nil {
		b.setStatus(true, "%v", err)
		return
	}
	b.setStatus(false, "saved %s", path)
}

// key handles a key press in browse mode. It returns (cmd, close).
func (b *browse) key(k tea.KeyPressMsg, exportDir string, pageH int) (tea.Cmd, bool) {
	s := k.String()
	if b.prompt != promptNone {
		return b.promptKey(k, exportDir), false
	}
	if len(s) == 1 && s >= "1" && s <= "9" {
		tags := b.tagList()
		if i := int(s[0] - '1'); i < len(tags) {
			b.cycleChip(tags[i])
		}
		return nil, false
	}
	b.status = ""
	switch browseKeys[s] {
	case actBack:
		return nil, true
	case actUp:
		b.moveCursor(-1)
	case actDown:
		b.moveCursor(1)
	case actPageUp:
		b.moveCursor(-max(1, pageH-1))
	case actPageDown:
		b.moveCursor(max(1, pageH-1))
	case actTop:
		for b.loadOlder() {
		}
		if v := b.visible(); len(v) > 0 {
			b.cursor = v[0]
		}
	case actBottom:
		b.cursor = b.lastVisible()
	case actMark:
		b.mark()
	case actExclude:
		b.toggleExclude(b.cursor)
	case actFind:
		b.prompt = promptFind
		b.pin.SetValue(b.find)
	case actNextMatch:
		b.jumpMatch(1, false)
	case actPrevMatch:
		b.jumpMatch(-1, false)
	case actDate:
		b.prompt = promptDate
		b.pin.SetValue("")
	case actExport:
		if len(b.selection()) == 0 {
			b.setStatus(true, "mark a range with m (only received lines export)")
			break
		}
		b.prompt = promptFormat
	case actCopy:
		sel := b.selection()
		if len(sel) == 0 {
			b.setStatus(true, "mark a range with m (only received lines export)")
			break
		}
		b.setStatus(false, "copied %d lines", len(sel))
		return tea.SetClipboard(scene.Plain(sel)), false
	}
	return nil, false
}

func (b *browse) cycleChip(tag string) {
	b.chips[tag] = b.chips[tag].Next()
	if b.cursor != nil && !scene.Visible(b.cursor.tags, b.chips) {
		b.moveCursor(0)
		if v := b.visible(); len(v) > 0 && !slices.Contains(v, b.cursor) {
			b.cursor = v[len(v)-1]
		}
	}
}

func (b *browse) promptKey(k tea.KeyPressMsg, exportDir string) tea.Cmd {
	s := k.String()
	if s == "esc" {
		b.prompt = promptNone
		return nil
	}
	if b.prompt == promptFormat {
		if f, ok := exportFormatKeys[s]; ok {
			b.format = f
			b.prompt = promptFilename
			first := b.selection()[0]
			b.pin.SetValue(scene.FileName(exportDir, first.Time.Local(), b.cs.ch.World, b.cs.ch.Name, f))
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
			if v != "" {
				b.jumpMatch(1, true)
			}
		case promptDate:
			b.gotoDate(v)
		case promptFilename:
			b.save(v)
		}
	case "backspace":
		b.pin.Backspace()
	case "left":
		b.pin.Left()
	case "right":
		b.pin.Right()
	case "home", "ctrl+a":
		b.pin.Home()
	case "end", "ctrl+e":
		b.pin.End()
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			b.pin.InsertText(k.Text)
		}
	}
	return nil
}

// promptText is the action bar's prompt label.
func (b *browse) promptLabel() string {
	switch b.prompt {
	case promptFind:
		return "find: "
	case promptDate:
		return "go to date (YYYY-MM-DD): "
	case promptFormat:
		return "export as (p)lain · (a)nsi · (h)tml   esc cancel"
	case promptFilename:
		return "save as: "
	}
	return ""
}

// rowsFor is how many body rows a line takes, including a day divider.
func (b *browse) rowsFor(l *bline, prev *bline, w int) int {
	n := len(ansi.Wrap(l.text, max(1, w-browsePrefixW)))
	if prev == nil || prev.day != l.day {
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
	rows := func(i int) int {
		var prev *bline
		if i > 0 {
			prev = v[i-1]
		}
		return b.rowsFor(v[i], prev, w)
	}
	ti := slices.Index(v, b.top)
	if ti < 0 || ci < ti {
		ti = ci
	}
	if ci-ti > h { // every line takes at least one row
		ti = ci - h
	}
	for ti < ci {
		n := 0
		for i := ti; i <= ci; i++ {
			n += rows(i)
		}
		if n <= h {
			break
		}
		ti++
	}
	// The earliest top that still fits everything through the last line.
	fill, n := len(v)-1, 0
	for ; fill >= 0; fill-- {
		if n+rows(fill) > h {
			break
		}
		n += rows(fill)
	}
	if fill+1 < ti {
		ti = fill + 1
	}
	b.top = v[ti]
}

// view draws the right pane (w×h) in browse mode. When a prompt is being
// typed, showCur is true and (curX, curY) is the cursor within the pane.
func (b *browse) view(w, h int) (rows []string, curX, curY int, showCur bool) {
	v := b.visible()
	bodyH := max(1, h-5)
	b.scrollToCursor(v, bodyH, w)

	// Header row 1.
	span := "no logs yet"
	if len(b.lines) > 0 {
		span = dayLabel(b.lines[0].day) + " → today"
		if b.hist != nil && !b.hist.Exhausted() {
			span = "…" + span
		}
	}
	head := bold + "BROWSE " + b.cs.ch.Name + style.Reset + " · " + span
	if b.find != "" {
		i, n := b.matchPos()
		head += fmt.Sprintf("   find: %s %d/%d", b.find, i, n)
	}
	// Header row 2: chips.
	chipRow := "tags:"
	b.chipSpans = b.chipSpans[:0]
	for i, t := range b.tagList() {
		label := t
		switch b.chips[t] {
		case scene.Only:
			label = glyphChipOnly + t
		case scene.Hide:
			label = glyphChipHide + t
		}
		chip := fmt.Sprintf(" %d[%s]", i+1, label)
		from := xansi.StringWidth(chipRow) + 1
		chipRow += chip
		b.chipSpans = append(b.chipSpans, chipSpan{tag: t, from: from, to: xansi.StringWidth(chipRow)})
		if b.chips[t] != scene.Neutral {
			chipRow = chipRow[:len(chipRow)-len(chip)] + " " + reverse + chip[1:] + style.Reset
		}
	}
	rows = []string{head, chipRow, style.Dim(strings.Repeat("─", w))}

	// Body.
	b.rowLines = b.rowLines[:0]
	ms := b.matches()
	start := slices.Index(v, b.top)
	for i := max(0, start); i < len(v) && len(b.rowLines) < bodyH; i++ {
		l := v[i]
		if i == 0 || v[i-1].day != l.day {
			rows = append(rows, style.Dim("── "+dayLabel(l.day)+" ──"))
			b.rowLines = append(b.rowLines, nil)
			if len(b.rowLines) >= bodyH {
				break
			}
		}
		ts := l.e.Time.Local().Format("15:04")
		if l == b.cursor {
			ts = reverse + ts + style.Reset
		} else {
			ts = style.Dim(ts)
		}
		gutter := " "
		if b.inRange(l) {
			gutter = glyphSelected
			if b.excluded[l] {
				gutter = glyphExcluded
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
	for len(rows) < 3+bodyH {
		rows = append(rows, "")
		b.rowLines = append(b.rowLines, nil)
	}

	// Action bar.
	rows = append(rows, style.Dim(strings.Repeat("─", w)))
	switch {
	case b.prompt == promptFormat:
		rows = append(rows, b.promptLabel())
	case b.prompt != promptNone:
		label := b.promptLabel()
		text, x := promptWindow([]rune(b.pin.Value()), b.pin.col, w-xansi.StringWidth(label)-1)
		rows = append(rows, label+text)
		curX, curY, showCur = xansi.StringWidth(label)+x, h-1, true
	case b.status != "":
		msg := b.status
		if b.statusErr {
			msg = red + msg + style.Reset
		}
		rows = append(rows, msg)
	default:
		rows = append(rows, style.Dim(browseHints))
	}
	return rows, curX, curY, showCur
}

// dayLabel formats "2026-09-24" as "Thu Sep 24".
func dayLabel(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format("Mon Jan 2")
}

// highlightFind reverses every case-insensitive occurrence of needle.
func highlightFind(plain, needle string) string {
	if needle == "" {
		return plain
	}
	lower, ln := strings.ToLower(plain), strings.ToLower(needle)
	var out strings.Builder
	for {
		i := strings.Index(lower, ln)
		if i < 0 || len(ln) == 0 {
			out.WriteString(plain)
			return out.String()
		}
		out.WriteString(plain[:i] + reverse + plain[i:i+len(ln)] + style.Reset)
		plain, lower = plain[i+len(ln):], lower[i+len(ln):]
	}
}

// click handles a left click at (x, y) within the right pane.
func (b *browse) click(x, y int, shift bool) {
	if y == 1 {
		for _, c := range b.chipSpans {
			if x >= c.from && x < c.to {
				b.cycleChip(c.tag)
			}
		}
		return
	}
	row := y - 3
	if row < 0 || row >= len(b.rowLines) || b.rowLines[row] == nil {
		return
	}
	l := b.rowLines[row]
	switch {
	case shift:
		if b.start == nil {
			b.start = b.cursor
		}
		b.cursor = l
		b.end = nil
		b.mark()
	case x == browsePrefixW-2: // the gutter column
		b.toggleExclude(l)
	default:
		b.cursor = l
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
