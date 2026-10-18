package smoke

import (
	"bytes"
	"os"
	"testing"

	"github.com/latrani/Kiln/internal/str"
)

// TestSavedWorldFollowsTheAllowlist: a world the player has saved (a
// highlight written into it) takes where to connect from relay.toml on
// the next load, keeping the player's edits; the page can't change it.
func TestSavedWorldFollowsTheAllowlist(t *testing.T) {
	ws, _, _, cfg := launchAll(t, nil)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	path := "/home/kiln/.config/kiln/worlds/fake.toml"
	world := `new TextDecoder().decode(fs.entry("` + path + `").bytes)`
	saved := `new Promise((ok) => { const r = indexedDB.open("kiln"); r.onsuccess = () => {
		const g = r.result.transaction("files").objectStore("files").get("` + path + `");
		g.onsuccess = () => { r.result.close(); ok(g.result ? new TextDecoder().decode(g.result.bytes) : ""); };
	}; })`
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Active" && e.Key === "fake/tester")`, "opening the character")
	ev(`localStorage.setItem("kiln.notify.asked", "1")`)
	ev(`document.querySelector(".bottom textarea").focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "/highlight lighthouse"})
	pressKey(t, ws, "Enter", 13)
	ev(`(async function poll() { window.isSaved = (await ` + saved + `).includes("lighthouse"); if (!window.isSaved) setTimeout(poll, 100); })()`)
	waitFor(t, ev, `window.isSaved === true`, "the world saved")

	// The relay's world moves (the same fake MUCK, by another name).
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, bytes.Replace(b, []byte(`host = "127.0.0.1"`), []byte(`host = "localhost"`), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	ev(`(async function poll() { window.moved = (await (await fetch("preset/config/worlds/fake.toml")).text()).includes("localhost"); if (!window.moved) setTimeout(poll, 100); })()`)
	waitFor(t, ev, `window.moved === true`, "the relay reloaded")

	devtools(t, ws, "Page.reload", map[string]any{})
	waitFor(t, ev, `typeof kilnPage !== "undefined" && kilnPage.events.some(e => e.type === "Order")`, "the page reloaded")
	waitFor(t, ev, world+`.includes('host = "localhost"') && `+world+`.includes("lighthouse")`, "the saved world on the relay's host, its highlight kept")
}

// TestDroppedWorldIsGone: a world the player has saved that leaves
// relay.toml is gone on the next load, characters and all, and the
// status line says so.
func TestDroppedWorldIsGone(t *testing.T) {
	ws, _, _, cfg := launchAll(t, nil)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	path := "/home/kiln/.config/kiln/worlds/fake.toml"
	saved := `new Promise((ok) => { const r = indexedDB.open("kiln"); r.onsuccess = () => {
		const g = r.result.transaction("files").objectStore("files").get("` + path + `");
		g.onsuccess = () => { r.result.close(); ok(g.result ? new TextDecoder().decode(g.result.bytes) : ""); };
	}; })`
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Active" && e.Key === "fake/tester")`, "opening the character")
	ev(`localStorage.setItem("kiln.notify.asked", "1")`)
	ev(`document.querySelector(".bottom textarea").focus()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "/highlight lighthouse"})
	pressKey(t, ws, "Enter", 13)
	ev(`(async function poll() { window.isSaved = (await ` + saved + `).includes("lighthouse"); if (!window.isSaved) setTimeout(poll, 100); })()`)
	waitFor(t, ev, `window.isSaved === true`, "the world saved")

	// The world leaves the allowlist.
	b, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, b[:bytes.Index(b, []byte("[[world]]"))], 0o644); err != nil {
		t.Fatal(err)
	}
	ev(`(async function poll() { window.dropped = !(await (await fetch("preset/manifest.json")).text()).includes("fake"); if (!window.dropped) setTimeout(poll, 100); })()`)
	waitFor(t, ev, `window.dropped === true`, "the relay reloaded")

	devtools(t, ws, "Page.reload", map[string]any{})
	waitFor(t, ev, `typeof kilnPage !== "undefined" && kilnPage.events.some(e => e.type === "Order")`, "the page reloaded")
	waitFor(t, ev, `!fs.exists("`+path+`") && document.querySelector('[role="status"]').textContent.includes(`+jsString(str.WebWorldsGone(1, "fake"))+`)`, "the world gone, the status saying so")
	if got := ev(`(kilnPage.events.filter(e => e.type === "Worlds").at(-1).Worlds ?? []).length`); got != "0" {
		t.Errorf("the picker still has %s worlds", got)
	}
}
