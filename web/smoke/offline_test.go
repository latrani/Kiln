package smoke

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestOfflineLaunch launches Kiln once with the service worker in
// charge, so it keeps the page; then, with no network, Kiln still
// launches.
func TestOfflineLaunch(t *testing.T) {
	ws, _ := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	waitFor(t, ev, `navigator.serviceWorker.getRegistration().then(r => !!r?.active)`, "the service worker running")
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond) // the old page is gone
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order") && !!navigator.serviceWorker.controller`, "a launch with the worker in charge")
	waitFor(t, ev, `caches.open("kiln").then(c => c.match(new URL("kiln.wasm", location.href).href)).then(r => !!r)`, "the wasm kept")

	devtools(t, ws, "Network.enable", map[string]any{})
	devtools(t, ws, "Network.emulateNetworkConditions", map[string]any{"offline": true, "latency": 0, "downloadThroughput": -1, "uploadThroughput": -1})
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "an offline launch")
}

// TestNewBuildReachesAnOnlineLaunch changes a file of the page on the
// relay's disk: with the worker in charge, the next launch gets the new
// one, not the kept one.
func TestNewBuildReachesAnOnlineLaunch(t *testing.T) {
	ws, _, static := launchFrom(t, nil)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	waitFor(t, ev, `navigator.serviceWorker.getRegistration().then(r => !!r?.active)`, "the service worker running")
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order") && !!navigator.serviceWorker.controller`, "a launch with the worker in charge")
	waitFor(t, ev, `caches.open("kiln").then(c => c.match(new URL("app.css", location.href).href)).then(r => !!r)`, "the styles kept")

	css := filepath.Join(static, "app.css")
	b, err := os.ReadFile(css)
	if err != nil {
		t.Fatal(err)
	}
	// A later modification time than the kept copy's, as a deploy makes.
	if err := os.WriteFile(css, append(b, "\n/* kiln-smoke-new */\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(css, later, later)
	devtools(t, ws, "Page.reload", map[string]any{})
	time.Sleep(200 * time.Millisecond)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the next launch")
	waitFor(t, ev, `[...document.styleSheets].some(s => [...s.cssRules].some(r => r.cssText.includes("kiln-smoke-new"))) || fetch("app.css").then(r => r.text()).then(t => t.includes("kiln-smoke-new"))`, "the new styles")
}
