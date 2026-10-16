package smoke

import (
	"encoding/json"
	"testing"
	"time"
)

// TestCollapse: Ctrl+B collapses the sidebar, the top bar starting with
// ≡; a click on ≡ brings it back.
func TestCollapse(t *testing.T) {
	s := goldenScene(t, nil, nil)
	for _, typ := range []string{"rawKeyDown", "keyUp"} {
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "b", "code": "KeyB", "windowsVirtualKeyCode": 66, "modifiers": 2})
	}
	waitFor(t, s.ev, `!document.querySelector("nav.side") && document.querySelector("header .menu")`, "collapsed, ≡ in the top bar")
	click(t, s.ws, s.ev, `document.querySelector("header .menu")`)
	waitFor(t, s.ev, `document.querySelector("nav.side") && !document.querySelector("header .menu")`, "back")
	collapse := `document.querySelector(".stitle button")`
	waitFor(t, s.ev, `document.activeElement === `+collapse, "focus on ‹, where ≡ was")
	pressKey(t, s.ws, "Enter", 13) // ‹, from the keyboard
	waitFor(t, s.ev, `!document.querySelector("nav.side") && document.activeElement === document.querySelector("header .menu")`, "collapsed, focus on ≡")
}

// TestNarrow: at a phone's width there's no sidebar column; ≡ opens the
// sidebar over the scrollback, focus going into it; Esc closes it, focus
// back on the input; a click on a row picks it and closes it.
func TestNarrow(t *testing.T) {
	s := goldenScene(t, nil, nil)
	devtools(t, s.ws, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 400, "height": 800, "deviceScaleFactor": 1, "mobile": false})
	waitFor(t, s.ev, `document.querySelector(".kiln.narrow") && !document.querySelector("nav.side") && document.querySelector("header .menu")`, "narrow: no column, ≡")
	click(t, s.ws, s.ev, `document.querySelector("header .menu")`)
	waitFor(t, s.ev, `document.querySelector(".kiln.narrow.open nav.side") && document.activeElement === document.querySelector(".stitle button")`, "open, focus on ‹")
	pressKey(t, s.ws, "Escape", 27)
	waitFor(t, s.ev, `!document.querySelector("nav.side") && document.activeElement === document.querySelector(".bottom textarea")`, "closed, focus on the input")
	click(t, s.ws, s.ev, `document.querySelector("header .menu")`)
	waitFor(t, s.ev, `document.querySelector("nav.side")`, "open again")
	click(t, s.ws, s.ev, `document.querySelector("nav.side button.name")`) // the world's row
	waitFor(t, s.ev, `!document.querySelector("nav.side")`, "picked: closed")

	// A double-click on a row: the first picks and closes it; the second
	// lands on what was under it, the overview's first character, and
	// does nothing there.
	time.Sleep(450 * time.Millisecond) // past the last pick's double-click window
	click(t, s.ws, s.ev, `document.querySelector("header .menu")`)
	waitFor(t, s.ev, `document.querySelector("nav.side")`, "open for a double-click")
	var at struct{ X, Y float64 }
	json.Unmarshal([]byte(mustUnquote(t, s.ev(`(() => { const r = document.querySelector("nav.side button.name").getBoundingClientRect(); return JSON.stringify({ X: r.x + r.width / 2, Y: r.y + r.height / 2 }); })()`))), &at)
	for n := 1; n <= 2; n++ {
		for _, typ := range []string{"mousePressed", "mouseReleased"} {
			devtools(t, s.ws, "Input.dispatchMouseEvent", map[string]any{"type": typ, "x": at.X, "y": at.Y, "button": "left", "clickCount": n})
		}
	}
	time.Sleep(300 * time.Millisecond)
	waitFor(t, s.ev, `document.querySelector("header .where").textContent === "fm"`, "still the overview: the second click did nothing")
	time.Sleep(450 * time.Millisecond)

	// Picking a character: closed, focus on the input.
	s.ev(`kilnPage.act({Do: "switch", Key: "fm/kit"})`)
	click(t, s.ws, s.ev, `document.querySelector("header .menu")`)
	waitFor(t, s.ev, `document.querySelectorAll("nav.side button.name").length > 1`, "open, with a character row")
	click(t, s.ws, s.ev, `document.querySelectorAll("nav.side button.name")[1]`)
	waitFor(t, s.ev, `!document.querySelector("nav.side") && document.activeElement === document.querySelector(".bottom textarea")`, "picked a character: closed, focus on the input")
	time.Sleep(450 * time.Millisecond)

	// Log mode: f opens the panel over the log, Esc closes it, the log
	// focused, its keys working.
	devtools(t, s.ws, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 960, "height": 600, "deviceScaleFactor": 1, "mobile": false})
	waitFor(t, s.ev, `!document.querySelector(".kiln.narrow")`, "wide")
	s.ev(`kilnPage.act({Do: "switch", Key: "fm/kit"})`)
	devtools(t, s.ws, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 400, "height": 800, "deviceScaleFactor": 1, "mobile": false})
	waitFor(t, s.ev, `document.querySelector(".kiln.narrow") && document.querySelector("header .where").textContent.endsWith("Kit")`, "narrow, on Kit")
	ctrlL(t, s.ws)
	waitFor(t, s.ev, `document.activeElement === document.querySelector(".log")`, "log mode")
	for _, typ := range []string{"keyDown", "keyUp"} {
		devtools(t, s.ws, "Input.dispatchKeyEvent", map[string]any{"type": typ, "key": "f", "text": "f"})
	}
	waitFor(t, s.ev, `document.querySelector(".kiln.narrow.open .filter-panel")`, "f: the panel, over the log")
	pressKey(t, s.ws, "Escape", 27)
	waitFor(t, s.ev, `!document.querySelector(".filter-panel") && document.activeElement === document.querySelector(".log")`, "closed, the log focused")
}
