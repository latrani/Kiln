// Package glyph is the symbols Kiln draws, in one place so every front
// end draws the same ones. Each is one cell wide and in WGL4 where it
// can be. None is text for people.
package glyph

//str:ok
const (
	Echo        = "›" // before a line you sent
	Search      = "/" // before the history panel's query, in its place
	Sys         = "*" // before Kiln's own lines
	Rule        = "─" // rules, and around a day's label
	Open        = "▼" // an open disclosure
	Collapsed   = ">" // a shut one
	Filtered    = "•" // a shut parent with a filter set under it
	Remove      = "×" // removes or closes what it's on
	Selected    = "▌" // a selected line in log mode
	Excluded    = "░" // a line taken out of the selection
	CursorAbove = "▲" // log mode's cursor is above the view
	CursorBelow = "▼" // and below it
	Priority    = "●" // a priority line
	ChoicePrev  = "◄" // a choice field's arrows, while it's focused
	ChoiceNext  = "►"
	Add         = "+" // before an add row
	Menu        = "≡" // the collapsed sidebar, at the top bar's left: opens it
	Collapse    = "‹" // collapses the sidebar, at its title's right
	Connecting  = "…" // a character still connecting
	TreeLine    = "│" // a tree's line down from under a ▼, past rows not its children
	TreeBranch  = "├" // and to a child with more after it
	TreeLast    = "└" // and to its last child
)

// All is every glyph by its constant's name, in order, for front ends
// that can't import Go (the page's generated module).
//
//str:ok
var All = [][2]string{
	{"Echo", Echo}, {"Search", Search}, {"Sys", Sys}, {"Rule", Rule}, {"Open", Open}, {"Collapsed", Collapsed},
	{"Filtered", Filtered}, {"Remove", Remove}, {"Selected", Selected}, {"Excluded", Excluded},
	{"CursorAbove", CursorAbove}, {"CursorBelow", CursorBelow},
	{"Priority", Priority}, {"ChoicePrev", ChoicePrev}, {"ChoiceNext", ChoiceNext}, {"Add", Add}, {"Menu", Menu}, {"Collapse", Collapse},
	{"Connecting", Connecting}, {"TreeLine", TreeLine}, {"TreeBranch", TreeBranch}, {"TreeLast", TreeLast},
}
