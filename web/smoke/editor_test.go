package smoke

import (
	"strings"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// TestEditor edits the way a person does: a world from the picker, a
// check and a refusal fixed, a save; a choice by arrow keys; a new world;
// a character's editor from Alt+T, hidden and back with its edit; and a
// delete pressed twice.
func TestEditor(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")
	field := func(k string) string { return `document.querySelector("#ed-` + k + `")` }
	selected := `document.querySelector('nav [aria-selected="true"]').id`
	filter := `document.querySelector('.bottom input[role="combobox"]')`

	// The picker, ↑ from the first character to the world's row, Enter:
	// its editor, focus in Host, the scrollback dimmed.
	ctrlO(t, ws)
	waitFor(t, ev, selected+` === "pick-fake/historian" && document.activeElement === `+filter, "the picker, on the first character, the filter focused")
	pressKey(t, ws, "ArrowUp", 38)
	waitFor(t, ev, selected+` === "pick-=fake"`, "the world's row")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+field("host"), "the world's editor, Host focused")
	if got := ev(`document.querySelector(".scroll.dim") !== null`); got != "true" {
		t.Error("the scrollback isn't dimmed behind the editor")
	}

	// A bad port shows its check by the field; Ctrl+O is eaten; Enter is
	// refused by Go, focus going to Port; nothing typed reached the input.
	port := mustUnquote(t, ev(field("port")+`.value`))
	ev(field("port") + `.select()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "99x"})
	waitFor(t, ev, field("port")+`.getAttribute("aria-invalid") === "true"`, "the port's check")
	ctrlO(t, ws)
	if got := ev(field("port") + ` !== null`); got != "true" {
		t.Error("Ctrl+O closed the editor")
	}
	ev(field("host") + `.focus()`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Problems" && e.Op === "save_world") && document.activeElement === `+field("port"), "refused, focus on Port")
	if got := ev(`kilnPage.sent.some(a => a.Do === "input")`); got != "false" {
		t.Error("typing in the editor went to the chat input")
	}
	// Refused again with focus already on Port: it's focused anew, so a
	// screen reader reads the field and its error again.
	ev(`window.portFocus = 0; ` + field("port") + `.addEventListener("focus", () => window.portFocus++)`)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Problems" && e.Op === "save_world").length === 2 && window.portFocus > 0 && document.activeElement === `+field("port"), "refused again, Port focused anew")

	// A choice: ↓ on the chosen radio picks the next (the browser's radio
	// keys). Then the port put back, Enter: saved, back at the filter.
	ev(`document.querySelector("details").open = true`)
	waitFor(t, ev, `document.querySelector("details[open]")`, "the extras shown")
	// With the mouse: a click on the shown value takes the next, and a
	// click on the drawn box toggles it (twice, so TLS stays on).
	click(t, ws, ev, `document.querySelector('[name="ed-reconnect"]:checked + span')`)
	waitFor(t, ev, `document.querySelector('[name="ed-reconnect"]:checked').value === "on"`, "a click chose on")
	click(t, ws, ev, `document.querySelector("#ed-tls + [aria-hidden]")`)
	waitFor(t, ev, `!`+field("tls")+`.checked`, "a click on the box turned TLS off")
	click(t, ws, ev, `document.querySelector("#ed-tls + [aria-hidden]")`)
	waitFor(t, ev, field("tls")+`.checked`, "and on again")
	ev(`document.querySelector('[name="ed-reconnect"]:checked').focus()`)
	// A focused choice's ◄ ► are drawn, not read: their alt text is empty.
	if got := ev(`getComputedStyle(document.querySelector('[name="ed-reconnect"]:checked + span'), "::before").content`); !strings.Contains(got, `/ \"\"`) {
		t.Errorf("the focused choice's ◄ has no empty alt text: %s", got)
	}
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, `document.querySelector('[name="ed-reconnect"]:checked').value === "off"`, "↓ chose off")
	ev(field("port") + `.select()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": port})
	// A click on the scrollback leaves the keys with the editor: Enter
	// still saves.
	click(t, ws, ev, `document.querySelector(".scroll")`)
	waitFor(t, ev, `document.activeElement === document.body`, "focus on nothing")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Saved" && e.Op === "save_world") && !`+field("port")+` && document.activeElement === `+filter, "saved, back at the filter")
	waitFor(t, ev, `document.querySelector('[role="status"]').textContent.includes(`+jsString(str.EditorSaved("fake"))+`)`, "the status says saved")

	// + World, the last row: a new world saved lands on its + Character.
	for range 5 {
		pressKey(t, ws, "PageDown", 34)
	}
	waitFor(t, ev, selected+` === "pick-+"`, "+ World")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+field("id"), "a new world's editor, World focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "zz"})
	ev(field("host") + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "zz.test"})
	ev(field("port") + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "7777"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, selected+` === "pick-+zz"`, "the new world's add row highlighted")

	// Second, picked; then from the main view Alt+T, an edit, Alt+T hides
	// it (the picker with it), Alt+T brings it back, unsaved.
	ev(filter + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
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
