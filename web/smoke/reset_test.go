package smoke

import (
	"encoding/json"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// TestReset wipes what the browser saved: /reset asks, Enter again
// wipes and reloads, and the preset's world is back as it shipped.
func TestReset(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	world := `new TextDecoder().decode(fs.entry("/home/kiln/.config/kiln/worlds/fake.toml").bytes)`
	in := `document.querySelector(".bottom textarea")`
	// saved is the world's file as IndexedDB has it, "" for none.
	saved := `new Promise((ok) => { const r = indexedDB.open("kiln"); r.onsuccess = () => {
		const g = r.result.transaction("files").objectStore("files").get("/home/kiln/.config/kiln/worlds/fake.toml");
		g.onsuccess = () => { r.result.close(); ok(g.result ? new TextDecoder().decode(g.result.bytes) : ""); };
	}; })`

	// Something saved: a highlight, written into the preset's world, and
	// the notifications offer turned down.
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Active" && e.Key === "fake/tester")`, "opening the character")
	ev(`localStorage.setItem("kiln.notify.asked", "1")`)
	ev(in + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "/highlight lighthouse"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, world+`.includes("lighthouse")`, "the highlight in the world's file")
	ev(`(async function poll() { window.isSaved = (await ` + saved + `).includes("lighthouse"); if (!window.isSaved) setTimeout(poll, 100); })()`)
	waitFor(t, ev, `window.isSaved === true`, "the highlight saved")

	// The first Enter asks, keeping /reset in the input.
	ev(`window.notReloaded = true`)
	ev(in + `.focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "/reset"})
	pressKey(t, ws, "Enter", 13)
	ask, _ := json.Marshal(str.StatusResetConfirm())
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Status" && e.Text === `+string(ask)+`)`, "the reset's warning")
	waitFor(t, ev, in+`.value === "/reset"`, "/reset kept in the input")
	if got := ev(`window.notReloaded === true`); got != "true" {
		t.Fatal("the first Enter reset")
	}

	// The second wipes and reloads; the preset seeds a first visit.
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `window.notReloaded === undefined && kilnPage.events.some(e => e.type === "Order")`, "the page reloaded")
	if got := ev(world + `.includes("lighthouse")`); got != "false" {
		t.Error("the highlight outlived the reset")
	}
	if got := ev(world + `.includes("Tester")`); got != "true" {
		t.Error("the preset's world didn't come back")
	}
	if got := ev(`(async () => (await ` + saved + `).includes("lighthouse"))()`); got != "false" {
		t.Error("the highlight is still saved")
	}
	if got := ev(`localStorage.getItem("kiln.notify.asked")`); got != "null" {
		t.Errorf("the notifications offer is still turned down: %s", got)
	}
}
