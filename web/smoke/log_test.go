package smoke

import (
	"fmt"
	"testing"

	"github.com/coder/websocket"
	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/kilntest"
	"github.com/latrani/Kiln/internal/logstore"
	"github.com/latrani/Kiln/internal/str"
)

// ctrlL presses Ctrl+L: log mode shown or hidden.
func ctrlL(t *testing.T, ws *websocket.Conn) {
	t.Helper()
	for _, typ := range []string{"rawKeyDown", "keyUp"} {
		devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "l", "code": "KeyL", "windowsVirtualKeyCode": 76, "modifiers": 2})
	}
}

// TestLog uses log mode the way a person does: Ctrl+L, a click and a
// shift-click, find, Esc out of a prompt, copy, export (a download), and
// Ctrl+L hiding it as it was.
func TestLog(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")

	// Second, connected, with a line or two from the fake MUCK.
	ctrlO(t, ws)
	filter := `document.querySelector('.bottom input[role="combobox"]')`
	waitFor(t, ev, `document.activeElement === `+filter, "the filter")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Key === "fake/second" && e.Lines.some(l => l.Kind === "server"))`, "Second's lines")

	// Ctrl+L: log mode, focused, its hints below, the chip lit.
	ctrlL(t, ws)
	logEl := `document.querySelector(".log")`
	waitFor(t, ev, `document.activeElement === `+logEl+` && document.querySelector(".lrow")`, "log mode, focused, with lines")
	waitFor(t, ev, `document.querySelector(".bottom").textContent === `+jsString(str.WebLogHints()), "the hints")

	// / opens find; typing goes to the prompt, not log mode's letters;
	// Enter finds; Esc in a prompt closes only the prompt.
	ev(logEl + `.focus()`)
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "/", "text": "/", "code": "Slash", "windowsVirtualKeyCode": 191})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "/", "code": "Slash", "windowsVirtualKeyCode": 191})
	waitFor(t, ev, `document.activeElement === document.querySelector("#kiln-logprompt")`, "find's prompt")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "fake muck"})
	if got := ev(`kilnPage.sent.some(a => a.Do === "find_older" || a.Do === "copy")`); got != "false" {
		t.Error("typing in the prompt ran log mode's letters")
	}
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "LogAt" && e.Key === "fake/second") && document.querySelector("mark")`, "found, marked")
	waitFor(t, ev, `document.querySelector("header").textContent.includes(`+jsString(str.BrowseFindStatus("fake muck", 1, 1))+`)`, "the find status: one match, today's the only day")

	// Esc in find's prompt closes the prompt alone: log mode stays, focused.
	waitFor(t, ev, `document.activeElement === `+logEl, "the log focused again")
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "/", "text": "/", "code": "Slash", "windowsVirtualKeyCode": 191})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "/", "code": "Slash", "windowsVirtualKeyCode": 191})
	waitFor(t, ev, `document.activeElement === document.querySelector("#kiln-logprompt")`, "find's prompt again")
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector("#kiln-logprompt") && document.activeElement === `+logEl, "the prompt closed, the log focused")
	if got := ev(`kilnPage.sent.some(a => a.Do === "log_close")`); got != "false" {
		t.Error("Esc in a prompt closed log mode")
	}

	// A click selects a line (and is where find goes from next: find
	// searches older from where you are, as the TUI's); a shift-click on
	// another extends the range.
	rows := `[...document.querySelectorAll(".lrow")]`
	click(t, ws, ev, rows+`[0]`)
	waitFor(t, ev, rows+`[0].querySelector(".gutter").textContent === `+jsString(glyph.Selected), "the first line selected")
	shiftClick(t, ws, ev, rows+`.at(-1)`)
	waitFor(t, ev, rows+`.every(r => r.querySelector(".gutter").textContent === `+jsString(glyph.Selected)+`)`, "the range runs to the last line")
	// A shift-click inside the range takes that line out, and again puts
	// it back.
	shiftClick(t, ws, ev, rows+`[1]`)
	waitFor(t, ev, rows+`[1].querySelector(".gutter").textContent === `+jsString(glyph.Excluded), "the middle line taken out")
	shiftClick(t, ws, ev, rows+`[1]`)
	waitFor(t, ev, rows+`.every(r => r.querySelector(".gutter").textContent === `+jsString(glyph.Selected)+`)`, "and back in")

	// c copies (the effect reaches the page); e, then h, then Enter:
	// a download of the export.
	ev(logEl + `.focus()`)
	for _, k := range []string{"c", "e"} {
		devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": k, "text": k})
		devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k})
	}
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Effect" && e.effect === "Copy")`, "copied")
	waitFor(t, ev, `document.querySelector(".logbar").textContent.includes(`+jsString(str.BrowseFormatPrompt())+`) && document.activeElement === document.querySelector(".logbar")`, "export's format prompt, focused")
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "h", "text": "h"})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "h"})
	waitFor(t, ev, `document.querySelector("#kiln-logprompt") && document.querySelector("#kiln-logprompt").value.endsWith(".html")`, "the file name, suggested")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Effect" && e.effect === "SaveFile")`, "the download")

	// Ctrl+L hides it, the input back (Second's password field: it has no
	// saved password); Ctrl+L again shows it as it was.
	ctrlL(t, ws)
	waitFor(t, ev, `!document.querySelector(".log") && document.activeElement.matches(".bottom input, .bottom textarea")`, "hidden, the input back")
	ctrlL(t, ws)
	waitFor(t, ev, `document.querySelector("mark")`, "back as it was")

	// Esc closes it.
	ev(logEl + `.focus()`)
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector(".log") && kilnPage.events.filter(e => e.type === "Log").at(-1).Open === false`, "closed")
}

// TestFilter uses the filter panel the way a person does: f, + Text,
// a text typed (its f and o never reaching the panel), Only on it, x,
// Esc; the Filter chip; Ctrl+L keeping it.
func TestFilter(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")
	ctrlO(t, ws)
	waitFor(t, ev, `document.activeElement === document.querySelector('.bottom input[role="combobox"]')`, "the filter")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Key === "fake/second" && e.Lines.some(l => l.Kind === "server"))`, "Second's lines")
	ctrlL(t, ws)
	logEl := `document.querySelector(".log")`
	waitFor(t, ev, `document.activeElement === `+logEl+` && document.querySelectorAll(".lrow").length === 3`, "log mode, three lines")
	letter := func(k string) {
		devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": k, "text": k})
		devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k})
	}

	// f: the panel, focus on its first entry, its hints below.
	letter("f")
	sel := `document.querySelector(".filter-panel .fsel")`
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterUntagged()), "the panel, Untagged focused")
	waitFor(t, ev, `document.querySelector(".bottom").textContent === `+jsString(str.FilterHints()), "the panel's hints")

	// ↓ to + Text, Enter: the prompt. "fake muck", typed key by key there,
	// has an f and no panel key reaches the panel; Enter adds it, the new
	// row focused.
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterAddText()), "+ Text")
	pressKey(t, ws, "Enter", 13)
	prompt := `document.querySelector("#kiln-logprompt")`
	waitFor(t, ev, `document.activeElement === `+prompt, "the filter prompt")
	for _, r := range "fake muck" {
		letter(string(r))
	}
	waitFor(t, ev, prompt+`.value === "fake muck"`, "the text typed")
	if got := ev(`!!document.querySelector(".filter-panel") && !kilnPage.sent.some(a => a.Do === "only" || a.Do === "hide")`); got != "true" {
		t.Error("typing in the prompt reached the panel")
	}
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterTextItem("fake muck")), "the new text row, focused")

	// A new text is Only, as the TUI's: only the Welcome line. o lets the
	// others back; o again, only the Welcome line.
	waitFor(t, ev, `document.querySelectorAll(".lrow").length === 1`, "only the Welcome line")
	letter("o")
	waitFor(t, ev, `document.querySelectorAll(".lrow").length === 3`, "all three back")
	letter("o")
	waitFor(t, ev, `document.querySelectorAll(".lrow").length === 1`, "only the Welcome line again")

	// x removes the text row, its Only with it, the highlight moving to +
	// Text.
	letter("x")
	waitFor(t, ev, `!kilnPage.events.filter(e => e.type === "Filter").at(-1).Entries.some(e => e.Item.Text) && document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterAddText()), "removed, + Text focused")
	waitFor(t, ev, `document.querySelectorAll(".lrow").length === 3`, "all three back, the Only gone with it")

	// Go re-sending the panel with a row gone above the highlight keeps
	// focus on it: texts aa and bb, bb focused, aa removed from outside.
	act := func(do, term string) {
		ev(`kilnPage.act({Do: ` + jsString(do) + `, Key: "fake/second", Term: ` + jsString(term) + `})`)
	}
	act("add_text", "aa")
	act("add_text", "bb")
	pressKey(t, ws, "ArrowUp", 38)
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterTextItem("bb")), "bb focused")
	act("remove_text", "aa")
	waitFor(t, ev, `!document.querySelector(".filter-panel").textContent.includes(`+jsString(str.FilterTextItem("aa"))+`)`, "aa gone")
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterTextItem("bb")), "bb still focused")

	// Enter on a focused Hide presses it, as a button's Enter does.
	pressKey(t, ws, "Tab", 9)
	waitFor(t, ev, `document.activeElement.textContent === `+jsString(str.FilterHide()), "Hide focused")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.sent.some(a => a.Do === "hide" && a.Item.Name === "bb")`, "Enter pressed Hide")

	// Esc closes the panel, the log focused; the chip opens it again, lit.
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector(".filter-panel") && document.activeElement === `+logEl, "closed, the log focused")
	click(t, ws, ev, `[...document.querySelectorAll("header button")].find(b => b.textContent === `+jsString(str.ViewFilterButton())+`)`)
	waitFor(t, ev, `document.querySelector(".filter-panel")`, "the chip opened it")

	// Ctrl+L hides log mode, panel and all; Ctrl+L again: both back.
	ctrlL(t, ws)
	waitFor(t, ev, `!document.querySelector(".log") && !document.querySelector(".filter-panel")`, "hidden, the characters back")
	ctrlL(t, ws)
	waitFor(t, ev, `document.querySelector(".log") && document.querySelector(".filter-panel")`, "back as it was")

	// Esc in the log closes the panel, not log mode.
	ev(logEl + `.focus()`)
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector(".filter-panel") && document.querySelector(".log")`, "the panel closed, log mode stays")
}

// TestLogPlace: log mode keeps each character's place across a switch
// and Ctrl+L; Esc and reopening starts at the newest line.
func TestLogPlace(t *testing.T) {
	many := make([]string, 80)
	for i := range many {
		many[i] = fmt.Sprintf("line %d", i)
	}
	s := goldenScene(t, nil, func(s *scene) {
		kilntest.WriteLog(t, logstore.Layout{Root: s.logs, World: "fm", Char: "kit", CharName: "Kit"}, kilntest.LogDay, many...)
		kilntest.WriteLog(t, logstore.Layout{Root: s.logs, World: "fm", Char: "rook", CharName: "Rook"}, kilntest.LogDay, many...)
	})
	logEl := `document.querySelector(".log")`
	rows := `document.querySelectorAll(".lrow").length === 80`
	atTop := `(e => e && e.scrollTop < 5)(` + logEl + `)`
	atEnd := `(e => e && e.scrollTop + e.clientHeight >= e.scrollHeight - 5)(` + logEl + `)`
	where := `document.querySelector("header .where").textContent`
	ctrlL(t, s.ws)
	waitFor(t, s.ev, rows+` && `+atEnd, "Kit's log, at the newest")
	s.ev(logEl + `.scrollTop = 0`)
	waitFor(t, s.ev, atTop, "Kit's log, scrolled to the top")

	// Ctrl+L away and back: still at the top.
	ctrlL(t, s.ws)
	waitFor(t, s.ev, `!`+logEl, "hidden")
	ctrlL(t, s.ws)
	waitFor(t, s.ev, rows+` && `+atTop, "back where it was")

	// Rook's log mode, then back to Kit: still at the top.
	s.ev(`kilnPage.act({Do: "open", Key: "fm/rook"})`)
	s.ev(`kilnPage.act({Do: "switch", Key: "fm/rook"})`)
	waitFor(t, s.ev, where+`.endsWith("Rook")`, "Rook")
	ctrlL(t, s.ws)
	waitFor(t, s.ev, rows+` && `+atEnd, "Rook's log, at the newest")
	s.ev(`kilnPage.act({Do: "switch", Key: "fm/kit"})`)
	waitFor(t, s.ev, where+`.endsWith("Kit") && `+atTop, "Kit's place kept")

	// Esc closes log mode; Ctrl+L opens it again at the newest line.
	s.ev(logEl + `.focus()`)
	pressKey(t, s.ws, "Escape", 27)
	waitFor(t, s.ev, `!`+logEl, "closed")
	ctrlL(t, s.ws)
	waitFor(t, s.ev, rows+` && `+atEnd, "reopened at the newest")
}

// TestLogCursor moves log mode's cursor the way a person does: it starts
// on the newest line; ↑, k, ↓ and j move it a line, scrolling it into
// view; scrolling the view drags it along at the edge it would leave
// by; a click puts it on a line.
func TestLogCursor(t *testing.T) {
	many := make([]string, 80)
	for i := range many {
		many[i] = fmt.Sprintf("line %d", i)
	}
	s := goldenScene(t, nil, func(s *scene) {
		kilntest.WriteLog(t, logstore.Layout{Root: s.logs, World: "fm", Char: "kit", CharName: "Kit"}, kilntest.LogDay, many...)
	})
	logEl := `document.querySelector(".log")`
	cursor := `(() => { const r = document.querySelector(".lrow .r-log-cursor")?.closest(".lrow"); return r ? [...document.querySelectorAll(".lrow")].indexOf(r) : -1; })()`
	inView := `(() => { const e = ` + logEl + `, r = document.querySelector(".lrow .r-log-cursor").closest(".lrow"); return r.offsetTop >= e.scrollTop && r.offsetTop + r.offsetHeight <= e.scrollTop + e.clientHeight; })()`
	ctrlL(t, s.ws)
	waitFor(t, s.ev, `document.querySelectorAll(".lrow").length === 80 && `+cursor+` === 79`, "the cursor on the newest line")
	s.ev(logEl + `.focus()`)
	letter := func(k string) {
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": k, "text": k})
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k})
	}
	pressKey(t, s.ws, "ArrowUp", 38)
	waitFor(t, s.ev, cursor+` === 78`, "↑ moved it up")
	letter("k")
	waitFor(t, s.ev, cursor+` === 77`, "k too")
	pressKey(t, s.ws, "ArrowDown", 40)
	letter("j")
	waitFor(t, s.ev, cursor+` === 79`, "↓ and j moved it back down")
	for range 40 {
		letter("k")
	}
	waitFor(t, s.ev, cursor+` === 39 && `+inView, "forty lines up, still in view")

	// Scrolled to the top, it leaves by the bottom, so it rides along on
	// the last line wholly in view; scrolled back to the bottom, on the
	// first.
	whole := `[...document.querySelectorAll(".lrow")].map((r, i) => [r, i]).filter(([r]) => { const e = ` + logEl + `; return r.offsetTop >= e.scrollTop && r.offsetTop + r.offsetHeight <= e.scrollTop + e.clientHeight; }).map(([, i]) => i)`
	s.ev(logEl + `.scrollTop = 0`)
	waitFor(t, s.ev, cursor+` === `+whole+`.at(-1) && `+cursor+` < 39`, "dragged along at the bottom edge")
	s.ev(logEl + `.scrollTop = ` + logEl + `.scrollHeight`)
	waitFor(t, s.ev, cursor+` === `+whole+`[0] && `+cursor+` > 39`, "dragged along at the top edge")

	// A click puts it on the line clicked.
	click(t, s.ws, s.ev, `[...document.querySelectorAll(".lrow")].at(-3)`)
	waitFor(t, s.ev, cursor+` === 77`, "the line clicked")
}
