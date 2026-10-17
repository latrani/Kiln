package smoke

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/str"
)

// TestResumeKeepsTheCharacter kills the page's WebSockets to the relay
// without a word, as a phone dropping its network does: Kiln resumes the
// held connection, so the line typed meanwhile reaches the MUCK once and
// the MUCK never sees a second connection.
func TestResumeKeepsTheCharacter(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	logIn(t, ws, ev)
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

// logIn opens and connects the fake world's Tester and answers the
// password prompt, as a person does.
func logIn(t *testing.T, ws *websocket.Conn, ev func(string) string) {
	t.Helper()
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	ev(`kilnPage.act({Do: "submit", Key: "fake/tester", Text: "", N: 1000})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Input" && e.Key === "fake/tester" && e.NeedPW)`, "the password prompt")
	waitFor(t, ev, `document.activeElement === document.querySelector('.bottom input[type="password"]')`, "the password field focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "secret"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes("connect Tester")))`, "the login")
}

// say types line into the input and sends it.
func say(t *testing.T, ws *websocket.Conn, ev func(string) string, line string) {
	t.Helper()
	waitFor(t, ev, `document.activeElement === document.querySelector(".bottom textarea")`, "the input focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": line})
	pressKey(t, ws, "Enter", 13)
}

// shownOnce waits until the scrollback shows each of texts, then checks
// it shows each once.
func shownOnce(t *testing.T, ev func(string) string, texts ...string) {
	t.Helper()
	for _, s := range texts {
		waitFor(t, ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes(`+jsString(s)+`))`, s)
	}
	for _, s := range texts {
		if n := ev(`[...document.querySelectorAll("main .line")].filter(l => l.textContent.includes(` + jsString(s) + `)).length`); n != "1" {
			t.Errorf("%q shows %s times, want once", s, n)
		}
	}
}

// A reload while lines arrive picks the character back up: each line
// once, no second log-in, the MUCK never sees another connection.
func TestReloadResumesTheCharacter(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	logIn(t, ws, ev)
	accepted := muckAccepts.Load()
	say(t, ws, ev, "later 3 after 1500")
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes('You say, "later 3 after 1500"')))`, "the echo")
	time.Sleep(1200 * time.Millisecond) // a save has landed; the lines come as the page reloads
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond) // the old page is gone
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the reloaded page's first batch")
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	shownOnce(t, ev, "late 1:", "late 2:", "late 3:", "connect Tester")
	waitFor(t, ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.startsWith(`+jsString(upTo(str.ConnResumed(mark)))+`))`, "the resumed line")
	if got := muckAccepts.Load(); got != accepted {
		t.Errorf("the MUCK took %d more connections, want none", got-accepted)
	}
}

// Use here hands the character to the new tab without dropping it.
func TestHandoffResumesTheCharacter(t *testing.T) {
	ws, another := launch(t)
	ev := evaluator(t, ws)
	logIn(t, ws, ev)
	accepted := muckAccepts.Load()
	say(t, ws, ev, "later 3 after 400")
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes('You say, "later 3 after 400"')))`, "the echo")
	ws2 := another()
	ev2 := evaluator(t, ws2)
	waitFor(t, ev2, `[...document.querySelectorAll("main button")].some(b => b.textContent === `+jsString(str.WebUseHere())+`)`, "the second tab offering to take over")
	ev2(`[...document.querySelectorAll("main button")].find(b => b.textContent === ` + jsString(str.WebUseHere()) + `).click()`)
	waitFor(t, ev2, `kilnPage && document.querySelector("main button") === null`, "the second tab taking over")
	waitFor(t, ev2, `document.querySelector("nav.side")`, "the new tab running") // not ?record: no events
	ev2(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	shownOnce(t, ev2, "late 1:", "late 2:", "late 3:", "connect Tester")
	if got := muckAccepts.Load(); got != accepted {
		t.Errorf("the MUCK took %d more connections, want none", got-accepted)
	}
}

// A tab that really closes tells the relay, which lets the character go
// after short_hold.
func TestClosedTabLetsGoSoon(t *testing.T) {
	relaySettings = "short_hold = \"1s\"\n"
	t.Cleanup(func() { relaySettings = "" })
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	logIn(t, ws, ev)
	ended := muckEnded.Load()
	// Closing the tab (not navigating, which can keep it in the back-
	// forward cache). Its DevTools socket goes with it, so no answer.
	ws.Write(context.Background(), websocket.MessageText, []byte(`{"id":1,"method":"Page.close"}`))
	deadline := time.Now().Add(5 * time.Second)
	for muckEnded.Load() == ended {
		if time.Now().After(deadline) {
			t.Fatal("the closed tab's character was still held after 5 s")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
