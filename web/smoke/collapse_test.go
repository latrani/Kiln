package smoke

import "testing"

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
}
