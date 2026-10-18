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
	waitFor(t, ev, `document.querySelectorAll('nav [role="option"]').length === 3`, "the world, Second and its add row left (no + World on the web)")
	// A character and its add row sit one space in from the world, as the
	// TUI's: their text starts right of the row's edge.
	inset := func(id string) string {
		return ev(`(() => {
			const li = document.getElementById(` + jsString(id) + `), r = document.createRange();
			const name = [...li.childNodes].at(-1); // the name, or the add row's chip
			if (name.nodeType === Node.TEXT_NODE) r.selectNodeContents(name);
			else r.selectNode(name);
			return String(Math.round(r.getBoundingClientRect().left - li.getBoundingClientRect().left));
		})()`)
	}
	for _, id := range []string{"pick-fake/second", "pick-+fake"} {
		if got := mustUnquote(t, inset(id)); got == "0" {
			t.Errorf("%s isn't indented under its world", id)
		}
	}
	// A click on something that can't take focus (the scrollback)
	// leaves the keys with the picker, as the TUI's.
	var at struct{ X, Y float64 }
	json.Unmarshal([]byte(mustUnquote(t, ev(`(() => { const r = document.querySelector(".scroll").getBoundingClientRect(); return JSON.stringify({ X: r.x + 2, Y: r.y + r.height / 2 }); })()`))), &at)
	for _, typ := range []string{"mousePressed", "mouseReleased"} {
		devtools(t, ws, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": at.X, "y": at.Y, "button": "left", "clickCount": 1})
	}
	pressKey(t, ws, "ArrowDown", 40)
	waitFor(t, ev, `document.querySelector('nav [aria-selected="true"]').id === "pick-+fake" && document.activeElement === `+filter, "↓ after a click on the scrollback")
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

	// On the world's overview, the rule between Second and Newcomer dims
	// with the lines.
	ev(`kilnPage.act({Do: "switch", Key: "=fake"})`)
	waitFor(t, ev, `document.querySelector(".scroll .ov-rule")`, "the overview's rule")
	ctrlO(t, ws)
	waitFor(t, ev, `document.querySelector(".scroll.dim .ov-rule")`, "the overview dimmed")
	if got := ev(`(() => { const s = getComputedStyle(document.querySelector(".scroll")); return getComputedStyle(document.querySelector(".scroll .ov-rule")).borderTopColor === s.color; })()`); got != "true" {
		t.Error("a dimmed overview's rule isn't in scrollback.inactive's look")
	}
}
