package smoke

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// keptWasm waits until the service worker has kept the launch's files.
const keptWasm = `caches.open("kiln-meta").then(c => c.match("current")).then(r => r ? r.json() : null).then(k => !!k && Object.keys(k.stamps).some(u => u.endsWith("/kiln.wasm")))`

// TestOfflineLaunch launches Kiln once, online, then with no network:
// it still launches, from the files the first launch kept.
func TestOfflineLaunch(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	waitFor(t, ev, keptWasm, "the launch's files kept")

	devtools(t, ws, "Network.enable", map[string]any{})
	devtools(t, ws, "Network.emulateNetworkConditions", map[string]any{"offline": true, "latency": 0, "downloadThroughput": -1, "uploadThroughput": -1})
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond) // the old page is gone
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "an offline launch")
}

// TestNewBuildReachesAnOnlineLaunch changes a file of the page on the
// relay's disk: with the worker in charge, the next launch uses the new
// one, not the kept one.
func TestNewBuildReachesAnOnlineLaunch(t *testing.T) {
	ws, _, static := launchFrom(t, nil)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	waitFor(t, ev, keptWasm, "the launch's files kept")
	waitFor(t, ev, `!!navigator.serviceWorker.controller`, "the worker in charge")

	css := filepath.Join(static, "app.css")
	b, err := os.ReadFile(css)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(css, append(b, "\n.kiln-smoke-new { color: red }\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	// A later modification time than the kept copy's, as a deploy makes.
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(css, later, later); err != nil {
		t.Fatal(err)
	}
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the next launch")
	waitFor(t, ev, `[...document.styleSheets].some(s => [...s.cssRules].some(r => r.selectorText === ".kiln-smoke-new"))`, "the new styles in use")
}
