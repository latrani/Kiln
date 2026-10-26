package smoke

import (
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// TestEditor edits the way a person does: a world from the picker, its
// host, port and TLS the relay's and fixed; a check and a refusal fixed,
// a save; a choice by arrow keys; no + World; a character's editor from
// Alt+T, hidden and back with its edit; and a delete pressed twice.
func TestEditor(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")
	field := func(k string) string { return `document.querySelector("#ed-` + k + `")` }
	selected := `document.querySelector('nav [aria-selected="true"]').id`
	filter := `document.querySelector('nav.side input[role="combobox"]')`

	// The picker, ↑ from the first character to the world's row, Enter:
	// its editor, focus past the fixed fields on Save, the scrollback
	// dimmed. No + World: the worlds are the relay's.
	ctrlO(t, ws)
	waitFor(t, ev, selected+` === "pick-fake/historian" && document.activeElement === `+filter, "the picker, on the first character, the filter focused")
	if got := ev(`document.getElementById("pick-+") === null`); got != "true" {
		t.Error("the picker offers + World")
	}
	pressKey(t, ws, "ArrowUp", 38)
	waitFor(t, ev, selected+` === "pick-=fake"`, "the world's row")
	pressKey(t, ws, "Enter", 13)
	save := `document.querySelector('.editor button[type="submit"]')`
	waitFor(t, ev, `document.activeElement === `+save, "the world's editor, Save focused")
	if got := ev(`document.querySelector(".scroll.dim") !== null`); got != "true" {
		t.Error("the scrollback isn't dimmed behind the editor")
	}
	if got := ev(field("host") + `.readOnly && ` + field("port") + `.readOnly && ` + field("tls") + `.disabled`); got != "true" {
		t.Error("the relay's host, port and TLS can be edited")
	}
	click(t, ws, ev, `document.querySelector("#ed-tls + [aria-hidden]")`)
	if got := ev(field("tls") + `.checked`); got != "true" {
		t.Error("a click on the fixed TLS box changed it")
	}

	// More settings folds as the filter panel's tags do: a click off its
	// glyph does nothing; ←/→ open and shut it; Enter toggles it; a click
	// on the glyph toggles it.
	summary := `document.querySelector("details summary")`
	shown := `document.querySelector("details").open`
	click(t, ws, ev, summary)
	if got := ev(shown); got != "false" {
		t.Error("a click on More settings' label opened it")
	}
	ev(summary + `.focus()`)
	pressKey(t, ws, "ArrowRight", 39)
	waitFor(t, ev, shown, "→ opened the extras")
	pressKey(t, ws, "ArrowLeft", 37)
	waitFor(t, ev, `!`+shown, "← shut them")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, shown, "Enter opened them")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `!`+shown, "Enter shut them")
	click(t, ws, ev, `document.querySelector("summary .dglyph")`)
	waitFor(t, ev, shown, "a click on the glyph opened them")

	// A bad number shows its check by the field; Ctrl+O is eaten; Enter
	// is refused by Go, focus going to the field; nothing typed reached
	// the input.
	mb := field("max_line_bytes")
	ev(mb + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "99x"})
	waitFor(t, ev, mb+`.getAttribute("aria-invalid") === "true"`, "the number's check")
	ctrlO(t, ws)
	if got := ev(mb + ` !== null`); got != "true" {
		t.Error("Ctrl+O closed the editor")
	}
	ev(field("login") + `.focus()`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Problems" && e.Op === "save_world") && document.activeElement === `+mb, "refused, focus on the number")
	if got := ev(`kilnPage.sent.some(a => a.Do === "input")`); got != "false" {
		t.Error("typing in the editor went to the chat input")
	}
	// Refused again with focus already there: it's focused anew, so a
	// screen reader reads the field and its error again.
	ev(`window.mbFocus = 0; ` + mb + `.addEventListener("focus", () => window.mbFocus++)`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Problems" && e.Op === "save_world").length === 2 && window.mbFocus > 0 && document.activeElement === `+mb, "refused again, focused anew")

	// A choice: with the mouse, a click on the shown value takes the
	// next; ↓ on the chosen radio picks the next (the browser's radio
	// keys). Then the number cleared, Enter: saved, back at the filter.
	click(t, ws, ev, `document.querySelector('[name="ed-reconnect"]:checked + span')`)
	waitFor(t, ev, `document.querySelector('[name="ed-reconnect"]:checked').value === "on"`, "a click chose on")
	ev(`document.querySelector('[name="ed-reconnect"]:checked').focus()`)
	// A focused choice's ◄ ► are drawn, not read: their alt text is empty.
	if got := ev(`getComputedStyle(document.querySelector('[name="ed-reconnect"]:checked + span'), "::before").content`); !strings.Contains(got, `/ \"\"`) {
		t.Errorf("the focused choice's ◄ has no empty alt text: %s", got)
	}
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, `document.querySelector('[name="ed-reconnect"]:checked').value === "off"`, "↓ chose off")
	ev(mb + `.value = ""; ` + mb + `.dispatchEvent(new Event("input", {bubbles: true}))`)
	// A click on the scrollback leaves the keys with the editor: Enter
	// still saves.
	click(t, ws, ev, `document.querySelector(".scroll")`)
	waitFor(t, ev, `document.activeElement === document.body`, "focus on nothing")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Saved" && e.Op === "save_world") && !`+mb+` && document.activeElement === `+filter, "saved, back at the filter")
	waitFor(t, ev, `document.querySelector('[role="status"]').textContent.includes(`+jsString(str.EditorSaved("fake"))+`)`, "the status says saved")

	// Second, picked; then from the main view Alt+T, an edit, Alt+T hides
	// it (the picker with it), Alt+T brings it back, unsaved.
	ev(filter + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
	waitFor(t, ev, selected+` === "pick-=fake"`, "still on the world's row, which the filter offers")
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, selected+` === "pick-fake/second"`, "Second highlighted")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Second")`, "Second showing")
	altT := func() {
		for _, typ := range []string{"rawKeyDown", "keyUp"} {
			devtools(t, ws, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "†", "code": "KeyT", "windowsVirtualKeyCode": 84, "modifiers": 1})
		}
	}
	altT()
	waitFor(t, ev, `document.activeElement === `+field("aliases"), "Second's editor")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "twoey"})
	altT()
	waitFor(t, ev, `!`+field("aliases")+` && !document.querySelector(".scroll.dim")`, "hidden, the picker closed with it")
	altT()
	unsaved := jsString(str.EditorEditing("fake/Second") + str.Separator() + str.EditorUnsaved())
	waitFor(t, ev, field("aliases")+`.value === "twoey" && document.querySelector("#ed-title").textContent === `+unsaved+` && document.activeElement === `+field("aliases"), "back, unsaved, Aliases focused")

	// A double-click on Delete is one press, not two; leaving it asks
	// again from the start.
	del := `[...document.querySelectorAll(".editor button")].at(-1)`
	note := `document.querySelector(".editor [aria-live]").textContent`
	click(t, ws, ev, del)
	waitFor(t, ev, note+` === `+jsString(str.WebDeleteAgain("fake/second")), "the first click asks")
	click2(t, ws, ev, del)
	if got := ev(`kilnPage.sent.some(a => a.Do === "delete_character")`); got != "false" {
		t.Fatal("a double-click on Delete deleted")
	}
	ev(field("aliases") + `.focus()`)
	waitFor(t, ev, note+` === `+jsString(str.WebFormHint(str.EditorHint())), "moving off Delete stops asking")

	// Delete, pressed twice: Second's gone, and so is the editor.
	ev(`[...document.querySelectorAll(".editor button")].at(-1).focus()`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.querySelector(".editor [aria-live]").textContent === `+jsString(str.WebDeleteAgain("fake/second")), "asked again")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Saved" && e.Op === "delete_character") && !document.querySelector(".editor")`, "deleted, the editor gone")
}
