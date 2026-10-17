package smoke

import (
	"fmt"
	"strings"
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
	waitFor(t, ev, `document.querySelector(".bottom").textContent === `+jsString(str.BrowseHintsNone()), "the hints")

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
	waitFor(t, ev, `document.querySelector(".status").textContent.includes(`+jsString(str.BrowseFindStatus("fake muck", 1, 1))+`) && !document.querySelector("header").textContent.includes(`+jsString(str.BrowseFindStatus("fake muck", 1, 1))+`)`, "the find status in the status line: one match, today's the only day")

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

	// A click moves the cursor (where find goes from next: find searches
	// older from where you are, as the TUI's) and selects nothing. s, or
	// the Select chip, starts selecting from it; while selecting a click
	// outside the range grows it, one inside takes a line out (and again
	// puts it back), and s locks it. No Shift anywhere.
	rows := `[...document.querySelectorAll(".lrow")]`
	selected := func(sel string) string { return sel + `.querySelector(".gutter").textContent === ` + jsString(glyph.Selected) }
	click(t, ws, ev, rows+`[0]`)
	waitFor(t, ev, rows+`[0].querySelector(".r-log-cursor") && !(`+selected(rows+`[0]`)+`)`, "the cursor moved, nothing selected")
	click(t, ws, ev, `document.querySelector(".status .chip")`)
	waitFor(t, ev, selected(rows+`[0]`)+` && document.querySelector(".status .chip").getAttribute("aria-pressed") === "true"`, "selecting, the chip lit")
	click(t, ws, ev, rows+`.at(-1)`)
	waitFor(t, ev, rows+`.every(r => `+selected("r")+`)`, "the range grown to the last line")
	click(t, ws, ev, rows+`[1]`)
	waitFor(t, ev, rows+`[1].querySelector(".gutter").textContent === `+jsString(glyph.Excluded), "the middle line taken out")
	click(t, ws, ev, rows+`[1]`)
	waitFor(t, ev, rows+`.every(r => `+selected("r")+`)`, "and back in")
	ev(logEl + `.focus()`)
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "s", "text": "s"})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "s"})
	waitFor(t, ev, `document.querySelector(".status .chip").getAttribute("aria-pressed") === "false" && `+rows+`.every(r => `+selected("r")+`)`, "s locks it")

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
	waitFor(t, ev, `document.activeElement === document.querySelector("#kiln-logprompt") && document.activeElement.value.endsWith(".html")`, "the file name, suggested and focused")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Effect" && e.effect === "SaveFile")`, "the download")

	// Ctrl+L hides it, the input back (Second's password field: it has no
	// saved password); Ctrl+L again shows it as it was.
	ctrlL(t, ws)
	waitFor(t, ev, `!document.querySelector(".log") && document.activeElement.matches(".bottom input, .bottom textarea")`, "hidden, the input back")
	ctrlL(t, ws)
	waitFor(t, ev, `document.querySelector("mark")`, "back as it was")

	// Esc turns find off, log mode staying; / starts with its term again.
	ev(logEl + `.focus()`)
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector("mark") && document.querySelector(".log") && !document.querySelector(".status").textContent.includes(`+jsString(str.BrowseFindStatus("fake muck", 1, 1))+`)`, "find off, log mode open")
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "/", "text": "/", "code": "Slash", "windowsVirtualKeyCode": 191})
	devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "/", "code": "Slash", "windowsVirtualKeyCode": 191})
	waitFor(t, ev, `document.activeElement === document.querySelector("#kiln-logprompt") && document.activeElement.value === "fake muck"`, "/ starts with the last term")
	pressKey(t, ws, "Escape", 27)

	// Esc again closes log mode.
	waitFor(t, ev, `document.activeElement === `+logEl, "the log focused")
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
	waitFor(t, ev, `document.activeElement === `+sel+` && `+sel+`.textContent === `+jsString(str.FilterCategories()), "the panel, Categories focused")
	waitFor(t, ev, `document.querySelector(".bottom").textContent === `+jsString(str.BrowseHintsNoneAway())+` && document.querySelector(".ffoot").textContent === `+jsString(strings.Join([]string{str.FilterKeyHide(), str.FilterKeyOnly(), str.FilterKeyFold(), str.FilterKeyLog()}, str.Separator())), "the footers, the panel focused on Categories")

	// ↓ past kiln and sent to + Text, Enter: the prompt. "fake muck",
	// typed key by key there, has an f and no panel key reaches the panel;
	// Enter adds it, the new row focused.
	pressKey(t, ws, "ArrowDown", 40)
	pressKey(t, ws, "ArrowDown", 40)
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

	// Esc gives the log focus, the panel staying; its keys still reach
	// the panel (h), and f gives the panel focus again.
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `document.querySelector(".filter-panel") && document.activeElement === `+logEl, "the log focused, the panel there")
	waitFor(t, ev, `document.querySelector(".ffoot").textContent === `+jsString(strings.Join([]string{str.FilterKeyHide(), str.FilterKeyOnly(), str.FilterKeyRemove()}, str.Separator())), "the panel's footer for a text, the log focused")
	n := ev(`String(kilnPage.sent.filter(a => a.Do === "hide").length)`)
	letter("h")
	waitFor(t, ev, `String(kilnPage.sent.filter(a => a.Do === "hide").length) !== `+jsString(n), "h from the log hid the highlight")
	letter("f")
	waitFor(t, ev, `document.activeElement === `+sel, "f: the panel focused")

	// Ctrl+L hides log mode, panel and all; Ctrl+L again: both back.
	ctrlL(t, ws)
	waitFor(t, ev, `!document.querySelector(".log") && !document.querySelector(".filter-panel")`, "hidden, the characters back")
	ctrlL(t, ws)
	waitFor(t, ev, `document.querySelector(".log") && document.querySelector(".filter-panel")`, "back as it was")
	waitFor(t, ev, `document.activeElement === `+sel, "back with the panel focused, as left")

	// A prompt opened from the panel gives it focus back when it closes.
	letter("/")
	waitFor(t, ev, `document.activeElement === `+prompt, "find's prompt, from the panel")
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `document.activeElement === `+sel, "the panel focused again")

	// x from the log removes the highlighted text, the log keeping focus.
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `document.activeElement === `+logEl, "the log focused")
	letter("x")
	waitFor(t, ev, `!document.querySelector(".filter-panel").textContent.includes(`+jsString(str.FilterTextItem("bb"))+`)`, "bb removed from the log")
	waitFor(t, ev, `document.activeElement === `+logEl, "the log still focused")
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
// by; a click puts it on a line; go-to starts from it.
func TestLogCursor(t *testing.T) {
	many := make([]string, 80)
	for i := range many {
		many[i] = fmt.Sprintf("line %d", i)
	}
	many[78] = "Kit waves." // tagged self
	s := goldenScene(t, nil, func(s *scene) {
		kilntest.WriteLog(t, logstore.Layout{Root: s.logs, World: "fm", Char: "kit", CharName: "Kit"}, kilntest.LogDay, many...)
	})
	logEl := `document.querySelector(".log")`
	tagsLabel, _, _ := strings.Cut(str.BrowseLineTags("\x00"), "\x00")
	status := `document.querySelector(".status").textContent`
	cursor := `(() => { const r = document.querySelector(".lrow .r-log-cursor")?.closest(".lrow"); return r ? [...document.querySelectorAll(".lrow")].indexOf(r) : -1; })()`
	inView := `(() => { const e = ` + logEl + `, r = document.querySelector(".lrow .r-log-cursor").closest(".lrow"); return r.offsetTop >= e.scrollTop && r.offsetTop + r.offsetHeight <= e.scrollTop + e.clientHeight; })()`
	ctrlL(t, s.ws)
	waitFor(t, s.ev, `document.querySelectorAll(".lrow").length === 80 && `+cursor+` === 79`, "the cursor on the newest line")
	s.ev(logEl + `.focus()`)
	letter := func(k string) {
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": k, "text": k})
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k})
	}
	if got := mustUnquote(t, s.ev(status)); strings.Contains(got, tagsLabel) {
		t.Errorf("an untagged line's tags in the status line: %q", got)
	}
	pressKey(t, s.ws, "ArrowUp", 38)
	waitFor(t, s.ev, cursor+` === 78`, "↑ moved it up")
	waitFor(t, s.ev, status+`.includes(`+jsString(tagsLabel)+`) && `+status+`.includes("self")`, "its tags in the status line")
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

	// g: go-to, starting from the cursor's day and time, the caret at the
	// end; a time edited in goes to its line.
	clock := func(i int) string {
		return mustUnquote(t, s.ev(fmt.Sprintf(`document.querySelectorAll(".lrow")[%d].querySelector("span").textContent`, i)))
	}
	day := kilntest.LogDay.Format("2006-01-02")
	ev := s.ev
	letter("g")
	prompt := `document.querySelector("#kiln-logprompt")`
	waitFor(t, ev, `document.activeElement === `+prompt+` && `+prompt+`.value === `+jsString(day+" "+clock(77))+` && `+prompt+`.selectionStart === `+prompt+`.value.length`, "go-to at the cursor's day and time, the caret at the end")
	ev(prompt + `.select()`)
	devtools(t, s.ws, "Input.insertText", map[string]any{"text": day + " " + clock(10)})
	pressKey(t, s.ws, "Enter", 13)
	waitFor(t, ev, cursor+` === 10`, "the line at that time")
}
