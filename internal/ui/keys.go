package ui

import "github.com/latrani/Kiln/internal/glyph"

// Log-mode (browse, in the code) key bindings and glyphs live here so they can be retuned in
// one place. Keys are tea.KeyPressMsg.String() values.

type browseAction int

const (
	actNone browseAction = iota
	actBack
	actUp
	actDown
	actPageUp
	actPageDown
	actTop
	actBottom
	actSelect
	actExclude
	actFind
	actNextMatch
	actPrevMatch
	actDate
	actExport
	actCopy
)

// openBrowseKey opens log mode (browse, in the code) from the normal view,
// and closes it again. Either way it repaints the whole screen too, as
// Ctrl+L does in most terminal programs.
const openBrowseKey = "ctrl+l"

// openPickerKey opens the open-connection picker (like /open).
const openPickerKey = "ctrl+o"

// openEditorKey opens the editor for the world or character highlighted
// in the picker, or from the main view for the active character (like
// /edit); in the editor it hides it, keeping the edits for the next time
// it opens (Esc drops them). Ctrl+E is the input's end of line.
const openEditorKey = "ctrl+t"

var browseKeys = map[string]browseAction{
	"esc":    actBack,
	"ctrl+c": actBack,
	"up":     actUp,
	"k":      actUp,
	"down":   actDown,
	"j":      actDown,
	"pgup":   actPageUp,
	"pgdown": actPageDown,
	"home":   actTop,
	"end":    actBottom,
	"s":      actSelect,
	"space":  actExclude,
	" ":      actExclude,
	"/":      actFind,
	"n":      actNextMatch,
	"N":      actPrevMatch,
	"g":      actDate,
	"e":      actExport,
	"c":      actCopy,
}

// openFilterKey moves log mode's keys between the log and its filter
// panel.
const openFilterKey = "f"

// sharedKeys are the keys both the log and the filter panel use; they go
// to whichever has focus. panelOnlyKeys are the panel's that the log
// doesn't use: they work from either side, as the log's own do.
var (
	sharedKeys    = map[string]bool{"up": true, "down": true, "k": true, "j": true, "space": true, " ": true, "esc": true, "ctrl+c": true, "enter": true}
	panelOnlyKeys = map[string]bool{"h": true, "o": true, "left": true, "right": true, "x": true, "delete": true}
)

// The sidebar's toggles, ≡ and ‹, are their glyph twice: one cell is a
// hard target on a phone. collapseW is ‹'s cells.
const (
	menuButton     = glyph.Menu + glyph.Menu
	collapseButton = glyph.Collapse + glyph.Collapse
	collapseW      = 2
)

// Filter panel glyphs: a parent's disclosure, the mark on a collapsed
// parent with a filter set under it, and the × that removes a text row.
const (
	glyphOpen       = glyph.Open
	glyphCollapsed  = glyph.Collapsed
	glyphFiltered   = glyph.Filtered
	glyphRemove     = glyph.Remove
	glyphTreeLine   = glyph.TreeLine
	glyphTreeBranch = glyph.TreeBranch
	glyphTreeLast   = glyph.TreeLast
)

// Export format keys, pressed after actExport.
var exportFormatKeys = map[string]string{"p": "plain", "a": "ansi", "h": "html"}

// Browse glyphs.
const (
	glyphSelected = glyph.Selected
	glyphExcluded = glyph.Excluded
	glyphAbove    = glyph.CursorAbove
	glyphBelow    = glyph.CursorBelow
)
