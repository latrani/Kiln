// Package glyph is the symbols Kiln draws, in one place so every front
// end draws the same ones. Each is one cell wide and in WGL4 where it
// can be. None is text for people.
package glyph

//str:ok
const (
	Echo       = "›" // before a line you sent
	Sys        = "*" // before Kiln's own lines
	Rule       = "─" // rules, and around a day's label
	Open       = "▼" // an open disclosure
	Collapsed  = "►" // a shut one
	Filtered   = "•" // a shut parent with a filter set under it
	Remove     = "×" // removes or closes what it's on
	Selected   = "▌" // a selected line in log mode
	Excluded   = "░" // a line taken out of the selection
	Priority   = "●" // a priority line
	ChoicePrev = "◄" // a choice field's arrows, while it's focused
	ChoiceNext = "►"
	Add        = "+" // before an add row
	Menu       = "≡" // the web's sidebar menu
	Connecting = "…" // a character still connecting
)

// All is every glyph by its constant's name, in order, for front ends
// that can't import Go (the page's generated module).
//
//str:ok
var All = [][2]string{
	{"Echo", Echo}, {"Sys", Sys}, {"Rule", Rule}, {"Open", Open}, {"Collapsed", Collapsed},
	{"Filtered", Filtered}, {"Remove", Remove}, {"Selected", Selected}, {"Excluded", Excluded},
	{"Priority", Priority}, {"ChoicePrev", ChoicePrev}, {"ChoiceNext", ChoiceNext}, {"Add", Add}, {"Menu", Menu},
	{"Connecting", Connecting},
}
