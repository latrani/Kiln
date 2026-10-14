package kilntest

import (
	"fmt"
	"time"
)

// The golden scenarios' data, which both front ends build from: the
// TUI's text goldens (internal/ui) and the web's screenshots
// (web/smoke). Change it and both change.

// GoldenWorld is fm with local echo on, so sent lines show.
const GoldenWorld = `host = "muck.test"
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

// GoldenStart is the clock when a scenario starts; each then moves it a
// minute on, once Kit's connected.
var GoldenStart = time.Date(2026, 9, 24, 21, 14, 0, 0, time.Local)

// GoldenPasswords are the saved passwords: Kit logs in by itself.
var GoldenPasswords = map[string]string{"fm/kit": "hunter2", "fm/rook": "pw"}

// LogDay is when the log scene starts, a line a minute.
var LogDay = time.Date(2026, 9, 24, 21, 0, 0, 0, time.Local)

// LogScene is Kit's log in the log scenarios.
var LogScene = []string{
	"Rook says, \"Evening!\"",
	"Sable waves a paw.",
	"Mira pages: you around?",
	"> :grins.",
	"Kit grins.",
	"Rook says, \"The lighthouse is dark.\"",
	"Rook yawns.",
}

// MainLines are what Kit hears in main.
var MainLines = []string{"Rook says, \"Evening, Kit.\"", "See https://kiln.test/map for the way"}

// ScrolledLine is scrolled's line i of 40, each with a link.
func ScrolledLine(i int) string { return fmt.Sprintf("line %d https://kiln.test/%d", i, i) }

// What a person types in the scenarios.
const (
	InputText     = "this line runs past twenty bytes"
	MainTyped     = ":waves."
	PasswordTyped = "s3cret"
	FindTerm      = "Mira" // the page the Filter chip shows
)
