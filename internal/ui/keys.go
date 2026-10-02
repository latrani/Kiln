package ui

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
	actMark
	actExclude
	actFind
	actNextMatch
	actPrevMatch
	actDate
	actExport
	actCopy
	actTags
)

// openBrowseKey opens log mode (browse, in the code) from the normal view,
// and closes it again.
const openBrowseKey = "ctrl+l"

// openPickerKey opens the open-connection picker (like /open).
const openPickerKey = "ctrl+o"

// openEditorKey opens the editor for the world or character highlighted
// in the picker, or from the main view for the active character (like
// /edit); in the editor it closes it unsaved, like Esc. Ctrl+E is the
// input's end of line.
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
	"m":      actMark,
	"space":  actExclude,
	" ":      actExclude,
	"/":      actFind,
	"n":      actNextMatch,
	"N":      actPrevMatch,
	"g":      actDate,
	"e":      actExport,
	"c":      actCopy,
	"t":      actTags,
}

// openFilterKey opens and closes log mode's filter panel.
const openFilterKey = "f"

// Filter panel glyphs: a parent's disclosure, the mark on a collapsed
// parent with a filter set under it, and the × that removes a text row.
const (
	glyphOpen      = "▼"
	glyphCollapsed = "►"
	glyphFiltered  = "•"
	glyphRemove    = "×"
)

// Export format keys, pressed after actExport.
var exportFormatKeys = map[string]string{"p": "plain", "a": "ansi", "h": "html"}

// Browse glyphs.
const (
	glyphSelected = "▌"
	glyphExcluded = "░"
)
