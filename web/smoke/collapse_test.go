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
}
