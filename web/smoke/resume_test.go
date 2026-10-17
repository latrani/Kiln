package smoke

import (
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// TestResumeKeepsTheCharacter kills the page's WebSockets to the relay
// without a word, as a phone dropping its network does: Kiln resumes the
// held connection, so the line typed meanwhile reaches the MUCK once and
// the MUCK never sees a second connection.
func TestResumeKeepsTheCharacter(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	ev(`kilnPage.act({Do: "submit", Key: "fake/tester", Text: "", N: 1000})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Input" && e.Key === "fake/tester" && e.NeedPW)`, "the password prompt")
	waitFor(t, ev, `document.activeElement === document.querySelector('.bottom input[type="password"]')`, "the password field focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "secret"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes("connect Tester")))`, "the login")
	accepted := muckAccepts.Load()

	ev(`kilnPage.dropLinks()`)
	waitFor(t, ev, `document.activeElement === document.querySelector(".bottom textarea")`, "the input focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "after the drop"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes('You say, "after the drop"')))`, "the echo")
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.startsWith(`+jsString(upTo(str.ConnResumed(mark)))+`)))`, "the resumed line")
	if n := ev(`kilnPage.events.filter(e => e.type === "Lines").flatMap(e => e.Lines).filter(l => l.Plain.includes('You say, "after the drop"')).length`); n != "1" {
		t.Errorf("the echo came %s times, want once", n)
	}
	if got := muckAccepts.Load(); got != accepted {
		t.Errorf("the MUCK took %d more connections, want none", got-accepted)
	}
}
