package smoke

import (
	"testing"

	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/str"
)

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
	ctrlL := func() {
		for _, typ := range []string{"rawKeyDown", "keyUp"} {
			devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "l", "code": "KeyL", "windowsVirtualKeyCode": 76, "modifiers": 2})
		}
	}
	ctrlL()
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
	ctrlL()
	waitFor(t, ev, `!document.querySelector(".log") && document.activeElement.matches(".bottom input, .bottom textarea")`, "hidden, the input back")
	ctrlL()
	waitFor(t, ev, `document.querySelector("mark")`, "back as it was")

	// Esc closes it.
	ev(logEl + `.focus()`)
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!document.querySelector(".log") && kilnPage.events.filter(e => e.type === "Log").at(-1).Open === false`, "closed")
}
