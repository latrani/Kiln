package ui

// Browse-mode key bindings and glyphs live here so they can be retuned in
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
)

// openBrowseKey opens browse mode from the normal view.
const openBrowseKey = "ctrl+b"

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
}

// Export format keys, pressed after actExport.
var exportFormatKeys = map[string]string{"p": "plain", "a": "ansi", "h": "html"}

// Browse glyphs.
const (
	glyphSelected = "▌"
	glyphExcluded = "░"
	glyphChipOnly = "+"
	glyphChipHide = "−"
	browseHints   = "m mark · space exclude · / find · n/N · g date · 1-9 tags · e export · c copy · esc back"
)
