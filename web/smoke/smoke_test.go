// Package smoke drives Kiln's web build end to end: the page in headless
// Chrome, through kiln-relay, to a fake MUCK. It's skipped unless
// KILN_SMOKE=1; see docs/web-dev.md.
package smoke

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/latrani/Kiln/internal/glyph"
	"github.com/latrani/Kiln/internal/str"
)

// launch starts the fake MUCK, the relay and Chrome, and opens the page
// (recording events and actions) in a tab, skipping without
// KILN_SMOKE=1 or Chrome. another opens the page, not recording, in
// another tab.
func launch(t *testing.T) (ws *websocket.Conn, another func() *websocket.Conn) {
	t.Helper()
	if os.Getenv("KILN_SMOKE") != "1" {
		t.Skip("set KILN_SMOKE=1 to run the browser smoke test")
	}
	chrome := os.Getenv("KILN_CHROME")
	if chrome == "" {
		chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}
	if _, err := os.Stat(chrome); err != nil {
		t.Skipf("no Chrome at %s (set KILN_CHROME)", chrome)
	}
	root, _ := filepath.Abs("../..")
	static := buildPage(t, root)
	muckPort := fakeMuck(t)
	preset, allow := config(t, muckPort)
	page := startRelay(t, root, static, preset, allow)
	ws, tab := openTab(t, chrome, page+"?record")
	return ws, func() *websocket.Conn { return tab(page) }
}

func TestPageConnectsAndEchoes(t *testing.T) {
	ws, another := launch(t)
	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Active" && e.Key === "fake/tester")`, "opening the character")
	ev(`kilnPage.act({Do: "submit", Key: "fake/tester", Text: "", N: 1000})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Char" && e.Key === "fake/tester" && e.State === 2)`, "connecting")
	// The default login line asks for the password: answer it, and the
	// fake MUCK echoes the login.
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Input" && e.Key === "fake/tester" && e.NeedPW)`, "the password prompt")
	waitFor(t, ev, `document.activeElement === document.querySelector('.bottom input[type="password"]')`, "the password field focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "secret"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes('You say, "connect Tester secret"')))`, "the login")
	if got := ev(`JSON.stringify(kilnPage.sent).includes("secret")`); got != "false" {
		t.Error("the password is in the page's record of what it sent")
	}
	// Typed and sent as a person does: the input has focus, and Enter sends.
	waitFor(t, ev, `document.activeElement === document.querySelector(".bottom textarea")`, "the input focused")
	devtools(t, ws, "Input.insertText", map[string]any{"text": "hello"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes('You say, "hello"')))`, "the echo")
	waitFor(t, ev, `[...document.querySelectorAll("main .line")].some(l => l.textContent.includes('You say, "hello"'))`, "the echo on the page")
	waitFor(t, ev, `document.querySelector(".bottom textarea").value === ""`, "the input cleared")
	pressKey(t, ws, "ArrowUp", 38)
	waitFor(t, ev, `document.querySelector(".bottom textarea").value === "hello"`, "history recalled")

	// /backup gives a file; /restore picks one and puts it back.
	ev(`document.querySelector(".bottom textarea").select()`)
	devtools(t, ws, "Input.insertText", map[string]any{"text": "/backup"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Effect" && e.effect === "SaveFile" && e.Name.endsWith(".zip"))`, "a backup")
	var zip []byte
	json.Unmarshal([]byte(ev(`kilnPage.events.find(e => e.type === "Effect" && e.effect === "SaveFile").Data`)), &zip)
	file := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(file, zip, 0o600); err != nil || len(zip) == 0 {
		t.Fatalf("backup of %d bytes: %v", len(zip), err)
	}
	devtools(t, ws, "Input.insertText", map[string]any{"text": "/restore"})
	pressKey(t, ws, "Enter", 13)
	waitFor(t, ev, `document.querySelector('input[type="file"]')`, "the file picker")
	var picker struct{ Result struct{ ObjectID string } }
	json.Unmarshal(devtools(t, ws, "Runtime.evaluate", map[string]any{"expression": `document.querySelector('input[type="file"]')`}), &picker)
	devtools(t, ws, "DOM.setFileInputFiles", map[string]any{"files": []string{file}, "objectId": picker.Result.ObjectID})
	var restored []string // what the status says for any likely count
	for n := 1; n <= 50; n++ {
		restored = append(restored, str.StatusRestored(n))
	}
	said, _ := json.Marshal(restored)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Status" && `+string(said)+`.includes(e.Text))`, "the backup restored")

	// The sidebar: as wide as the TUI's, in whole characters; and a
	// character's name switches to it.
	waitFor(t, ev, `(() => {
		const p = Object.assign(document.createElement("span"), { style: "position:absolute;width:1ch" });
		document.querySelector("nav").append(p);
		const n = document.querySelector("nav").getBoundingClientRect().width / p.getBoundingClientRect().width;
		p.remove();
		return Math.abs(n - Math.round(n)) < 0.01 && n >= 14 && n <= 22;
	})()`, "the sidebar a whole number of characters wide")
	ev(`kilnPage.act({Do: "open", Key: "fake/second"})`)
	waitFor(t, ev, `[...document.querySelectorAll("nav button")].some(b => b.textContent === "Second")`, "a second character in the sidebar")
	ev(`[...document.querySelectorAll("nav button")].find(b => b.textContent === "Second").click()`)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Active").at(-1).Key === "fake/second"`, "switching from the sidebar")
	ev(`[...document.querySelectorAll("nav button")].find(b => b.textContent === "Tester").click()`)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Active").at(-1).Key === "fake/tester"`, "switching back from the sidebar")

	// Presence: a key is you here. Ctrl+↓ steps to the next character.
	ev(`window.dispatchEvent(new FocusEvent("focus")); document.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }))`)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Presence").at(-1)?.State === 1`, "presence here")
	ev(`document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", ctrlKey: true, bubbles: true }))`)
	waitFor(t, ev, `kilnPage.events.filter(e => e.type === "Active").at(-1).Key !== "fake/tester"`, "Ctrl+Down switching")

	// Every glyph Kiln draws comes from the bundled font, one cell wide. A
	// glyph drawn by a fallback measures differently under a serif and a
	// monospace fallback; the bundled face itself must have loaded.
	ev(`document.fonts.load('16px "JetBrains Mono"')`)
	waitFor(t, ev, `[...document.fonts].some(f => f.family.replace(/"/g, "") === "JetBrains Mono" && f.status === "loaded")`, "the bundled font")
	var gs []string
	for _, g := range glyph.All {
		gs = append(gs, g[1])
	}
	list, _ := json.Marshal(gs)
	ev(`(() => { const c = document.createElement("canvas").getContext("2d");
		const w = (font, s) => { c.font = font; return c.measureText(s).width; };
		const cell = w('16px "JetBrains Mono", monospace', "M");
		window.kilnGlyphs = ` + string(list) + `.filter(g =>
			Math.abs(w('16px "JetBrains Mono", serif', g) - w('16px "JetBrains Mono", monospace', g)) > 0.01 ||
			Math.abs(w('16px "JetBrains Mono", monospace', g) - cell) > 0.01).join(""); })()`)
	waitFor(t, ev, `window.kilnGlyphs === ""`, "every glyph one cell wide")

	// A second tab finds Kiln open in the first and says so, with Use
	// here; taking over leaves the first tab saying so in turn.
	ev2 := evaluator(t, another())
	waitFor(t, ev2, `[...document.querySelectorAll("main button")].some(b => b.textContent === `+jsString(str.WebUseHere())+`)`, "the second tab offering to take over")
	ev2(`[...document.querySelectorAll("main button")].find(b => b.textContent === ` + jsString(str.WebUseHere()) + `).click()`)
	waitFor(t, ev2, `kilnPage && document.querySelector("main button") === null`, "the second tab taking over")
	waitFor(t, ev, `[...document.querySelectorAll("main button")].some(b => b.textContent === `+jsString(str.WebUseHere())+`)`, "the first tab, taken over, offering it back")
}

// pressKey presses and lets go of a key in the tab, as a keyboard does.
func pressKey(t *testing.T, ws *websocket.Conn, key string, code int) {
	t.Helper()
	for _, typ := range []string{"rawKeyDown", "keyUp"} {
		p := map[string]any{"type": typ, "key": key, "code": key, "windowsVirtualKeyCode": code, "nativeVirtualKeyCode": code}
		if typ == "rawKeyDown" && key == "Enter" {
			p["text"] = "\r"
			p["type"] = "keyDown"
		}
		devtools(t, ws, "Input.dispatchKeyEvent", p)
	}
}

// jsString is s as a JavaScript string literal.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// buildPage builds the page (web/ui) and the wasm into a scratch
// directory and returns it. The page needs web/ui's packages installed.
func buildPage(t *testing.T, root string) string {
	t.Helper()
	dir := t.TempDir()
	ui := filepath.Join(root, "web", "ui")
	if _, err := os.Stat(filepath.Join(ui, "node_modules")); err != nil {
		t.Skip("run npm ci in web/ui first")
	}
	page := exec.Command("node", "build.mjs", dir)
	page.Dir = ui
	if out, err := page.CombinedOutput(); err != nil {
		t.Fatalf("building the page: %v\n%s", err, out)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "kiln.wasm"), "./cmd/kiln-web")
	build.Dir = filepath.Join(root, "web")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the wasm: %v\n%s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fakeMuck serves TLS on a free port with a self-signed certificate:
// a greeting, then `You say, "L"` for each line L. It returns the port.
func fakeMuck(t *testing.T) int {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "fake.muck"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				fmt.Fprint(c, "Welcome to the fake MUCK\r\n")
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					l := strings.Map(func(r rune) rune {
						if r < ' ' || r > '~' {
							return -1
						}
						return r
					}, sc.Text())
					if l != "" {
						fmt.Fprintf(c, "You say, %q\r\n", l)
					}
					// "spam N" sends N long lines now; "later N", in a second.
					var n int
					if _, err := fmt.Sscanf(l, "spam %d", &n); err == nil {
						lines(c, "spam", n)
					} else if _, err := fmt.Sscanf(l, "later %d", &n); err == nil {
						go func() {
							time.Sleep(time.Second)
							lines(c, "late", n)
						}()
					}
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// lines sends n numbered lines long enough to wrap at most widths.
func lines(c net.Conn, what string, n int) {
	for i := 1; i <= n; i++ {
		fmt.Fprintf(c, "%s %d: the kiln ticks as it cools, and the glaze settles into the slow colors of the evening light\r\n", what, i)
	}
}

// config writes a preset with the fake world, and an allowlist for it.
func config(t *testing.T, port int) (preset, allow string) {
	t.Helper()
	dir := t.TempDir()
	preset = filepath.Join(dir, "preset")
	if err := os.MkdirAll(filepath.Join(preset, "worlds"), 0o755); err != nil {
		t.Fatal(err)
	}
	world := fmt.Sprintf("host = \"127.0.0.1\"\nport = %d\ntls = true\ntls_trust = \"pin\"\n\n[[characters]]\nid = \"tester\"\nname = \"Tester\"\n\n[[characters]]\nid = \"second\"\nname = \"Second\"\n\n[[characters]]\nid = \"historian\"\nname = \"Historian\"\n", port)
	if err := os.WriteFile(filepath.Join(preset, "worlds", "fake.toml"), []byte(world), 0o644); err != nil {
		t.Fatal(err)
	}
	allow = filepath.Join(dir, "allow.toml")
	if err := os.WriteFile(allow, fmt.Appendf(nil, "[[world]]\nhost = \"127.0.0.1\"\nports = [%d]\n", port), 0o644); err != nil {
		t.Fatal(err)
	}
	return preset, allow
}

// startRelay runs kiln-relay -dev on a free port and returns the page's URL.
func startRelay(t *testing.T, root, static, preset, allow string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	bin := filepath.Join(t.TempDir(), "kiln-relay")
	build := exec.Command("go", "build", "-o", bin, "./cmd/kiln-relay")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building kiln-relay: %v\n%s", err, out)
	}
	relay := exec.Command(bin, "-dev", "-listen", addr, "-allow", allow, "-web", static, "-preset", preset)
	relay.Stdout, relay.Stderr = os.Stderr, os.Stderr
	if err := relay.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { relay.Process.Kill(); relay.Wait() })
	page := "http://" + addr + "/kiln/"
	for deadline := time.Now().Add(10 * time.Second); ; {
		if res, err := http.Get(page); err == nil {
			res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("kiln-relay never served the page")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return page
}

// openTab starts headless Chrome on page and returns a DevTools
// connection to its tab.
// openTab starts headless Chrome on page and returns its tab, and a way
// to open another tab in the same browser (sharing its storage and locks).
func openTab(t *testing.T, chrome, page string) (*websocket.Conn, func(page string) *websocket.Conn) {
	t.Helper()
	profile := t.TempDir()
	cmd := exec.Command(chrome, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile,
		"--no-first-run", "--no-default-browser-check", "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	var port string
	for deadline := time.Now().Add(15 * time.Second); port == ""; {
		if b, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			port, _, _ = strings.Cut(string(b), "\n")
		}
		if time.Now().After(deadline) {
			t.Fatal("Chrome never opened DevTools")
		}
		time.Sleep(100 * time.Millisecond)
	}
	tab := func(page string) *websocket.Conn { return newTab(t, port, page) }
	return tab(page), tab
}

// newTab opens page in another tab of the Chrome listening on port.
func newTab(t *testing.T, port, page string) *websocket.Conn {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, "http://127.0.0.1:"+port+"/json/new?"+page, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var target struct{ WebSocketDebuggerURL string }
	if err := json.NewDecoder(res.Body).Decode(&target); err != nil {
		t.Fatal(err)
	}
	ws, _, err := websocket.Dial(context.Background(), target.WebSocketDebuggerURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetReadLimit(64 << 20)
	t.Cleanup(func() { ws.CloseNow() })
	return ws
}

// evaluator returns a function that evaluates JS in the tab and returns
// its value as JSON ("" if it threw).
func evaluator(t *testing.T, ws *websocket.Conn) func(string) string {
	id := 0
	return func(expr string) string {
		t.Helper()
		id++
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		msg, _ := json.Marshal(map[string]any{"id": id, "method": "Runtime.evaluate",
			"params": map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": true}})
		if err := ws.Write(ctx, websocket.MessageText, msg); err != nil {
			t.Fatal(err)
		}
		for {
			_, b, err := ws.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var reply struct {
				ID     int
				Result struct {
					Result struct {
						Value json.RawMessage
					}
					ExceptionDetails json.RawMessage
				}
			}
			if json.Unmarshal(b, &reply) != nil || reply.ID != id {
				continue // an event, or someone else's reply
			}
			if reply.Result.ExceptionDetails != nil {
				return ""
			}
			return string(reply.Result.Result.Value)
		}
	}
}

// waitFor polls cond in the tab until it's true, failing after 20
// seconds with the page's last events.
func waitFor(t *testing.T, ev func(string) string, cond, what string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for ev("(() => { try { return !!("+cond+") } catch { return false } })()") != "true" {
		if time.Now().After(deadline) {
			last := ev(`JSON.stringify((globalThis.kilnPage?.events ?? []).slice(-20))`)
			t.Fatalf("waiting for %s: gave up; last events:\n%s", what, last)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
