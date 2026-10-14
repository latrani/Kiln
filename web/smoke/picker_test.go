package smoke

import (
	"encoding/json"
	"testing"
)

// TestPicker opens connections the way a person does: Enter or Ctrl+O, a
// filter, Enter; and adds a character from the picker.
func TestPicker(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Worlds")`, "the worlds")
	input := `document.querySelector(".bottom textarea")`
	filter := `document.querySelector('.bottom input[role="combobox"]')`

	// Enter on an empty input with nothing open shows the picker, focused;
	// Esc puts focus back.
	waitFor(t, ev, `document.activeElement === `+input, "the input focused")
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.activeElement === `+filter, "the filter focused")
	pressKey(t, ws, "Escape", 27)
	waitFor(t, ev, `!`+filter+` && document.activeElement === `+input, "focus back in the input")

	// Ctrl+O, a filter, Enter: Second opens, connects and shows.
	ctrlO(t, ws)
	waitFor(t, ev, `document.activeElement === `+filter, "Ctrl+O's filter focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "sec"})
	waitFor(t, ev, `document.querySelectorAll('nav [role="option"]').length === 2`, "Second and adding a character left")
	if got := ev(`kilnPage.sent.some(a => a.Do === "input")`); got != "false" {
		t.Error("typing a filter went to the chat input")
	}
	// A click on something that can't take focus (a world's header)
	// leaves the keys with the picker, as the TUI's.
	var at struct{ X, Y float64 }
	json.Unmarshal([]byte(mustUnquote(t, ev(`(() => { const r = document.querySelector('nav [role="group"] > span').getBoundingClientRect(); return JSON.stringify({ X: r.x + 2, Y: r.y + r.height / 2 }); })()`))), &at)
	for _, typ := range []string{"mousePressed", "mouseReleased"} {
		devtools(t, ws, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": at.X, "y": at.Y, "button": "left", "clickCount": 1})
	}
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, `document.querySelector('nav [aria-selected="true"]').id === "pick-+fake" && document.activeElement === `+filter, "↓ after a click on the world's name")
	pressKey(t, ws, "ArrowUp", 38)
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Char" && e.Key === "fake/second" && e.State === 2)`, "Second connected")
	waitFor(t, ev, `document.querySelector("header").textContent.includes("Second") && document.activeElement === document.querySelector('.bottom input[type="password"]')`, "Second showing, its password asked")

	// Behind the picker the scrollback dims: a line wears the scroller's
	// color, scrollback.inactive's.
	ev(`document.querySelector("nav button.chip").click()`) // + Connection
	waitFor(t, ev, `document.querySelector(".scroll.dim .line")`, "the scrollback dimmed")
	if got := ev(`(() => { const s = getComputedStyle(document.querySelector(".scroll")); return s.opacity < 1 && [...document.querySelectorAll(".scroll .line, .scroll .line *")].every(e => { const c = getComputedStyle(e); return c.color === s.color && c.opacity === "1"; }); })()`); got != "true" {
		t.Error("a dimmed line isn't in scrollback.inactive's look")
	}

	// + Character: a name with a space is refused as you type; a good one
	// is saved, opened and connected, and the picker closes.
	ev(`document.querySelector('nav [role="option"][id="pick-+fake"]').click()`)
	name := `document.querySelector("#kiln-name")`
	waitFor(t, ev, `document.activeElement === `+name, "the name field focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "New Comer"})
	waitFor(t, ev, name+`.getAttribute("aria-invalid") === "true"`, "a space refused")
	ctrlO(t, ws) // as the TUI's name form: nothing
	if got := ev(name + `.value`); got != `"New Comer"` {
		t.Errorf("after Ctrl+O the name field holds %s, want what was typed", got)
	}
	ev(name + `.select()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "Newcomer"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Char" && e.Name === "Newcomer" && e.State === 2)`, "Newcomer added and connected")
	waitFor(t, ev, `!document.querySelector(".scroll.dim") && document.querySelector("header").textContent.includes("Newcomer")`, "the picker closed, Newcomer showing")

}
