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
	SidebarMore         Role = "sidebar.more"

	Picker              Role = "picker"
	PickerWorld         Role = "picker.world"
	PickerWorldSelected Role = "picker.world.selected"
	PickerSelected      Role = "picker.selected"
	PickerAdd           Role = "picker.add"

	Divider Role = "divider"
	Rule    Role = "rule"

	Scrollback           Role = "scrollback" // never an area: server text keeps the terminal background
	ScrollbackDay        Role = "scrollback.day"
	ScrollbackHistoryEnd Role = "scrollback.history_end"
	ScrollbackLoading    Role = "scrollback.loading"
	ScrollbackEcho       Role = "scrollback.echo"
	ScrollbackSys        Role = "scrollback.sys"
	ScrollbackPill       Role = "scrollback.pill"
	ScrollbackSelection  Role = "scrollback.selection"
	ScrollbackInactive   Role = "scrollback.inactive"
	Link                 Role = "link"
	LinkHover            Role = "link.hover"

	Input          Role = "input" // area
	InputHint      Role = "input.hint"
	InputOverLimit Role = "input.over_limit"
	InputSelection Role = "input.selection"

	Status      Role = "status" // area
	StatusLog   Role = "status.log"
	StatusError Role = "status.error"
	StatusClock Role = "status.clock"

	Form      Role = "form"
	FormLabel Role = "form.label"
	FormHint  Role = "form.hint"
	FormError Role = "form.error"
	FormFocus Role = "form.focus"
	FormTitle Role = "form.title"

	Log         Role = "log"
	LogHeader   Role = "log.header" // area
	LogTitle    Role = "log.header.title"
	LogChip     Role = "log.header.chip"
	LogChipOn   Role = "log.header.chip.on"
	LogTime     Role = "log.time"
	LogCursor   Role = "log.cursor"
	LogSelected Role = "log.selected"
	LogExcluded Role = "log.excluded"
	LogFind     Role = "log.find"
	LogDay      Role = "log.day"
	LogLoading  Role = "log.loading"
	LogBar      Role = "log.bar" // area
	LogHints    Role = "log.bar.hints"
	LogError    Role = "log.bar.error"

	Export Role = "export" // the HTML export's page colors
)

// Roles is every role, each after its parent.
var Roles = []Role{
	Sidebar, SidebarWorld, SidebarChar, SidebarActive, SidebarUnread, SidebarAttention,
	SidebarConnecting, SidebarDisconnected, SidebarAdd, SidebarMore,
	Picker, PickerWorld, PickerWorldSelected, PickerSelected, PickerAdd,
	Divider, Rule,
	Scrollback, ScrollbackDay, ScrollbackHistoryEnd, ScrollbackLoading, ScrollbackEcho, ScrollbackSys,
	ScrollbackPill, ScrollbackSelection, ScrollbackInactive, Link, LinkHover,
	Input, InputHint, InputOverLimit, InputSelection,
	Status, StatusLog, StatusError, StatusClock,
	Form, FormLabel, FormHint, FormError, FormFocus, FormTitle,
	Log, LogHeader, LogTitle, LogChip, LogChipOn, LogTime, LogCursor, LogSelected, LogExcluded,
	LogFind, LogDay, LogLoading, LogBar, LogHints, LogError,
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
