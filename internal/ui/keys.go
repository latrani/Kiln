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

// openBrowseKey opens log mode (browse, in the code) from the normal view.
const openBrowseKey = "ctrl+l"

// openPickerKey opens the open-connection picker (like /open).
const openPickerKey = "ctrl+o"

// openEditorKey opens the editor for the world or character highlighted
// in the picker, or from the main view for the active character (like
// /edit). Ctrl+E is the input's end of line.
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

// Export format keys, pressed after actExport.
var exportFormatKeys = map[string]string{"p": "plain", "a": "ansi", "h": "html"}

// Browse glyphs.
const (
	glyphSelected = "▌"
	glyphExcluded = "░"
	glyphChipOnly = "+"
	glyphChipHide = "−"
)
