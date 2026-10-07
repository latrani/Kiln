package theme

// Role names a part of the screen that a theme styles. Role names are
// API: theme files refer to them. A role inherits every style field of
// its parent (the name up to its last dot) and overrides what it sets.
type Role string

// The roles. Their names are theme vocabulary, not text for people.
//
//str:ok
const (
	Sidebar             Role = "sidebar" // area
	SidebarWorld        Role = "sidebar.world"
	SidebarChar         Role = "sidebar.char"
	SidebarActive       Role = "sidebar.active"
	SidebarUnread       Role = "sidebar.unread"
	SidebarAttention    Role = "sidebar.attention"
	SidebarConnecting   Role = "sidebar.connecting"
	SidebarDisconnected Role = "sidebar.disconnected"
	SidebarAdd          Role = "sidebar.add"
	SidebarAction       Role = "sidebar.action" // Back up and Restore, the web build's sidebar footer
	SidebarMore         Role = "sidebar.more"

	Picker              Role = "picker" // area, in place of sidebar while it's open
	PickerWorld         Role = "picker.world"
	PickerWorldSelected Role = "picker.world.selected"
	PickerSelected      Role = "picker.selected"
	PickerAdd           Role = "picker.add"

	Divider    Role = "divider"
	Rule       Role = "rule"
	RuleInput  Role = "rule.input"  // above the input area
	RuleForm   Role = "rule.form"   // above it while a form is up
	RuleStatus Role = "rule.status" // above the statusline

	Scrollback           Role = "scrollback" // never an area: server text keeps the terminal background
	ScrollbackDay        Role = "scrollback.day"
	ScrollbackHistoryEnd Role = "scrollback.history_end"
	ScrollbackLoading    Role = "scrollback.loading"
	ScrollbackEcho       Role = "scrollback.echo"
	ScrollbackSys        Role = "scrollback.sys"
	ScrollbackPill       Role = "scrollback.pill"
	ScrollbackSelection  Role = "scrollback.selection"
	ScrollbackInactive   Role = "scrollback.inactive"
	ScrollbackEmpty      Role = "scrollback.empty"    // the hint in an empty pane
	ScrollbackOverview   Role = "scrollback.overview" // a character's name in a world's overview
	ScrollbackRule       Role = "scrollback.rule"     // the line between characters in a world's overview
	Link                 Role = "link"
	LinkHover            Role = "link.hover"

	Input          Role = "input" // area
	InputHint      Role = "input.hint"
	InputOverLimit Role = "input.over_limit"
	InputSelection Role = "input.selection"

	Status         Role = "status"     // area
	StatusLog      Role = "status.log" // the Log chip
	StatusLogOn    Role = "status.log.on"
	StatusFilter   Role = "status.filter" // the Filter chip, in log mode
	StatusFilterOn Role = "status.filter.on"
	StatusError    Role = "status.error"
	// The presence chip: here, away, or can't tell (no focus events seen).
	StatusPresence        Role = "status.presence"
	StatusPresenceHere    Role = "status.presence.here"
	StatusPresenceAway    Role = "status.presence.away"
	StatusPresenceUnknown Role = "status.presence.unknown"

	Form                Role = "form" // area, in place of input while it's up
	FormLabel           Role = "form.label"
	FormHint            Role = "form.hint"
	FormError           Role = "form.error"
	FormFocus           Role = "form.focus"
	FormTitle           Role = "form.title"
	FormButton          Role = "form.button" // the primary one: Save
	FormButtonSecondary Role = "form.button.secondary"

	Log         Role = "log"
	LogTime     Role = "log.time"
	LogCursor   Role = "log.cursor"
	LogSelected Role = "log.selected"
	LogExcluded Role = "log.excluded"
	LogFind     Role = "log.find"
	LogDay      Role = "log.day"
	LogLoading  Role = "log.loading"
	LogBar      Role = "log.bar" // area
	LogHints    Role = "log.bar.hints"

	Filter         Role = "filter" // area, in place of sidebar while log mode's filter panel is open
	FilterItem     Role = "filter.item"
	FilterSelected Role = "filter.selected"
	FilterButton   Role = "filter.button"
	FilterButtonOn Role = "filter.button.on"
	FilterAdd      Role = "filter.add"
	FilterMore     Role = "filter.more"

	Export Role = "export" // the HTML export's page colors
)

// Roles is every role, each after its parent.
var Roles = []Role{
	Sidebar, SidebarWorld, SidebarChar, SidebarActive, SidebarUnread, SidebarAttention,
	SidebarConnecting, SidebarDisconnected, SidebarAdd, SidebarAction, SidebarMore,
	Picker, PickerWorld, PickerWorldSelected, PickerSelected, PickerAdd,
	Divider, Rule, RuleInput, RuleForm, RuleStatus,
	Scrollback, ScrollbackDay, ScrollbackHistoryEnd, ScrollbackLoading, ScrollbackEcho, ScrollbackSys,
	ScrollbackPill, ScrollbackSelection, ScrollbackInactive, ScrollbackEmpty, ScrollbackOverview, ScrollbackRule, Link, LinkHover,
	Input, InputHint, InputOverLimit, InputSelection,
	Status, StatusLog, StatusLogOn, StatusFilter, StatusFilterOn, StatusError, StatusPresence, StatusPresenceHere, StatusPresenceAway, StatusPresenceUnknown,
	Form, FormLabel, FormHint, FormError, FormFocus, FormTitle, FormButton, FormButtonSecondary,
	Log, LogTime, LogCursor, LogSelected, LogExcluded,
	LogFind, LogDay, LogLoading, LogBar, LogHints,
	Filter, FilterItem, FilterSelected, FilterButton, FilterButtonOn, FilterAdd, FilterMore,
	Export,
}

// parent is r's parent role, or "" for a top-level one.
func (r Role) parent() Role {
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == '.' {
			return r[:i]
		}
	}
	return ""
}
