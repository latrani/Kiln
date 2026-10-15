package kilntest

import (
	"fmt"
	"time"
)

// The golden scenarios' data, which both front ends build from: the
// TUI's text goldens (internal/ui) and the web's screenshots
// (web/smoke). Change it and both change. Its words are a fake server's
// and a fake person's, never Kiln's own.

//str:ok: test data, not Kiln's words
const (
	// GoldenWorld is fm with local echo on, so sent lines show.
	GoldenWorld = `host = "muck.test"
port = 8888
tls = true
use = ["fuzzball"]
login = "connect {name} {password}"
max_line_bytes = 20
local_echo = true

[[characters]]
id = "kit"
name = "Kit"
autoconnect = true

[[characters]]
id = "rook"
name = "Rook"
`

	scrolledLine = "line %d https://kiln.test/%d"

	// What a person types in the scenarios.
	InputText     = "this line runs past twenty bytes"
	MainTyped     = ":waves."
	PasswordTyped = "s3cret"
	FindTerm      = "Mira" // the page the Filter chip shows
)

//str:ok: test data, not Kiln's words
var (
	// GoldenStart is the clock when a scenario starts; each then moves it
	// a minute on, once Kit's connected.
	GoldenStart = time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)

	// GoldenPasswords are the saved passwords: Kit logs in by itself.
	GoldenPasswords = map[string]string{"fm/kit": "hunter2", "fm/rook": "pw"}

	// LogDay is when the log scene starts, a line a minute.
	LogDay = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

	// LogScene is Kit's log in the log scenarios.
	LogScene = []string{
		"Rook says, \"Evening!\"",
		"Sable waves a paw.",
		"Mira pages: you around?",
		"> :grins.",
		"Kit grins.",
		"Rook says, \"The lighthouse is dark.\"",
		"Rook yawns.",
	}

	// HistoryDay is the day before the scenarios, when Kit's log for the
	// history scenario starts.
	HistoryDay = time.Date(2026, 9, 23, 22, 0, 0, 0, time.Local)

	// HistoryLogged is Kit's log for the history scenario: what was sent
	// the day before, a login among it.
	HistoryLogged = []string{"Welcome to fm.", "> connect Kit ***", "> look", "> +finger Rook", "> home", "> say I'll be at the lighthouse."}

	// HistorySent is what Kit sends in the history scenario, a minute
	// apart, before opening the panel.
	HistorySent = []string{
		"page Rook = around?",
		":waves.",
		"say The lighthouse is dark tonight, and the tide is coming in fast.",
		"+where",
		"look",
		"say Evening, Ash.",
	}

	// MainLines are what Kit hears in main.
	MainLines = []string{"Rook says, \"Evening, Kit.\"", "See https://kiln.test/map for the way"}
)

// ScrolledLine is scrolled's line i of 40, each with a link.
func ScrolledLine(i int) string { return fmt.Sprintf(scrolledLine, i, i) }
