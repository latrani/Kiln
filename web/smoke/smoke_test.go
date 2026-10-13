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
)

func TestPageConnectsAndEchoes(t *testing.T) {
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
	ws := openTab(t, chrome, page)

	ev := evaluator(t, ws)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Order")`, "the first batch")
	ev(`kilnPage.act({Do: "open", Key: "fake/tester"})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Active" && e.Key === "fake/tester")`, "opening the character")
	ev(`kilnPage.act({Do: "submit", Key: "fake/tester", Text: "", N: 1000})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Char" && e.Key === "fake/tester" && e.State === 2)`, "connecting")
	// The default login line asks for the password: answer it, and the
	// fake MUCK echoes the login.
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Input" && e.Key === "fake/tester" && e.NeedPW)`, "the password prompt")
	ev(`kilnPage.act({Do: "submit", Key: "fake/tester", Text: "secret", N: 1001})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes('You say, "connect Tester secret"')))`, "the login")
	ev(`kilnPage.act({Do: "submit", Key: "fake/tester", Text: "hello", N: 1002})`)
	waitFor(t, ev, `kilnPage.events.some(e => e.type === "Lines" && e.Lines.some(l => l.Plain.includes('You say, "hello"')))`, "the echo")
}

// buildPage builds the wasm into a copy of web/static and returns it.
func buildPage(t *testing.T, root string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(root, "web", "static")
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".test.mjs") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
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
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

// config writes a preset with the fake world, and an allowlist for it.
func config(t *testing.T, port int) (preset, allow string) {
	t.Helper()
	dir := t.TempDir()
	preset = filepath.Join(dir, "preset")
	if err := os.MkdirAll(filepath.Join(preset, "worlds"), 0o755); err != nil {
		t.Fatal(err)
	}
	world := fmt.Sprintf("host = \"127.0.0.1\"\nport = %d\ntls = true\ntls_trust = \"pin\"\n\n[[characters]]\nid = \"tester\"\nname = \"Tester\"\n", port)
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
func openTab(t *testing.T, chrome, page string) *websocket.Conn {
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
