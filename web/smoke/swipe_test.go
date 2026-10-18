package smoke

import (
	"encoding/json"
	"testing"

	"github.com/coder/websocket"
)

// TestSwipe flicks the narrow page with one finger: right opens the
// sidebar, left closes it, and a flick that starts in the input is the
// input's.
func TestSwipe(t *testing.T) {
	s := goldenScene(t, nil, nil)
	devtools(t, s.ws, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 400, "height": 800, "deviceScaleFactor": 1, "mobile": true})
	devtools(t, s.ws, "Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 1})
	waitFor(t, s.ev, `document.querySelector(".kiln.narrow") && !document.querySelector("nav.side")`, "narrow, closed")

	flick(t, s.ws, 100, 300, 260, 305)
	waitFor(t, s.ev, `document.querySelector(".kiln.narrow.open nav.side")`, "a flick right opened it")
	flick(t, s.ws, 300, 300, 120, 290)
	waitFor(t, s.ev, `!document.querySelector("nav.side")`, "a flick left closed it")

	var in struct{ X, Y float64 }
	json.Unmarshal([]byte(mustUnquote(t, s.ev(`(() => { const r = document.querySelector(".bottom textarea").getBoundingClientRect(); return JSON.stringify({ X: r.x + 10, Y: r.y + r.height / 2 }); })()`))), &in)
	flick(t, s.ws, in.X, in.Y, in.X+200, in.Y)
	if got := s.ev(`document.querySelector("nav.side") === null`); got != "true" {
		t.Error("a flick that started in the input opened the sidebar")
	}
}

// flick is one finger from (x1, y1) to (x2, y2), quickly.
func flick(t *testing.T, ws *websocket.Conn, x1, y1, x2, y2 float64) {
	t.Helper()
	point := func(x, y float64) []map[string]any { return []map[string]any{{"x": x, "y": y}} }
	devtools(t, ws, "Input.dispatchTouchEvent", map[string]any{"type": "touchStart", "touchPoints": point(x1, y1)})
	devtools(t, ws, "Input.dispatchTouchEvent", map[string]any{"type": "touchMove", "touchPoints": point((x1+x2)/2, (y1+y2)/2)})
	devtools(t, ws, "Input.dispatchTouchEvent", map[string]any{"type": "touchMove", "touchPoints": point(x2, y2)})
	devtools(t, ws, "Input.dispatchTouchEvent", map[string]any{"type": "touchEnd", "touchPoints": []map[string]any{}})
}
